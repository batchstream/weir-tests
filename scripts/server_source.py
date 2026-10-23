"""Controlled full-checkout and server-only source snapshot boundaries."""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess


PUBLIC_SOURCE = 'https://github.com/batchstream/weir.git'
SERVER_MODULE = 'github.com/batchstream/weir'


def digest(data):
    return hashlib.sha256(data).hexdigest()


def build_environment():
    environment = {key: value for key, value in os.environ.items() if not key.startswith('GIT_')}
    environment.update(GOENV='off', GOWORK='off', GOTOOLCHAIN='local', GOFLAGS='-mod=readonly',
                       CGO_ENABLED='0', GIT_CONFIG_GLOBAL='/dev/null', GIT_CONFIG_SYSTEM='/dev/null',
                       GIT_CONFIG_NOSYSTEM='1', GIT_TERMINAL_PROMPT='0')
    return environment


def git(arguments, directory, environment):
    command = ['git', '-c', 'credential.helper=', '-c', 'core.hooksPath=/dev/null',
               '-c', 'protocol.file.allow=never', '-c', 'protocol.ext.allow=never', *arguments]
    return subprocess.check_output(command, cwd=directory, env=environment, timeout=180)


def check_checkout(directory, revision, environment):
    actual = git(['rev-parse', 'HEAD'], directory, environment).decode().strip()
    status = git(['status', '--porcelain', '--untracked-files=all', '--ignored'], directory, environment)
    if actual != revision or status:
        raise ValueError('immutable server source is dirty or mismatched')


def checkout_source(directory, revision, environment):
    if re.fullmatch(r'[0-9a-f]{40}', revision) is None:
        raise ValueError('source must be a full immutable commit')
    directory.mkdir()
    git(['init', '-q', '--template=', '.'], directory, environment)
    git(['fetch', '--depth=1', PUBLIC_SOURCE, revision], directory, environment)
    git(['checkout', '-q', '--detach', 'FETCH_HEAD'], directory, environment)
    check_checkout(directory, revision, environment)


def native_toolchain(environment, expected=None):
    raw = subprocess.check_output(['go', 'env', 'GOVERSION', 'GOHOSTOS', 'GOHOSTARCH', 'GOOS', 'GOARCH'],
                                  env=environment, text=True, timeout=10).splitlines()
    if len(raw) != 5:
        raise ValueError('cannot determine native Go toolchain and target')
    version, host_os, host_arch, target_os, target_arch = raw
    if (host_os, host_arch) != (target_os, target_arch) or (expected and version != 'go' + expected):
        raise ValueError('requires the locked native Go toolchain and target')
    return version, host_os + '/' + host_arch


def validate_identity(identity, toolchain, revision):
    version, target = toolchain
    # A snapshot build has no module version or VCS stamp; the revision comes from -X main.revision.
    expected = dict(product='weir', version='dev', revision=revision, go=version, target=target)
    if identity != expected:
        raise ValueError('server identity does not match source, toolchain or target')


def safe_path(name):
    path = PurePosixPath(name)
    forbidden = any(part.startswith('.') or part == 'credentials' for part in path.parts)
    if not name or path.is_absolute() or path.as_posix() != name or forbidden or '\\' in name:
        raise ValueError('unsafe server source path: ' + name)
    return Path(*path.parts)


def read_file(root, name):
    relative = safe_path(name)
    path = root
    for component in relative.parts:
        path = path / component
        if path.is_symlink():
            raise ValueError('symlink in server source: ' + name)
    if not path.is_file() or not path.resolve().is_relative_to(root.resolve()):
        raise ValueError('missing or escaping server source: ' + name)
    return path.read_bytes()


def module_metadata(root, environment):
    read_file(root, 'go.mod')
    raw = subprocess.check_output(['go', 'mod', 'edit', '-json'], cwd=root,
                                  env=environment, text=True, timeout=10)
    return json.loads(raw)


def production_file(name):
    return name.endswith('.go') and not name.endswith('_test.go')


def go_paths(root, relative):
    start = root / relative
    paths = []
    if start.is_symlink():
        raise ValueError('symlink in server source: ' + str(relative))
    if not start.exists():
        return paths
    for directory, directories, names in os.walk(start, followlinks=False):
        current = Path(directory)
        directories[:] = [name for name in directories if not name.startswith('.')
                           and name not in ('vendor', 'testdata')
                           and name != 'credentials']
        for name in directories:
            if (current / name).is_symlink():
                raise ValueError('symlink directory in server source')
        for name in names:
            if production_file(name) and not name.startswith('.'):
                path = (current / name).relative_to(root).as_posix()
                safe_path(path)
                if (current / name).is_symlink():
                    raise ValueError('symlink in server source: ' + path)
                paths.append(path)
            elif name == 'go.mod' and current != root:
                raise ValueError('unexpected nested module in server source')
    return paths


def verify_snapshot(root, manifest, expected):
    tree = hashlib.sha256()
    names = []
    for entry in manifest:
        name = entry['path']
        data = read_file(root, name)
        if digest(data) != entry['sha256']:
            raise ValueError('snapshot file hash mismatch: ' + name)
        names.append(name)
        tree.update(name.encode() + b'\0' + data + b'\0')
    actual = set()
    for directory, directories, files in os.walk(root, followlinks=False):
        current = Path(directory)
        if any((current / name).is_symlink() for name in directories + files):
            raise ValueError('symlink in source snapshot')
        actual.update((current / name).relative_to(root).as_posix() for name in files)
    if (names != sorted(set(names)) or tree.hexdigest() != expected
            or actual - {'source.diff'} != set(names)):
        raise ValueError('snapshot manifest or tree hash mismatch')


def freeze_server_source(source, output, environment):
    if source.is_symlink():
        raise ValueError('server source root must not be a symlink')
    source = source.resolve()
    if output.exists() or output.is_symlink():
        raise ValueError('server snapshot output must be new')
    metadata = module_metadata(source, environment)
    if metadata.get('Module', {}).get('Path') != SERVER_MODULE:
        raise ValueError('server go.mod has the wrong module identity')
    if metadata.get('Replace'):
        raise ValueError('server go.mod must not replace modules')
    files = {name: read_file(source, name) for name in ('go.mod', 'go.sum')}
    for directory in ('cmd', 'internal', 'pkg', 'api'):
        for name in go_paths(source, Path(directory)):
            files[name] = read_file(source, name)
    tree = hashlib.sha256()
    manifest = []
    # Read and verify the complete allowlist before creating any snapshot files.
    output.mkdir(parents=True)
    for name, data in sorted(files.items()):
        tree.update(name.encode() + b'\0' + data + b'\0')
        entry = dict(path=name, sha256=digest(data))
        manifest.append(entry)
        target = output / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
    if module_metadata(output, environment) != metadata:
        raise ValueError('server go.mod changed while freezing source')
    return tree.hexdigest(), manifest
