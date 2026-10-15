#!/usr/bin/env python3
"""Execute one committed finite tuning case set through the existing benchmark engines."""
import argparse
from decimal import Decimal
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess

from build_local_server import freeze_source
from performance_evidence import validate_report_provenance, validate_server_receipt
from run_crawler import planned_command as crawler_command
from run_write_batching import planned_command as write_command


def duration_ns(value):
    if not isinstance(value, str):
        raise ValueError('duration must be a Go duration string')
    match = re.fullmatch(r'(\d+(?:\.\d+)?)(ns|us|ms|s|m)', value)
    if match is None:
        raise ValueError('duration requires one explicit ns/us/ms/s/m unit')
    factors = dict(ns=1, us=1000, ms=1000000, s=1000000000, m=60000000000)
    number = Decimal(match[1]) * factors[match[2]]
    if number != int(number):
        raise ValueError('duration has fractional nanoseconds')
    return int(number)


def integer(value, low, high, field):
    if type(value) is not int or not low <= value <= high:
        raise ValueError(field + ': integer outside bounded plan range')


def validate_batching(batch):
    fields = {'max_operations', 'max_bytes', 'max_wait', 'max_in_flight'}
    if not isinstance(batch, dict) or set(batch) != fields:
        raise ValueError('batching must declare exactly all four controls')
    integer(batch['max_operations'], 1, 1024, 'max_operations')
    integer(batch['max_bytes'], 1, 32 << 20, 'max_bytes')
    integer(batch['max_in_flight'], 0, 4096, 'max_in_flight')
    if not 0 <= duration_ns(batch['max_wait']) <= 1000000000:
        raise ValueError('max_wait exceeds one second')


