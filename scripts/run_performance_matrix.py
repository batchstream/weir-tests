#!/usr/bin/env python3
"""Run serialized, verified ordinary/Lua benchmarks with retained source and fixture evidence."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

from build_local_server import freeze_source
from performance_evidence import validate_report_provenance, validate_server_receipt


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--backend', choices=('mongo', 'search'), required=True)
    parser.add_argument('--weir', required=True, type=Path)
    parser.add_argument('--server-receipt', type=Path)
    parser.add_argument('--mongod', type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--bulk', action='store_true', help='append read and write bulk32 comparisons')
    parser.add_argument('--batch-limit', type=int, default=0, help='0 keeps the server default')
    parser.add_argument('--warmup-duration', help='default: Search 20s, native Mongo 2s per path/stage')
    parser.add_argument('--duration', help='default measurement: Search 20s, native Mongo 5s per path/stage')
    args = parser.parse_args()
    warmup = args.warmup_duration or ('20s' if args.backend == 'search' else '2s')
    duration = args.duration or ('20s' if args.backend == 'search' else '5s')
    root = Path(__file__).resolve().parent.parent
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    client = output / 'performance-lab'
    weir = args.weir.resolve()
    versions = json.loads((root / 'versions.json').read_text())
    server_sha256 = hashlib.sha256(weir.read_bytes()).hexdigest()
    server_receipt_path = args.server_receipt.resolve() if args.server_receipt else weir.with_suffix('.receipt.json')
    server_receipt = json.loads(server_receipt_path.read_text())
    receipt_kind = validate_server_receipt(server_receipt, server_sha256, versions)
    archived_receipt = output / 'server.receipt.json'
    archived_receipt.write_text(json.dumps(server_receipt, indent=2) + '\n')
    source_sha256, source_files = freeze_source(root, output / 'client-source')
    build_env = dict(os.environ, GOENV='off', GOWORK='off', GOFLAGS='-mod=readonly')
    subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-o', str(client), './cmd/weir-lab'],
                   cwd=output / 'client-source', env=build_env, check=True, timeout=600)
    provenance = dict(client_source_tree_sha256=source_sha256, client_source_files=source_files,
                      client_binary_sha256=hashlib.sha256(client.read_bytes()).hexdigest(),
                      server_binary_sha256=server_sha256,
                      test_source=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip(),
                      test_source_dirty=bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=root)),
                      versions=versions, server_receipt=server_receipt, server_receipt_kind=receipt_kind)
    if args.mongod:
        mongod = args.mongod.resolve()
        provenance['native_mongodb_binary_sha256'] = hashlib.sha256(mongod.read_bytes()).hexdigest()
        mongo_receipt = mongod.with_suffix('.receipt.json')
        if mongo_receipt.exists():
            provenance['native_mongodb_receipt'] = json.loads(mongo_receipt.read_text())
            shutil.copy2(mongo_receipt, output / 'mongod.receipt.json')
    cases = [('read', 0, False, 'saturation'), ('mixed', 10, False, 'saturation'),
             ('write', 100, False, 'saturation'), ('lua', 100, True, 'saturation')]
    if args.bulk:
        cases += [('bulk-read', 0, False, 'bulk-saturation'), ('bulk-write', 100, False, 'bulk-saturation')]
    for name, writes, lua, mode in cases:
        directory = output / name
        command = [str(client), '-mode', mode, '-weir', str(weir), '-backend', args.backend,
                   '-client-processes', '4', '-concurrency-levels', '8,32',
                   '-batch-sizes', '32' if mode == 'bulk-saturation' else '1',
                   '-records', '1024', '-payload-bytes', '1024', '-rounds', '3',
                   '-warmup-duration', warmup, '-duration', duration, '-database-cpus', '2',
                   '-write-percent', str(writes), '-backend-batch-limit', str(args.batch_limit),
                   '-output', str(directory)]
        if args.mongod:
            command += ['-mongod', str(args.mongod.resolve())]
        if receipt_kind == 'local':
            command += ['-server-receipt', str(archived_receipt)]
        if lua:
            command.append('-lua-mutations')
        concurrency = 'total 8/32; four OS processes each have 2/8 single-record workers'
        if mode == 'bulk-saturation':
            concurrency = 'one client OS process with 8/32 workers; 32 records per call'
        receipt = dict(provenance, command=command, case=name, concurrency=concurrency)
        receipt_path = output / (name + '.receipt.json')
        receipt['server_binary_before_sha256'] = hashlib.sha256(weir.read_bytes()).hexdigest()
        if receipt['server_binary_before_sha256'] != server_sha256:
            receipt['validation_error'] = 'server binary changed before ' + name
            receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
            raise ValueError(receipt['validation_error'])
        receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
        print('Starting ' + name, flush=True)
        with (output / (name + '.log')).open('w') as log:
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT)
        receipt['exit_code'] = result.returncode
        receipt['server_binary_after_sha256'] = hashlib.sha256(weir.read_bytes()).hexdigest()
        if receipt['server_binary_after_sha256'] != server_sha256:
            receipt['validation_error'] = 'server binary changed during ' + name
            receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
            raise ValueError(receipt['validation_error'])
        report_path = directory / (args.backend + '.json')
        if report_path.exists():
            report = json.loads(report_path.read_text())
            try:
                validate_report_provenance(report, receipt)
            except ValueError as error:
                receipt['validation_error'] = str(error)
                receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
                raise
            fixture = Path(report['provenance']['fixture_logs'])
            evidence = output / (name + '-fixture')
            evidence.mkdir()
            for path in fixture.iterdir():
                if path.is_file() and (path.suffix in ('.json', '.log', '.txt')):
                    shutil.copy2(path, evidence / path.name)
            receipt['fixture_evidence'] = str(evidence)
        receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
        print('Finished ' + name + ': exit ' + str(result.returncode), flush=True)
        if result.returncode:
            raise SystemExit(result.returncode)


if __name__ == '__main__':
    main()
