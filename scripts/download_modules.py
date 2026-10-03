#!/usr/bin/env python3
"""Prepare the complete module cache without changing module inputs."""
import os
from pathlib import Path
import subprocess


root = Path(__file__).resolve().parent.parent
env = dict(os.environ, GOWORK='off', GOTOOLCHAIN='local', GOFLAGS='-mod=readonly')
env.pop('GOROOT', None)
before = {name: (root / name).read_bytes() for name in ('go.mod', 'go.sum')}
subprocess.run(['go', 'mod', 'download', 'all'], cwd=root, env=env, check=True, timeout=180)
for name, content in before.items():
    if (root / name).read_bytes() != content:
        raise RuntimeError('module preparation changed ' + name)