def validate_case(case):
    common = {'id', 'kind', 'batching', 'expected_batching', 'warmup', 'duration', 'rounds',
              'database_cpus', 'client_processes', 'description'}
    write = {'backend', 'payload_bytes', 'records', 'levels', 'write_percent', 'lua_mutations'}
    crawler = {'profile', 'rate', 'concurrency', 'paths', 'visibility', 'seed_padding_bytes', 'http_observer', 'page_size'}
    if not isinstance(case, dict) or case.get('kind') not in ('write', 'crawler'):
        raise ValueError('case kind must be write or crawler')
    if set(case) - common - (write if case['kind'] == 'write' else crawler):
        raise ValueError('unknown case fields: ' + ','.join(sorted(set(case) - common - write - crawler)))
    required = {'id', 'kind', 'batching', 'warmup', 'duration', 'rounds', 'database_cpus', 'client_processes'}
    if not required <= set(case):
        raise ValueError('missing required case controls')
    if not isinstance(case['id'], str) or re.fullmatch('[a-z0-9][a-z0-9-]{0,79}', case['id']) is None:
        raise ValueError('case id must be a safe lowercase identifier')
    validate_batching(case['batching'] if case['batching'] is not None else case.get('expected_batching'))
    if case['batching'] is not None and 'expected_batching' in case and case['expected_batching'] != case['batching']:
        raise ValueError('explicit batching and expected_batching must agree')
    max_warmup = 300 if case['kind'] == 'write' else 60
    if not 1000000000 <= duration_ns(case['warmup']) <= max_warmup * 1000000000:
        raise ValueError('warmup must be 1s through ' + str(max_warmup) + 's')
    if not 5000000000 <= duration_ns(case['duration']) <= 600000000000:
        raise ValueError('duration must be 5s through 10m')
    integer(case['rounds'], 1, 5, 'rounds')
    if type(case['database_cpus']) not in (int, float) or not 0 < case['database_cpus'] <= 8:
        raise ValueError('database_cpus must be positive and <=8')
    integer(case['client_processes'], 1, 8, 'client_processes')
    if case['kind'] == 'write':
        if not {'backend', 'payload_bytes', 'records', 'levels', 'write_percent'} <= set(case):
            raise ValueError('missing write controls')
        if case['backend'] not in ('search', 'mongo', 'all'):
            raise ValueError('unknown write backend')
        integer(case['payload_bytes'], 1, 1 << 20, 'payload_bytes')
        integer(case['records'], 1, 100000, 'records')
        integer(case['write_percent'], 0, 100, 'write_percent')
        if 'lua_mutations' in case and type(case['lua_mutations']) is not bool:
            raise ValueError('lua_mutations must be boolean')
        levels = case['levels']
        if not isinstance(levels, list) or not 1 <= len(levels) <= 12:
            raise ValueError('write levels require a finite ladder')
        for level in levels:
            integer(level, 1, 2048, 'level')
            if level % case['client_processes'] or level // case['client_processes'] > 1024:
                raise ValueError('levels must divide evenly into at most 1024 workers/process')
        if levels != sorted(set(levels)) or case['records'] < levels[-1]:
            raise ValueError('levels must strictly increase within the seeded record count')
        if case['records'] * (case['payload_bytes'] + 128) * 2 > 512 << 20:
            raise ValueError('precomputed document workspace exceeds 512 MiB')
        if case.get('lua_mutations', False) and (case['write_percent'] == 0 or case['payload_bytes'] > (256 << 10) - 128):
            raise ValueError('Lua mutation plan exceeds existing engine bounds')
        return len(levels) * case['rounds'] * 2 * (2 if case['backend'] == 'all' else 1)
    required = {'profile', 'rate', 'concurrency', 'paths', 'visibility', 'seed_padding_bytes', 'http_observer'}
    if not required <= set(case):
        raise ValueError('missing crawler controls')
    if case['profile'] not in ('partial-update', 'two-step', 'combined'):
        raise ValueError('unknown crawler profile')
    if case['visibility'] not in ('query_visible', 'application_acknowledged'):
        raise ValueError('unknown crawler completion policy')
    if case['visibility'] == 'application_acknowledged' and case['profile'] != 'partial-update':
        raise ValueError('application acknowledgement requires partial-update')
    integer(case['rate'], 1, 100000, 'rate')
    integer(case['concurrency'], 1, 2048, 'concurrency')
    if case['concurrency'] % case['client_processes'] or case['concurrency'] // case['client_processes'] > 512:
        raise ValueError('crawler concurrency must divide into at most 512 workers/process')
    integer(case['seed_padding_bytes'], 0, 1 << 20, 'seed_padding_bytes')
    if case['seed_padding_bytes'] and case['profile'] != 'partial-update':
        raise ValueError('seed padding requires partial-update')
    if case['concurrency'] * 2 * case['seed_padding_bytes'] > 256 << 20:
        raise ValueError('crawler seed padding workspace exceeds 256 MiB')
    if case.get('page_size', 1) != 1 or case['http_observer'] not in ('enabled', 'disabled'):
        raise ValueError('tuning requires single-record calls and an explicit observer mode')
    paths = case['paths']
    if not isinstance(paths, list) or not paths or any(path not in ('direct', 'current', 'optimized') for path in paths) or len(paths) != len(set(paths)):
        raise ValueError('crawler paths must contain distinct known paths')
    return len(paths) * case['rounds']


