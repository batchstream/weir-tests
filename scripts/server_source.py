"""Controlled full-checkout and server-only source snapshot boundaries."""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess


PUBLIC_SOURCE = 'https://github.com/batchstream/weir.git'
SERVER_MODULE = 'github.com/batchstream/weir'
NESTED = {'google.golang.org/grpc': 'third_party/grpc',
          'github.com/iceisfun/golua': 'third_party/golua'}
METADATA = {'go.mod', 'go.sum', 'LICENSE', 'NOTICE', 'AUTHORS',
            'WEIR_PATCH.diff', 'WEIR_PATCH.md', 'WEIR_UPSTREAM.json'}


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


def validate_identity(identity, toolchain, revision=None):
    version, target = toolchain
    expected = dict(product='weir', go=version, target=target)
    if revision is None:
        expected.update(revision='unknown', version='dev', state='dev', dirty='unknown')
    else:
        expected.update(revision=revision, version='local-' + revision, state='clean-commit', dirty='false')
    if any(identity.get(key) != value for key, value in expected.items()):
        raise ValueError('server identity does not match source, toolchain, target or build state')


def safe_path(name, public_grpc=False):
    path = PurePosixPath(name)
    # gRPC's public credentials/*.go packages are production code, not secret inputs.
    credential_package = public_grpc and name.endswith('.go') and path.name != 'credentials'
    forbidden = any(part.startswith('.') or (part == 'credentials' and not credential_package)
                    for part in path.parts)
    if not name or path.is_absolute() or path.as_posix() != name or forbidden or '\\' in name:
        raise ValueError('unsafe server source path: ' + name)
    return Path(*path.parts)


def read_file(root, name, public_grpc=False):
    public_grpc = public_grpc or name.startswith('third_party/grpc/')
    relative = safe_path(name, public_grpc)
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


def go_paths(root, relative, public_grpc=False):
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
                           and (name != 'credentials' or public_grpc)]
        for name in directories:
            if (current / name).is_symlink():
                raise ValueError('symlink directory in server source')
        for name in names:
            if production_file(name) and not name.startswith('.'):
                path = (current / name).relative_to(root).as_posix()
                safe_path(path, public_grpc)
                if (current / name).is_symlink():
                    raise ValueError('symlink in server source: ' + path)
                paths.append(path)
            elif name == 'go.mod' and current != root:
                raise ValueError('unexpected nested module in server source')
    return paths


def nested_files(root, options, environment):
    module, relative, version = options['module'], options['relative'], options['version']
    public_grpc = module == 'google.golang.org/grpc'
    directory = root / relative
    read_file(root, relative + '/go.mod')
    metadata = module_metadata(directory, environment)
    if metadata.get('Module', {}).get('Path') != module or metadata.get('Replace'):
        raise ValueError('nested module identity or replacement mismatch')
    manifest = json.loads(read_file(root, relative + '/WEIR_UPSTREAM.json'))
    if manifest.get('module') != module or manifest.get('version') != version:
        raise ValueError('nested upstream manifest identity mismatch')
    hashes = dict(manifest['files'])
    for name, value in hashes.items():
        safe_path(name, public_grpc)
        if re.fullmatch(r'[0-9a-f]{64}', value) is None:
            raise ValueError('invalid upstream file hash')
    changed = set()
    for entry in manifest.get('patchedFiles', []):
        name = entry['path']
        if name in changed or hashes.get(name) != entry['upstream_sha256']:
            raise ValueError('inconsistent patched file manifest')
        changed.add(name)
        hashes[name] = entry['sha256']
    for entry in manifest.get('newFiles', []):
        name = entry['path']
        if name in hashes:
            raise ValueError('duplicate new file manifest')
        hashes[name] = entry['sha256']
    selected = {}
    for name, value in hashes.items():
        safe_path(name, public_grpc)
        if re.fullmatch(r'[0-9a-f]{64}', value) is None:
            raise ValueError('invalid nested file hash')
        if name.endswith('_test.go'):
            continue
        if not production_file(name) and name not in METADATA:
            raise ValueError('unexpected upstream manifest file: ' + name)
        data = read_file(root, relative + '/' + name)
        if digest(data) != value:
            raise ValueError('nested source hash mismatch: ' + name)
        selected[relative + '/' + name] = data
    for name in ('go.mod', 'LICENSE', 'WEIR_PATCH.diff', 'WEIR_PATCH.md'):
        if relative + '/' + name not in selected:
            raise ValueError('nested source missing required metadata: ' + name)
    selected[relative + '/WEIR_UPSTREAM.json'] = read_file(root, relative + '/WEIR_UPSTREAM.json')
    actual_names = {relative + '/' + name for name in go_paths(directory, Path('.'), public_grpc)}
    expected_names = {name for name in selected if production_file(name)}
    if actual_names != expected_names:
        raise ValueError('nested production source differs from manifest')
    for name in ('go.mod', 'go.sum', 'LICENSE', 'NOTICE', 'AUTHORS'):
        if (directory / name).exists() and relative + '/' + name not in selected:
            raise ValueError('unhashed nested metadata: ' + name)
    return selected


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
    requirements = {item['Path']: item['Version'] for item in metadata.get('Require', [])}
    replacements = {}
    for item in metadata.get('Replace', []):
        old, new = item['Old'], item['New']
        name = old['Path']
        if (name not in NESTED or name in replacements or old.get('Version') or new.get('Version')
                or new['Path'] != './' + NESTED[name] or name not in requirements):
            raise ValueError('unexpected or escaping server module replacement')
        replacements[name] = NESTED[name]
    files = {name: read_file(source, name) for name in ('go.mod', 'go.sum')}
    for directory in ('cmd', 'internal', 'pkg', 'api'):
        for name in go_paths(source, Path(directory)):
            files[name] = read_file(source, name)
    for module, relative in replacements.items():
        options = dict(module=module, relative=relative, version=requirements[module])
        files.update(nested_files(source, options, environment))
    if replacements:
        files['third_party/README.md'] = read_file(source, 'third_party/README.md')
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
