#!/usr/bin/env python3
"""Prepare the checksum-locked native macOS MongoDB fixture for affected Docker kernels."""
import hashlib
import json
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request


root = Path(__file__).resolve().parent.parent
pins = json.loads((root / 'versions.json').read_text())
arch = {'arm64': 'arm64', 'x86_64': 'amd64'}.get(platform.machine())
if platform.system() != 'Darwin' or arch is None:
    raise SystemExit('native preparation supports macOS arm64/amd64; use the locked Docker fixture on supported Linux kernels')
prefix = 'mongodb_darwin_' + arch
url = pins[prefix + '_url']
expected = pins[prefix + '_sha256']
output = root / '.tools' / 'mongod'
receipt_path = output.with_suffix('.receipt.json')
if output.exists():
    receipt = json.loads(receipt_path.read_text())
    if receipt['archive_sha256'] != expected or receipt['binary_sha256'] != hashlib.sha256(output.read_bytes()).hexdigest():
        raise ValueError('existing native fixture does not match its receipt; preserving it')
    print(output)
    raise SystemExit(0)
output.parent.mkdir(parents=True, exist_ok=True)
archive = output.parent / ('mongodb-darwin-' + arch + '.tgz')
with tempfile.TemporaryDirectory(prefix='weir-native-mongo-') as temporary:
    directory = Path(temporary)
    if not archive.exists():
        candidate = directory / 'mongo.tgz'
        with urllib.request.urlopen(url, timeout=60) as response, candidate.open('wb') as file:
            shutil.copyfileobj(response, file)
        if hashlib.sha256(candidate.read_bytes()).hexdigest() != expected:
            raise ValueError('official MongoDB archive checksum mismatch')
        shutil.copyfile(candidate, archive)
    if hashlib.sha256(archive.read_bytes()).hexdigest() != expected:
        raise ValueError('cached MongoDB archive does not match its lock')
    with tarfile.open(archive, 'r:gz') as tar:
        members = [member for member in tar.getmembers() if member.name.endswith('/bin/mongod')]
        if len(members) != 1 or not members[0].isfile() or members[0].size > 512 << 20:
            raise ValueError('archive must contain exactly one bounded regular mongod binary')
        # Extract only that regular binary, never archive paths, links or other files.
        binary = directory / 'mongod'
        with tar.extractfile(members[0]) as source, binary.open('wb') as file:
            shutil.copyfileobj(source, file)
    binary.chmod(0o755)
    version = json.loads(subprocess.check_output([str(binary), '--version'], text=True, timeout=10).split('Build Info: ', 1)[-1])
    if version['version'] != pins['mongodb_version']:
        raise ValueError('native MongoDB version differs from lock')
    receipt = dict(version=version['version'], source=url, archive_sha256=expected,
                   binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(), target='darwin/' + arch)
    shutil.copyfile(binary, output)
    output.chmod(0o755)
    receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
print(output)
