#!/usr/bin/env python3
"""Build the locked Weir source separately from the test module's dependency graph."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import sys

# Never import helpers from a downloaded checkout, including under Python -I.
sys.path.insert(0, str(Path(__file__).resolve().parent))
from server_source import (build_environment, check_checkout, checkout_source, freeze_server_source,
                           native_toolchain, validate_identity, verify_snapshot)


ROOT = Path(__file__).resolve().parent.parent


def validate_source(pins, module):
    revision = pins['weir_source']
    if re.fullmatch(r'[0-9a-f]{40}', revision) is None:
        raise ValueError('Weir source must be a full immutable commit')
    if module.get('Path') != 'github.com/batchstream/weir' or module.get('Version') != pins['weir_version']:
        raise ValueError('downloaded Weir module does not match the locked version')
    if module.get('Origin', {}).get('Hash') != revision or module.get('Sum') != pins['weir_sum']:
        raise ValueError('downloaded Weir source or checksum does not match the lock')
    if module.get('Replace') or module.get('Error'):
        raise ValueError('Weir source must be an unreplaced verified module')


def verify_existing(output, pins, env, toolchain):
    receipt = json.loads(output.with_suffix('.receipt.json').read_text())
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    expected = dict(source=pins['weir_source'], version=pins['weir_version'], go=pins['go'],
                    sha256=digest, source_kind='verified full git checkout', source_dirty=False)
    if any(receipt.get(key) != value for key, value in expected.items()):
        raise ValueError('existing binary does not match its lock and receipt; preserving it')
    if re.fullmatch(r'[0-9a-f]{64}', receipt.get('source_tree_sha256', '')) is None:
        raise ValueError('existing binary lacks complete-source provenance; preserving it')
    identity = json.loads(subprocess.check_output([str(output), 'version'], env=env, text=True, timeout=10))
    validate_identity(identity, toolchain, pins['weir_source'])
    if receipt.get('identity') != identity:
        raise ValueError('existing binary identity differs from its receipt; preserving it')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--offline', action='store_true',
                        help='disable Go downloads (new preparation still requires public fixed-SHA Git access)')
    args = parser.parse_args()
    pins = json.loads((ROOT / 'versions.json').read_text())
    if re.fullmatch(r'[0-9a-f]{40}', pins['weir_source']) is None:
        raise ValueError('Weir source must be a full immutable commit')
    output = ROOT / '.tools' / ('weir.exe' if os.name == 'nt' else 'weir')
    receipt_path = output.with_suffix('.receipt.json')
    env = build_environment()
    if args.offline:
        env.update(GOPROXY='off', GOSUMDB='off')
    toolchain = native_toolchain(env, pins['go'])
    if output.is_symlink() or receipt_path.is_symlink():
        raise ValueError('binary and receipt must not be symlinks')
    if output.exists():
        verify_existing(output, pins, env, toolchain)
        print(output)
        return
    if receipt_path.exists():
        raise ValueError('preserving orphaned receipt; choose a new output')
    with tempfile.TemporaryDirectory(prefix='weir-source-') as temporary:
        directory = Path(temporary)
        metadata = directory / 'metadata'
        metadata.mkdir()
        (metadata / 'go.mod').write_text('module local.invalid/weir-build\n\ngo ' + pins['go'] + '\n')
        query = 'github.com/batchstream/weir@' + pins['weir_version']
        raw = subprocess.check_output(['go', 'mod', 'download', '-json', query], cwd=metadata,
                                      env=env, text=True, timeout=180)
        validate_source(pins, json.loads(raw))
        source = directory / 'checkout'
        checkout_source(source, pins['weir_source'], env)
        snapshot = directory / 'snapshot'
        tree_sha256, manifest = freeze_server_source(source, snapshot, env)
        subprocess.run(['go', 'mod', 'download'], cwd=snapshot, env=env, check=True, timeout=180)
        subprocess.run(['go', 'mod', 'verify'], cwd=snapshot, env=env, check=True, timeout=180)
        binary = directory / output.name
        ldflags = '-X main.sourceRevision=' + pins['weir_source']
        subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-ldflags', ldflags,
                        '-o', str(binary), './cmd/weir'], cwd=snapshot, env=env, check=True, timeout=600)
        identity = json.loads(subprocess.check_output([str(binary), 'version'], env=env, text=True, timeout=10))
        validate_identity(identity, toolchain, pins['weir_source'])
        verify_snapshot(snapshot, manifest, tree_sha256)
        check_checkout(source, pins['weir_source'], env)
        data = binary.read_bytes()
        receipt = dict(source=pins['weir_source'], version=pins['weir_version'], go=pins['go'],
                       sha256=hashlib.sha256(data).hexdigest(), identity=identity,
                       source_kind='verified full git checkout', source_dirty=False,
                       source_tree_sha256=tree_sha256, source_files=manifest)
        output.parent.mkdir(parents=True, exist_ok=True)
        # Keep construction and verification outside the final output directory.
        output.write_bytes(data)
        output.chmod(0o755)
        receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
    print(output)


if __name__ == '__main__':
    main()
