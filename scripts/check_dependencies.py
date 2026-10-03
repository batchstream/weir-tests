#!/usr/bin/env python3
"""Keep system tests independent of server internals and use released client contracts."""
import json
import os
from pathlib import Path
import subprocess


SERVER = 'github.com/batchstream/weir'
SDK = 'github.com/batchstream/weir-go'
PROTOCOL = 'github.com/batchstream/weir-protocol'


def objects(raw):
    decoder = json.JSONDecoder()
    items = []
    while raw.strip():
        item, end = decoder.raw_decode(raw.lstrip())
        items.append(item)
        raw = raw.lstrip()[end:]
    return items


def check_modules(items, pins):
    found = {}
    for item in items:
        name = item['Path']
        if item.get('Replace'):
            raise ValueError('local module replacements are forbidden: ' + name)
        if name == SERVER:
            raise ValueError('test module must not depend on the Weir server module')
        if name in (SDK, PROTOCOL):
            found[name] = item.get('Version')
    if found != {SDK: pins['sdk'], PROTOCOL: pins['protocol']}:
        raise ValueError('public SDK and protocol must match the exact released lock')


def check_graph(raw):
    for line in raw.splitlines():
        for node in line.split():
            if node.split('@', 1)[0] == SERVER:
                raise ValueError('server module exists in the raw dependency graph')


def check_packages(items):
    for item in items:
        if item.get('Module', {}).get('Path') == SERVER:
            raise ValueError('system tests import server packages: ' + item['ImportPath'])


def main():
    root = Path(__file__).resolve().parent.parent
    pins = json.loads((root / 'versions.json').read_text())
    env = dict(os.environ, GOWORK='off', GOFLAGS='-mod=readonly')
    graph = subprocess.check_output(['go', 'mod', 'graph'], cwd=root, env=env, text=True, timeout=120)
    check_graph(graph)
    raw = subprocess.check_output(['go', 'list', '-m', '-json', 'all'], cwd=root, env=env, text=True, timeout=120)
    check_modules(objects(raw), pins)
    raw = subprocess.check_output(['go', 'list', '-deps', '-test', '-tags=integration', '-json', './...'],
                                  cwd=root, env=env, text=True, timeout=120)
    check_packages(objects(raw))
    print('Independent blackbox tests and exact SDK/protocol release dependencies verified')


if __name__ == '__main__':
    main()
