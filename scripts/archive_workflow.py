#!/usr/bin/env python3
"""Retain every regular workflow artifact with deterministic gzip and two hashes."""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile


def forbidden_path(path):
    for part in path.parts:
        name = part.lower()
        if name.startswith('.env') or name in ('.ssh', 'credentials') or name.endswith(('.pem', '.key')):
            return True
    return False


def artifact_files(directory):
    files = []
    for path in sorted(directory.rglob('*')):
        if path.is_symlink():
            raise ValueError('artifact symlink refused before reading')
        if forbidden_path(path.relative_to(directory)):
            raise ValueError('sensitive artifact path refused before reading')
        if path.is_file():
            files.append(path)
    return files


def output_text(value):
    return value.decode(errors='replace') if isinstance(value, bytes) else (value or '')


def archive(options):
    if re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', options.repo) is None or re.fullmatch('[0-9a-f]{40}', options.source) is None or type(options.run) is not int or options.run < 1:
        raise ValueError('invalid repository or immutable source')
    if options.output.exists() or options.output.is_symlink():
        raise FileExistsError('preserving existing archive output')
    if forbidden_path(options.output):
        raise ValueError('sensitive archive output path refused')
    fields = 'databaseId,headSha,headBranch,status,conclusion,url,createdAt,updatedAt,event,workflowName'
    command = ['gh', 'run', 'view', str(options.run), '--repo', options.repo, '--json', fields]
    metadata = json.loads(subprocess.check_output(command))
    if metadata['status'] != 'completed':
        raise ValueError('workflow is not complete')
    if metadata['headSha'] != options.source:
        raise ValueError('workflow source differs from the declared immutable client')
    expected_url = 'https://github.com/' + options.repo + '/actions/runs/' + str(options.run)
    if metadata['databaseId'] != options.run or metadata['url'].lower() != expected_url.lower() or not isinstance(metadata['conclusion'], str) or not metadata['conclusion']:
        raise ValueError('workflow identity or completed conclusion differs from the declared run')
    temporary = Path(tempfile.mkdtemp(prefix='weir-retained-workflow-'))
    (temporary / 'workflow.json').write_text(json.dumps(metadata, indent=2) + '\n')
    command = ['gh', 'run', 'download', str(options.run), '--repo', options.repo,
               '--dir', str(temporary / 'artifacts')]
    download = dict(command=command, completed=False, exit_code=None)
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=300)
        download.update(completed=result.returncode == 0, exit_code=result.returncode,
                        stdout=result.stdout, stderr=result.stderr)
    except subprocess.TimeoutExpired as error:
        download.update(error='artifact download exceeded 300 seconds',
                        stdout=output_text(error.stdout), stderr=output_text(error.stderr))
    except OSError as error:
        download['error'] = str(error)
    (temporary / 'download.json').write_text(json.dumps(download, indent=2) + '\n')
    sources = artifact_files(temporary)
    options.output.mkdir(parents=True, exist_ok=False)
    inventory = []
    for source in sources:
        relative = source.relative_to(temporary).as_posix() + '.gz'
        target = options.output / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        checksum, length = hashlib.sha256(), 0
        with source.open('rb') as incoming, target.open('xb') as destination:
            with gzip.GzipFile(filename='', fileobj=destination, mode='wb', mtime=0, compresslevel=6) as packed:
                while chunk := incoming.read(1 << 20):
                    checksum.update(chunk)
                    length += len(chunk)
                    packed.write(chunk)
        with target.open('rb') as stream:
            compressed = hashlib.file_digest(stream, 'sha256').hexdigest()
        item = dict(path=relative, gzip_sha256=compressed, sha256=checksum.hexdigest(), bytes=length)
        inventory.append(item)
    (options.output / 'files.json').write_text(json.dumps(inventory, indent=2) + '\n')
    summary = dict(run=options.run, source=options.source, conclusion=metadata['conclusion'],
                   artifact_download_complete=download['completed'], files=len(inventory))
    return summary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', default='batchstream/weir-tests')
    parser.add_argument('--run', type=int, required=True)
    parser.add_argument('--source', required=True)
    parser.add_argument('--output', type=Path, required=True)
    options = parser.parse_args()
    summary = archive(options)
    print(json.dumps(summary))
    if not summary['artifact_download_complete']:
        raise SystemExit('Artifact download failed or timed out; workflow, download receipt and partial files were retained')


if __name__ == '__main__':
    main()
