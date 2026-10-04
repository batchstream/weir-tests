#!/usr/bin/env python3
"""Prepare the full module graph without changing the checkout's checksums."""
import os
from pathlib import Path
import subprocess
import tempfile


def download_modules(root):
    inputs = {name: (root / name).read_bytes() for name in ('go.mod', 'go.sum')}
    env = dict(os.environ, GOWORK='off', GOTOOLCHAIN='local', GOFLAGS='-mod=readonly')
    env.pop('GOROOT', None)
    with tempfile.TemporaryDirectory(prefix='weir-test-modules-') as temporary:
        work = Path(temporary)
        for name, content in inputs.items():
            (work / name).write_bytes(content)
        subprocess.run(['go', 'mod', 'download', 'all'], cwd=work, env=env, check=True, timeout=180)
    for name, content in inputs.items():
        if (root / name).read_bytes() != content:
            raise RuntimeError('module preparation changed ' + name)


if __name__ == '__main__':
    download_modules(Path(__file__).resolve().parent.parent)
