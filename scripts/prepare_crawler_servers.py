#!/usr/bin/env python3
"""Build two exact public Weir commits without changing the test dependency lock."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


def prepare(source, output):
    if re.fullmatch(r'[0-9a-f]{40}', source) is None:
        raise ValueError('source must be a full immutable commit')
    receipt_path = output.with_suffix('.receipt.json')
    if output.exists() or receipt_path.exists():
        raise ValueError('preserving existing binary and receipt; choose a new output')
    environment = dict(os.environ, GOENV='off', GOWORK='off', GOFLAGS='-mod=readonly',
                       CGO_ENABLED='0', GIT_CONFIG_GLOBAL='/dev/null',
                       GIT_CONFIG_NOSYSTEM='1', GIT_TERMINAL_PROMPT='0')
    environment.pop('GOROOT', None)
    with tempfile.TemporaryDirectory(prefix='weir-crawler-source-') as temporary:
        directory = Path(temporary)
        subprocess.run(['git', 'init', '-q', str(directory)], check=True, env=environment)
        subprocess.run(['git', '-c', 'credential.helper=', '-C', str(directory), 'fetch',
                        '--depth=1', 'https://github.com/batchstream/weir.git', source],
                       check=True, env=environment, timeout=180)
        subprocess.run(['git', '-C', str(directory), 'checkout', '-q', '--detach', 'FETCH_HEAD'],
                       check=True, env=environment)
        actual = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=directory,
                                         env=environment, text=True).strip()
        if actual != source or subprocess.check_output(['git', 'status', '--porcelain'], cwd=directory, env=environment):
            raise ValueError('immutable server source is dirty or mismatched')
        subprocess.run(['go', 'mod', 'download'], cwd=directory, env=environment, check=True, timeout=180)
        subprocess.run(['go', 'mod', 'verify'], cwd=directory, env=environment, check=True, timeout=180)
        binary = directory / 'weir-benchmark'
        subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-ldflags',
                        '-X main.sourceRevision=' + source, '-o', str(binary), './cmd/weir'],
                       cwd=directory, env=environment, check=True, timeout=600)
        identity = json.loads(subprocess.check_output([str(binary), 'version'], text=True, env=environment))
        if identity['revision'] != source:
            raise ValueError('server version identity does not match immutable source')
        data = binary.read_bytes()
        receipt = dict(source=source, source_dirty=False, identity=identity,
                       sha256=hashlib.sha256(data).hexdigest(),
                       go=subprocess.check_output(['go', 'version'], text=True, env=environment).strip(),
                       build=subprocess.check_output(['go', 'version', '-m', str(binary)], text=True, env=environment))
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_bytes(data)
        output.chmod(0o755)
        receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--current', required=True)
    parser.add_argument('--optimized', required=True)
    parser.add_argument('--output', type=Path, default=Path('.tools'))
    arguments = parser.parse_args()
    prepare(arguments.current, (arguments.output / 'weir-current').resolve())
    prepare(arguments.optimized, (arguments.output / 'weir-optimized').resolve())


if __name__ == '__main__':
    main()
