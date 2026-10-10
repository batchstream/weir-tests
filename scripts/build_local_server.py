#!/usr/bin/env python3
"""Freeze a local Weir Go source tree and build a checksummed benchmark binary."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess


def digest(data):
    return hashlib.sha256(data).hexdigest()


def freeze_source(source, output):
    source = source.resolve()
    files = []
    for directory, directories, names in os.walk(source, followlinks=False):
        current = Path(directory)
        directories[:] = [name for name in directories
                           if not name.startswith('.') and name not in ('vendor', 'credentials')
                           and not (current / name).is_symlink()
                           and (current != source or name in ('cmd', 'internal', 'pkg', 'api', 'tests'))]
        for name in names:
            if name.startswith('.') or name == 'credentials':
                continue
            path = current / name
            relative = path.relative_to(source)
            if path.is_symlink() or not path.is_file():
                continue
            if path.suffix != '.go' and relative.as_posix() not in ('go.mod', 'go.sum'):
                continue
            files.append((relative, path.read_bytes()))
    if not any(path.as_posix() == 'go.mod' for path, _ in files):
        raise ValueError('local source must contain go.mod')
    tree = hashlib.sha256()
    manifest = []
    for relative, data in sorted(files):
        name = relative.as_posix()
        tree.update(name.encode() + b'\0' + data + b'\0')
        entry = dict(path=name, sha256=digest(data))
        manifest.append(entry)
        target = output / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
    return tree.hexdigest(), manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path, help='new binary path; existing artifacts are preserved')
    args = parser.parse_args()
    source, output = args.source.resolve(), args.output.resolve()
    receipt_path = output.with_suffix('.receipt.json')
    frozen = output.with_suffix('.source')
    if output.exists() or receipt_path.exists() or frozen.exists():
        raise ValueError('output binary, receipt and source snapshot must all be new')
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip()
    status = subprocess.check_output(['git', 'status', '--porcelain'], cwd=source, text=True)
    frozen.mkdir(parents=True)
    tree_sha256, manifest = freeze_source(source, frozen)
    # Limit Git reads to the same explicit Go input allowlist used by the snapshot.
    diff_paths = [item['path'] for item in manifest]
    diff = subprocess.check_output(['git', 'diff', 'HEAD', '--', *diff_paths], cwd=source)
    (frozen / 'source.diff').write_bytes(diff)
    env = dict(os.environ, GOENV='off', GOWORK='off', GOFLAGS='-mod=readonly', CGO_ENABLED='0')
    # Ordinary local builds remain dev identities. The receipt identifies the frozen inputs.
    subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false',
                    '-o', str(output), './cmd/weir'], cwd=frozen, env=env, check=True, timeout=600)
    identity = json.loads(subprocess.check_output([str(output), 'version'], text=True))
    receipt = dict(source=revision, source_dirty=bool(status.strip()), source_tree_sha256=tree_sha256,
                   source_diff_sha256=digest(diff), source_files=manifest,
                   source_snapshot=str(frozen), source_kind='frozen local Go source tree',
                   sha256=digest(output.read_bytes()), identity=identity)
    receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
    result = dict(binary=str(output), receipt=str(receipt_path),
                  sha256=receipt['sha256'], source_tree_sha256=tree_sha256)
    print(json.dumps(result, indent=2))


if __name__ == '__main__':
    main()
