#!/usr/bin/env python3
"""Run a bounded, same-runner Search single-record write calibration or confirmation."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

from performance_evidence import validate_report_provenance, validate_server_receipt
from build_local_server import freeze_source


def planned_command(case, output):
    command = ['.tools/weir-lab', '-mode', 'saturation', '-backend', case['backend'],
               '-weir', '.tools/write-weir', '-server-receipt', '.tools/write-weir.receipt.json',
               '-client-processes', str(case['client_processes']),
               '-concurrency-levels', ','.join(map(str, case['levels'])), '-batch-sizes', '1',
               '-records', str(case['records']), '-payload-bytes', str(case['payload_bytes']),
               '-write-percent', str(case['write_percent']), '-database-cpus', str(case['database_cpus']),
               '-pending-records', '4096', '-rounds', str(case['rounds']),
               '-warmup-duration', case['warmup'], '-duration', case['duration'],
               '-capture-effective-config', '-output', str(output)]
    batch = case['batching']
    if batch is not None:
        command += ['-backend-batch-limit', str(batch['max_operations']),
                    '-backend-batch-bytes', str(batch['max_bytes']),
                    '-backend-batch-wait', batch['max_wait'],
                    '-backend-max-in-flight', str(batch['max_in_flight'])]
    if case.get('lua_mutations', False):
        command.append('-lua-mutations')
    return command


def cases(phase):
    if phase == 'calibration':
        return [
            ('immediate', 32, '0s', 0),
            ('bounded32', 32, '5ms', 16),
            ('bounded128', 128, '5ms', 16),
            ('bounded256', 256, '5ms', 16),
            ('longer-wait', 128, '20ms', 16),
            ('fewer-slots', 128, '5ms', 4),
        ]
    if phase == 'formal':
        return [('bounded128', 128, '5ms', 16)]
    raise ValueError('unknown write batching phase')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--phase', choices=('calibration', 'formal'), required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    output = root / 'results/local/write-batching'
    output.mkdir(parents=True, exist_ok=False)
    server = root / '.tools/write-weir'
    server_receipt_path = server.with_suffix('.receipt.json')
    server_receipt = json.loads(server_receipt_path.read_text())
    versions = json.loads((root / 'versions.json').read_text())
    server_hash = hashlib.sha256(server.read_bytes()).hexdigest()
    validate_server_receipt(server_receipt, server_hash, versions)
    if server_receipt['source_dirty']:
        raise ValueError('benchmark server source must be clean')
    client = root / '.tools/weir-lab'
    client_hash = hashlib.sha256(client.read_bytes()).hexdigest()
    source = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
    if subprocess.check_output(['git', 'status', '--porcelain'], cwd=root):
        raise ValueError('benchmark client source must be clean')
    client_tree, client_files = freeze_source(root, output / 'client-source')
    for name, operations, wait, inflight in cases(args.phase):
        directory = output / name
        calibration = args.phase == 'calibration'
        command = [str(client), '-mode', 'saturation', '-backend', 'search',
                   '-weir', str(server), '-server-receipt', str(server_receipt_path),
                   '-client-processes', '4', '-concurrency-levels', '128,512',
                   '-batch-sizes', '1', '-records', '8192', '-payload-bytes', '1024',
                   '-write-percent', '100', '-database-cpus', '1', '-pending-records', '4096',
                   '-backend-batch-limit', str(operations), '-backend-batch-bytes', str(8 << 20),
                   '-backend-batch-wait', wait, '-backend-max-in-flight', str(inflight),
                   '-rounds', '1' if calibration else '3',
                   '-warmup-duration', '2s' if calibration else '60s',
                   '-duration', '5s' if calibration else '60s', '-output', str(directory)]
        receipt = dict(phase=args.phase, case=name, command=command, test_source=source,
                       test_source_dirty=False, client_binary_sha256=client_hash,
                       client_source_tree_sha256=client_tree, client_source_files=client_files,
                       server_binary_sha256=server_hash, server_receipt=server_receipt,
                       versions=versions, workflow_run='https://github.com/' + os.environ['GITHUB_REPOSITORY']
                       + '/actions/runs/' + os.environ['GITHUB_RUN_ID'])
        receipt_path = output / (name + '.receipt.json')
        receipt['server_binary_before_sha256'] = hashlib.sha256(server.read_bytes()).hexdigest()
        if receipt['server_binary_before_sha256'] != server_hash:
            raise ValueError('benchmark server binary changed before measurement')
        receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
        print('Starting ' + name, flush=True)
        with (output / (name + '.log')).open('x') as log:
            process = subprocess.Popen(command, cwd=root, stdout=subprocess.PIPE,
                                       stderr=subprocess.STDOUT, text=True)
            for line in process.stdout:
                print(line, end='', flush=True)
                log.write(line)
            receipt['exit_code'] = process.wait()
        receipt['server_binary_after_sha256'] = hashlib.sha256(server.read_bytes()).hexdigest()
        receipt['client_binary_after_sha256'] = hashlib.sha256(client.read_bytes()).hexdigest()
        receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
        if receipt['server_binary_after_sha256'] != server_hash or receipt['client_binary_after_sha256'] != client_hash:
            raise ValueError('benchmark binary changed during measurement')
        if receipt['exit_code']:
            raise SystemExit(receipt['exit_code'])
        report = json.loads((directory / 'search.json').read_text())
        validate_report_provenance(report, receipt)
        expected = report.get('provenance', {})
        for key, value in dict(weir_batching_max_operations_override=str(operations),
                               weir_batching_max_bytes_override=str(8 << 20),
                               weir_batching_max_wait_override='0s' if wait == '0s' else wait,
                               weir_batching_max_in_flight_override=str(inflight)).items():
            if expected.get(key) != value:
                raise ValueError('write report batch control mismatch: ' + key)
        if report.get('incomplete') or not report.get('pairs'):
            raise ValueError('incomplete write benchmark')


if __name__ == '__main__':
    main()