def load_plan(path, case_set):
    root = Path(__file__).resolve().parent.parent
    if re.fullmatch('[a-z0-9][a-z0-9-]{0,79}', case_set) is None:
        raise ValueError('case-set must be a safe lowercase identifier')
    original = path.absolute()
    if original.is_symlink() or any(parent.is_symlink() for parent in original.parents if parent != root and root in parent.parents):
        raise ValueError('plan must not traverse symbolic links')
    path = path.resolve()
    relative = path.relative_to(root)
    if path.is_symlink() or path.suffix != '.json' or any(part.startswith('.') or part == 'credentials' for part in relative.parts):
        raise ValueError('plan must be an ordinary non-hidden repository JSON file')
    subprocess.run(['git', 'ls-files', '--error-unmatch', str(relative)], cwd=root,
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    raw = path.read_bytes()
    committed = subprocess.check_output(['git', 'show', 'HEAD:' + relative.as_posix()], cwd=root)
    if raw != committed:
        raise ValueError('plan differs from the committed immutable input')
    plan = json.loads(raw)
    if not isinstance(plan, dict) or plan.get('schema') != 1 or set(plan) - {'schema', 'server_source', 'case_sets', 'description', 'selection', 'metadata'}:
        raise ValueError('unknown plan schema or fields')
    if not isinstance(plan.get('server_source'), str) or re.fullmatch('[0-9a-f]{40}', plan['server_source']) is None:
        raise ValueError('plan server_source must be an immutable full commit')
    sets = plan.get('case_sets')
    if not isinstance(sets, dict) or case_set not in sets or not isinstance(sets[case_set], list) or not sets[case_set] or len(sets[case_set]) > 128:
        raise ValueError('case set must be a non-empty finite committed list')
    cases = sets[case_set]
    stages, timed_ns, seen = 0, 0, set()
    for case in cases:
        case_stages = validate_case(case)
        stages += case_stages
        timed_ns += case_stages * (duration_ns(case['warmup']) + duration_ns(case['duration']))
        if case['id'] in seen:
            raise ValueError('duplicate case id in selected case set')
        seen.add(case['id'])
    if stages > 256:
        raise ValueError('case set exceeds 256 planned timed stages')
    if timed_ns > 100 * 60 * 1000000000:
        raise ValueError('case set exceeds 100 minutes of declared warmup and measurement, excluding bounded setup/drain')
    return plan, raw, cases


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def audit_live_batching(report, case):
    expected = case['batching'] if case['batching'] is not None else case['expected_batching']
    values = {'weir_store_batch_max_operations': expected['max_operations'],
              'weir_store_batch_max_bytes': expected['max_bytes'],
              'weir_store_batch_max_wait_seconds': duration_ns(expected['max_wait']) / 1e9,
              'weir_store_active_executions_limit': expected['max_in_flight']}
    store = report['parameters']['dataset']['backend']
    routes = json.loads(report['provenance']['weir_routing_config'])
    stores = [entry for entry in routes['stores'] if entry.get('name') == store]
    if len(stores) != 1:
        raise ValueError('owned routing configuration has missing or duplicate Store')
    configured = stores[0].get('batching')
    if case['batching'] is None:
        if 'batching' in stores[0]:
            raise ValueError('default path must omit its entire batching configuration')
    else:
        validate_batching(dict(configured, max_bytes=int(configured['max_bytes'].removesuffix('B'))))
        if configured['max_operations'] != expected['max_operations'] or configured['max_bytes'] != str(expected['max_bytes']) + 'B' or configured['max_in_flight'] != expected['max_in_flight'] or duration_ns(configured['max_wait']) != duration_ns(expected['max_wait']):
            raise ValueError('owned routing configuration differs from declared controls')
    for pair in report['pairs']:
        for snapshot in (pair['weir']['server_metrics']['before'], pair['weir']['server_metrics']['after']):
            if snapshot.get('error'):
                raise ValueError('live Store controls are unavailable')
            for name, value in values.items():
                metrics = [metric for metric in snapshot['metrics'] if metric['name'] == name and metric.get('labels', {}).get('store') == store]
                if len(metrics) != 1 or metrics[0]['value'] != value:
                    raise ValueError('live Store control mismatch: ' + name)


def run_case(case, output, context):
    output.mkdir()
    report_output = output / ('crawler.json' if case['kind'] == 'crawler' else 'measurement')
    command = crawler_command(case, report_output) if case['kind'] == 'crawler' else write_command(case, report_output)
    server, client = context['server'], context['crawler'] if case['kind'] == 'crawler' else context['client']
    receipt = {key: value for key, value in context.items() if key not in ('server', 'client', 'crawler')}
    receipt.update(case=case, command=command, client_binary_sha256=digest(client),
                   server_binary_sha256=digest(server), server_binary_before_sha256=digest(server))
    receipt_path = output / 'receipt.json'
    receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
    try:
        with (output / 'execution.log').open('x') as log:
            process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            for line in process.stdout:
                print(line, end='', flush=True)
                log.write(line)
            receipt['exit_code'] = process.wait()
        receipt['server_binary_after_sha256'], receipt['client_binary_after_sha256'] = digest(server), digest(client)
        if receipt['server_binary_after_sha256'] != receipt['server_binary_sha256'] or receipt['client_binary_after_sha256'] != receipt['client_binary_sha256']:
            raise ValueError('benchmark executable changed during the case')
        if receipt['exit_code']:
            raise ValueError('benchmark case exited unsuccessfully: ' + str(receipt['exit_code']))
        if case['kind'] == 'crawler':
            audit = subprocess.run([str(client), 'audit', str(report_output)], capture_output=True, text=True)
            (output / 'offline-audit.log').write_text(audit.stdout + audit.stderr)
            if audit.returncode:
                raise ValueError('crawler offline full-field audit failed')
        else:
            for backend in ('mongo', 'search') if case['backend'] == 'all' else (case['backend'],):
                report = json.loads((report_output / (backend + '.json')).read_text())
                validate_report_provenance(report, receipt)
                audit_live_batching(report, case)
                if report.get('incomplete') or len(report.get('pairs', [])) != len(case['levels']) * case['rounds']:
                    raise ValueError('write stage matrix is incomplete')
                if not report.get('provenance', {}).get('weir_effective_config'):
                    raise ValueError('write effective configuration receipt is missing')
                for pair in report['pairs']:
                    for path in ('direct', 'weir'):
                        stage = pair[path]
                        if not stage.get('verified') or stage['succeeded'] != stage['attempted'] or any(stage.get(field, 0) for field in ('errors', 'indeterminate', 'business_operation_timeouts', 'not_attempted', 'applied_with_error')):
                            raise ValueError('write case has unverified or incomplete work')
        receipt['validation'] = 'passed'
    except Exception as error:
        receipt['validation_error'] = str(error)
        receipt['validation'] = 'failed'
    receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
    return receipt['validation'] == 'passed'


def run_cases(cases, output, context):
    outcomes = []
    for case in cases:
        print('Starting declared case ' + case['id'], flush=True)
        passed = run_case(case, output / case['id'], context)
        outcomes.append(dict(id=case['id'], passed=passed))
        (output / 'outcomes.json').write_text(json.dumps(outcomes, indent=2) + '\n')
    return all(item['passed'] for item in outcomes)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--plan', required=True, type=Path)
    parser.add_argument('--case-set', required=True)
    parser.add_argument('--inspect', action='store_true', help='validate committed inputs and print immutable server source')
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    os.chdir(root)
    plan, raw, cases = load_plan(args.plan, args.case_set)
    if args.inspect:
        print(plan['server_source'])
        return
    if subprocess.check_output(['git', 'status', '--porcelain']):
        raise ValueError('benchmark source checkout must be clean')
    output = root / 'results/local/batching-tuning'
    output.mkdir(parents=True, exist_ok=False)
    (output / 'plan.json').write_bytes(raw)
    versions = json.loads((root / 'versions.json').read_text())
    server = root / '.tools/write-weir'
    server_receipt = json.loads(server.with_suffix('.receipt.json').read_text())
    validate_server_receipt(server_receipt, digest(server), versions)
    if server_receipt['source_dirty'] or server_receipt['source'] != plan['server_source']:
        raise ValueError('server source differs from the immutable tuning plan')
    tree, files = freeze_source(root, output / 'client-source')
    context = dict(server=server, client=root / '.tools/weir-lab', crawler=root / '.tools/weir-crawler',
                   test_source=subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
                   test_source_dirty=False, client_source_tree_sha256=tree, client_source_files=files,
                   server_receipt=server_receipt, versions=versions, plan_sha256=hashlib.sha256(raw).hexdigest(),
                   host=platform.uname()._asdict(), host_cpus=os.cpu_count(),
                   host_cpu_model=next((line.split(':', 1)[1].strip() for line in Path('/proc/cpuinfo').read_text().splitlines() if line.startswith('model name')), 'unavailable'),
                   host_memory_bytes=int(next(line for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemTotal:')).split()[1]) * 1024,
                   go=subprocess.check_output(['go', 'version'], text=True).strip(),
                   client_builds={name: subprocess.check_output(['go', 'version', '-m', str(root / '.tools' / name)], text=True) for name in ('weir-lab', 'weir-crawler')},
                   case_set=args.case_set, workflow_run='https://github.com/' + os.environ['GITHUB_REPOSITORY']
                   + '/actions/runs/' + os.environ['GITHUB_RUN_ID'])
    if not run_cases(cases, output, context):
        raise SystemExit('One or more declared cases failed; all case receipts and subsequent outcomes are retained')


if __name__ == '__main__':
    main()
