#!/usr/bin/env python3
"""Record and execute exactly one bounded three-path crawler experiment."""
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess


def digest(binary):
    return hashlib.sha256(binary.read_bytes()).hexdigest()


def main():
    binary = Path('.tools/weir-crawler')
    servers = [Path('.tools/weir-current'), Path('.tools/weir-optimized')]
    source = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    if subprocess.check_output(['git', 'status', '--porcelain']):
        raise ValueError('benchmark source checkout must be clean')
    command = [str(binary), '-profiles', os.environ['CRAWLER_PROFILES'], '-after-max-in-flight', os.environ['CRAWLER_MAX_IN_FLIGHT'], '-rates', os.environ['CRAWLER_RATES'],
               '-concurrency-levels', os.environ['CRAWLER_LEVELS'],
               '-page-size', os.environ['CRAWLER_PAGE_SIZE'],
               '-duration', os.environ['CRAWLER_DURATION'],
               '-warmup', os.environ['CRAWLER_WARMUP'],
               '-rounds', os.environ['CRAWLER_ROUNDS'],
               '-database-cpus', '1', '-client-processes', '4',
               '-queue-pages', '1', '-timeout', '10s',
               '-output', 'results/local/crawler.json']
    receipts = [json.loads(server.with_suffix('.receipt.json').read_text()) for server in servers]
    hashes = [digest(server) for server in servers]
    for index, receipt in enumerate(receipts):
        if receipt['sha256'] != hashes[index] or receipt['source_dirty']:
            raise ValueError('server binary changed or source is dirty')
    cpu_model = next(line.split(':', 1)[1].strip() for line in Path('/proc/cpuinfo').read_text().splitlines() if line.startswith('model name'))
    memory_bytes = int(next(line for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemTotal:')).split()[1]) * 1024
    receipt = dict(host_cpu_model=cpu_model,host_memory_bytes=memory_bytes,source=source, source_dirty=False, command=command,
                   client_binary_sha256=digest(binary), servers=receipts,
                   host=platform.uname()._asdict(), host_cpus=os.cpu_count(),
                   go=subprocess.check_output(['go', 'version'], text=True).strip(),
                   client_build=subprocess.check_output(['go', 'version', '-m', str(binary)], text=True),
                   versions=json.loads(Path('versions.json').read_text()),
                   workflow_run='https://github.com/' + os.environ['GITHUB_REPOSITORY'] + '/actions/runs/' + os.environ['GITHUB_RUN_ID'])
    receipt_path = Path('results/local/crawler.receipt.json')
    receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
    with Path('results/local/crawler.log').open('x') as log:
        process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        for line in process.stdout:
            print(line, end='', flush=True)
            log.write(line)
        receipt['exit_code'] = process.wait()
    receipt['client_binary_after_sha256'] = digest(binary)
    receipt['server_binary_after_sha256'] = [digest(server) for server in servers]
    receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
    if receipt['client_binary_after_sha256'] != receipt['client_binary_sha256'] or receipt['server_binary_after_sha256'] != hashes:
        raise ValueError('a benchmark executable changed during measurement')
    raise SystemExit(receipt['exit_code'])


if __name__ == '__main__':
    main()
