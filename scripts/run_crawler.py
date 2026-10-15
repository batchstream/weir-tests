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


def workload_controls(environ):
    profile = environ['CRAWLER_PROFILES']
    partial = profile == 'partial-update'
    workers = max(map(int, environ['CRAWLER_LEVELS'].split(',')))
    page_size = int(environ['CRAWLER_PAGE_SIZE'])
    controls = dict(
        profiles=profile,
        logical_unit='single_partial_update' if partial else 'completed_business_product',
        seeded_documents=workers * page_size * 6 + 1,
        full_field_oracle='Every seeded product, seller and shared seller, including untouched and rejected keys',
        native_conflict_retries=0 if partial else 3,
        weir_backend_expression_conflict_retries=0 if partial else None,
        weir_lua_total_attempts=None if partial else 4,
        visibility=environ.get('CRAWLER_VISIBILITY', 'query_visible'),
        seed_padding_bytes=int(environ.get('CRAWLER_SEED_PADDING_BYTES', '0')),
        current_batch_max_operations=32,
        current_batch_max_bytes=8 << 20,
        current_batch_max_in_flight=0,
        optimized_batch_max_operations=int(environ.get('CRAWLER_BATCH_MAX_OPERATIONS', '32')),
        optimized_batch_max_bytes=int(environ.get('CRAWLER_BATCH_MAX_BYTES', str(8 << 20))),
        optimized_batch_max_in_flight=int(environ.get('CRAWLER_MAX_IN_FLIGHT', '0')),
        current_batch_max_wait='0s',
        optimized_batch_max_wait=environ.get('CRAWLER_BATCH_MAX_WAIT', '0s'),
        search_phase_metrics=dict(
            http_duration='client.Do through response body read and validation; bulk includes refresh=wait_for and is not pure indexing time',
            request_records='Attempted framed records at the client.Do boundary for mget/bulk; not acknowledged writes or proof of network delivery',
            request_body_bytes='Actual framed HTTP request body bytes at client.Do; attempts, not acknowledged writes',
            lua_duration='luaengine.Evaluate only; excludes document encoding/decoding and HTTP',
            availability='Missing before/after series and reset counters remain unavailable, never inferred as zero',
        ),
    )
    return controls


def main():
    binary = Path('.tools/weir-crawler')
    servers = [Path('.tools/weir-current'), Path('.tools/weir-optimized')]
    source = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    if subprocess.check_output(['git', 'status', '--porcelain']):
        raise ValueError('benchmark source checkout must be clean')
    modes = os.environ['CRAWLER_MODES']
    if modes == 'both':
        modes = 'fixed-rate,saturation'
    command = [str(binary), '-input-shape', os.environ['CRAWLER_INPUT_SHAPE'], '-http-observer', os.environ['CRAWLER_HTTP_OBSERVER'], '-modes', modes, '-profiles', os.environ['CRAWLER_PROFILES'], '-after-max-in-flight', os.environ['CRAWLER_MAX_IN_FLIGHT'], '-after-batch-max-wait', os.environ.get('CRAWLER_BATCH_MAX_WAIT', '0s'), '-rates', os.environ['CRAWLER_RATES'],
               '-visibility', os.environ.get('CRAWLER_VISIBILITY', 'query_visible'),
               '-seed-padding-bytes', os.environ.get('CRAWLER_SEED_PADDING_BYTES', '0'),
               '-after-batch-max-operations', os.environ.get('CRAWLER_BATCH_MAX_OPERATIONS', '32'),
               '-after-batch-max-bytes', os.environ.get('CRAWLER_BATCH_MAX_BYTES', str(8 << 20)),
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
    receipt = dict(workload_controls=workload_controls(os.environ),input_shape=os.environ['CRAWLER_INPUT_SHAPE'],http_observer=os.environ['CRAWLER_HTTP_OBSERVER'],host_cpu_model=cpu_model,host_memory_bytes=memory_bytes,source=source, source_dirty=False, command=command,
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
