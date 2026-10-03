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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--offline', action='store_true', help='require an already prepared module cache')
    args = parser.parse_args()
    pins = json.loads((ROOT / 'versions.json').read_text())
    output = ROOT / '.tools' / ('weir.exe' if os.name == 'nt' else 'weir')
    receipt_path = output.with_suffix('.receipt.json')
    if output.exists():
        receipt = json.loads(receipt_path.read_text())
        digest = hashlib.sha256(output.read_bytes()).hexdigest()
        if receipt['source'] != pins['weir_source'] or receipt['sha256'] != digest or receipt['go'] != pins['go']:
            raise ValueError('existing binary does not match its lock and receipt; preserving it')
        print(output)
        return
    env = dict(os.environ, GOENV='off', GOWORK='off', GOTOOLCHAIN='local', GOFLAGS='-mod=readonly', CGO_ENABLED='0')
    env.pop('GOROOT', None)
    if args.offline:
        env.update(GOPROXY='off', GOSUMDB='off')
    actual = subprocess.check_output(['go', 'version'], env=env, text=True, timeout=10).strip()
    if not actual.startswith('go version go' + pins['go'] + ' '):
        raise ValueError('requires the locked native Go ' + pins['go'] + ' toolchain')
    with tempfile.TemporaryDirectory(prefix='weir-source-') as temporary:
        directory = Path(temporary)
        (directory / 'go.mod').write_text('module local.invalid/weir-build\n\ngo ' + pins['go'] + '\n')
        query = 'github.com/batchstream/weir@' + pins['weir_version']
        raw = subprocess.check_output(['go', 'mod', 'download', '-json', query], cwd=directory, env=env, text=True, timeout=180)
        module = json.loads(raw)
        validate_source(pins, module)
        source = Path(module['Dir'])
        subprocess.run(['go', 'mod', 'download'], cwd=source, env=env, check=True, timeout=180)
        subprocess.run(['go', 'mod', 'verify'], cwd=source, env=env, check=True, timeout=180)
        binary = directory / output.name
        ldflags = '-X main.sourceRevision=' + pins['weir_source']
        subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-ldflags', ldflags,
                        '-o', str(binary), './cmd/weir'], cwd=source, env=env, check=True, timeout=600)
        identity = json.loads(subprocess.check_output([str(binary), 'version'], text=True, timeout=10))
        if identity['revision'] != pins['weir_source'] or identity['go'] != 'go' + pins['go']:
            raise ValueError('built binary reports a different source or toolchain')
        receipt = dict(source=pins['weir_source'], version=pins['weir_version'], go=pins['go'],
                       sha256=hashlib.sha256(binary.read_bytes()).hexdigest(), identity=identity)
        output.parent.mkdir(parents=True, exist_ok=True)
        # Keep construction and verification outside the final output directory.
        output.write_bytes(binary.read_bytes())
        output.chmod(0o755)
        receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
    print(output)


if __name__ == '__main__':
    main()
