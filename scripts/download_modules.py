#!/usr/bin/env python3
"""Prepare the full module graph without changing the checkout's checksums."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

from server_source import build_environment, freeze_server_source, read_file


def download_modules(root, *, server=False):
    if server and root.is_symlink():
        raise ValueError('server source root must not be a symlink')
    root = root.resolve()
    inputs = {} if server else {name: (root / name).read_bytes() for name in ('go.mod', 'go.sum')}
    env = dict(os.environ, GOWORK='off', GOTOOLCHAIN='local', GOFLAGS='-mod=readonly')
    env.pop('GOROOT', None)
    with tempfile.TemporaryDirectory(prefix='weir-test-modules-') as temporary:
        work = Path(temporary)
        if server:
            # Validate the same complete source/nested-patch allowlist as the offline build.
            work = work / 'server'
            _, manifest = freeze_server_source(root, work, build_environment())
            inputs = {entry['path']: (work / entry['path']).read_bytes() for entry in manifest
                      if Path(entry['path']).name in ('go.mod', 'go.sum')}
            # Candidate preparation is public and checksummed, even with private Go defaults.
            env = build_environment()
            env.pop('GOROOT', None)
            env.update(GOPROXY='https://proxy.golang.org', GOSUMDB='sum.golang.org',
                       GOPRIVATE='', GONOPROXY='', GONOSUMDB='', GOINSECURE='')
        else:
            for name, content in inputs.items():
                (work / name).write_bytes(content)
        subprocess.run(['go', 'mod', 'download', 'all'], cwd=work, env=env, check=True, timeout=180)
    for name, content in inputs.items():
        actual = read_file(root, name) if server else (root / name).read_bytes()
        if actual != content:
            raise RuntimeError('module preparation changed ' + name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--server-source', type=Path,
                        help='prepare an explicit candidate graph including validated nested patch modules')
    args = parser.parse_args()
    if args.server_source is not None:
        download_modules(args.server_source, server=True)
    else:
        download_modules(Path(__file__).resolve().parent.parent)


if __name__ == '__main__':
    main()
