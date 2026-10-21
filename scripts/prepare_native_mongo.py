#!/usr/bin/env python3
"""Prepare an explicitly selected, checksum-locked native MongoDB fixture."""
import hashlib
import json
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request


MAX_ARCHIVE_BYTES = 512 << 20
MAX_BINARY_BYTES = 512 << 20


def select_profile(pins):
    arch = {'arm64': 'arm64', 'x86_64': 'amd64'}.get(platform.machine())
    system = platform.system()
    if system == 'Darwin' and arch is not None:
        target = 'darwin/' + arch
        prefix = 'mongodb_darwin_' + arch
        profile = dict(target=target, version=pins['mongodb_version'],
                       url=pins[prefix + '_url'], sha256=pins[prefix + '_sha256'])
        return profile
    if system == 'Linux' and arch == 'amd64':
        release = platform.freedesktop_os_release()
        if release.get('ID') == 'ubuntu' and release.get('VERSION_ID') == '24.04':
            profile = dict(pins['mongodb_linux_amd64'], target='linux/amd64')
            if profile['version'] != pins['mongodb_version']:
                raise ValueError('native Linux fixture version differs from MongoDB lock')
            return profile
    raise ValueError('native preparation supports macOS arm64/amd64 or Ubuntu 24.04 amd64 only')


def copy_bounded(source, destination, limit):
    total = 0
    while True:
        block = source.read(min(1 << 20, limit - total + 1))
        if not block:
            return total
        total += len(block)
        if total > limit:
            raise ValueError('native MongoDB artifact exceeds its size bound')
        destination.write(block)


def digest(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def verify_archive(path, profile):
    size = path.stat().st_size
    if size > MAX_ARCHIVE_BYTES or ('size' in profile and size != profile['size']):
        raise ValueError('native MongoDB archive size differs from lock')
    if digest(path) != profile['sha256']:
        raise ValueError('native MongoDB archive checksum mismatch')


def extract_binary(archive, binary, profile):
    with tarfile.open(archive, 'r:gz') as tar:
        selected = None
        total = 0
        for index, member in enumerate(tar):
            total += member.size
            if index >= 10000 or total > 1 << 30:
                raise ValueError('native MongoDB archive exceeds member bounds')
            matches = (member.name == profile['member'] if 'member' in profile
                       else member.name.endswith('/bin/mongod'))
            if not matches:
                continue
            if selected is not None or not member.isfile() or member.size > MAX_BINARY_BYTES:
                raise ValueError('archive must contain exactly one bounded regular mongod binary')
            if 'member_size' in profile and member.size != profile['member_size']:
                raise ValueError('native MongoDB binary size differs from lock')
            selected = member
        if selected is None:
            raise ValueError('native MongoDB archive lacks its locked binary')
        # Copy only verified regular-file content, never archive paths or links.
        with tar.extractfile(selected) as source, binary.open('wb') as destination:
            count = copy_bounded(source, destination, selected.size)
        if count != selected.size:
            raise ValueError('native MongoDB binary is truncated')
    if 'member_sha256' in profile and digest(binary) != profile['member_sha256']:
        raise ValueError('native MongoDB binary checksum mismatch')


def prepare(root):
    pins = json.loads((root / 'versions.json').read_text())
    profile = select_profile(pins)
    output = root / '.tools' / 'mongod'
    metadata_path = output.with_suffix('.json')
    if output.exists():
        metadata = json.loads(metadata_path.read_text())
        binary_hash = digest(output)
        if (metadata['archive_sha256'] != profile['sha256'] or
                metadata['binary_sha256'] != binary_hash or
                metadata['version'] != profile['version'] or
                metadata['target'] != profile['target'] or
                metadata['source'] != profile['url'] or
                ('member_sha256' in profile and binary_hash != profile['member_sha256'])):
            raise ValueError('existing native fixture does not match its lock/metadata; preserving it')
        return output
    output.parent.mkdir(parents=True, exist_ok=True)
    archive = output.parent / ('mongodb-' + profile['target'].replace('/', '-') + '.tgz')
    with tempfile.TemporaryDirectory(prefix='weir-native-mongo-') as temporary:
        directory = Path(temporary)
        if not archive.exists():
            download = directory / 'mongo.tgz'
            with urllib.request.urlopen(profile['url'], timeout=60) as response, download.open('wb') as file:
                copy_bounded(response, file, profile.get('size', MAX_ARCHIVE_BYTES))
            verify_archive(download, profile)
            shutil.copyfile(download, archive)
        verify_archive(archive, profile)
        binary = directory / 'mongod'
        extract_binary(archive, binary, profile)
        binary.chmod(0o755)
        version = json.loads(subprocess.check_output(
            [str(binary), '--version'], text=True, timeout=10).split('Build Info: ', 1)[-1])
        if version['version'] != profile['version']:
            raise ValueError('native MongoDB version differs from lock')
        metadata = dict(version=version['version'], source=profile['url'],
                        archive_sha256=profile['sha256'], binary_sha256=digest(binary),
                        target=profile['target'])
        shutil.copyfile(binary, output)
        output.chmod(0o755)
        metadata_path.write_text(json.dumps(metadata, indent=2) + '\n')
    return output


if __name__ == '__main__':
    print(prepare(Path(__file__).resolve().parent.parent))
