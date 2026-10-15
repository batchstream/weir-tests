#!/usr/bin/env python3
"""Audit the prospective tuning inventory and apply frozen selection/capacity rules."""
import argparse
import hashlib
import importlib.util
import json
import math
from pathlib import Path
import subprocess
import sys
import tempfile

FOLDER = Path(__file__).resolve().parent
REPO = FOLDER.parents[1]
sys.path.insert(0, str(REPO / 'scripts'))


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    sys.modules[name] = value
    spec.loader.exec_module(value)
    return value


prior = module('default_previous_coverage', FOLDER.parent / 'batch-coverage-2026-10-15/analyze.py')
rules = module('default_selection_rules', FOLDER / 'selection_rules.py')
capacity_rules = module('default_capacity_rules', FOLDER / 'capacity_rules.py')
runner = module('default_plan_runner', REPO / 'scripts/run_tuning_plan.py')
helpers = prior.helpers
require, load = helpers.require, helpers.load


def checksum(raw):
    return hashlib.sha256(raw).hexdigest()


def committed(repository, source, name):
    return subprocess.check_output(['git', 'show', source + ':' + name], cwd=repository)


def command_flags(command):
    result = {}
    index = 1
    while index < len(command):
        name = command[index]
        require(name.startswith('-') and name not in result, 'invalid/duplicate measurement flag')
        if index + 1 == len(command) or command[index + 1].startswith('-'):
            result[name] = True
            index += 1
        else:
            result[name] = command[index + 1]
            index += 2
    return result


def identity(options):
    receipt, entry, case = options['receipt'], options['entry'], options['case']
    require(receipt['test_source'] == entry['client_source'] and receipt['test_source_dirty'] is False, 'dirty/different client source')
    require(receipt['workflow_run'] == entry['workflow_run'] and receipt['case_set'] == entry['case_set'] and receipt['case'] == case, 'workflow/case identity differs')
    require(receipt['plan_sha256'] == entry['plan_sha256'], 'retained plan hash differs')
    tree, files = prior.source_snapshot(REPO, entry['client_source'])
    require(receipt['client_source_tree_sha256'] == tree and receipt['client_source_files'] == files, 'client Go inputs differ from immutable source')
    server = receipt['server_receipt']
    require(server['source'] == entry['server_source'] and server['source_dirty'] is False, 'dirty/different server source')
    tree, files = prior.source_snapshot(REPO.parent / 'weir', entry['server_source'])
    require(server['source_tree_sha256'] == tree and server['source_files'] == files and server['source_diff_sha256'] == checksum(b''), 'server Go inputs differ from immutable source')
    require(server['sha256'] == receipt['server_binary_sha256'] == receipt['server_binary_before_sha256'] == receipt['server_binary_after_sha256'], 'server executable changed')
    require(receipt['client_binary_sha256'] == receipt['client_binary_after_sha256'], 'client executable changed')
    versions = json.loads(committed(REPO, entry['client_source'], 'versions.json'))
    require(receipt['versions'] == versions, 'versions differ from immutable client')
    flags = command_flags(receipt['command'])
    expected = runner.write_command(case, Path('OUTPUT')) if case['kind'] == 'write' else runner.crawler_command(case, Path('OUTPUT'))
    declared = command_flags(expected)
    require({k: v for k, v in flags.items() if k != '-output'} == {k: v for k, v in declared.items() if k != '-output'}, 'measurement command differs from the committed case')
    require(flags.get('-client-processes', flags.get('-processes')) == str(case['client_processes']), 'client process command differs')
    require(flags.get('-database-cpus', flags.get('-cpus')) == str(case['database_cpus']), 'database quota command differs')
    return flags


def fixture_proof(options):
    folder, directory, binary, backend = (options[key] for key in ('folder', 'directory', 'binary', 'backend'))
    directory = directory.strip('/')
    fixture = load(prior.one(folder, directory + '/manifest.json.gz'))
    cleanup = load(prior.one(folder, directory + '/cleanup.json.gz'))
    require(fixture['owner'] and cleanup['owner'] == fixture['owner'] and not cleanup['errors'] and fixture['binary_sha256'] == binary and fixture['host_os'] == 'linux', 'owned fixture/cleanup identity differs')
    original = {(x['id'], x['name'], x['image']) for x in fixture['containers']}
    removed = {(x['id'], x['name'], x['image']) for x in cleanup['containers'] if x['removed']}
    require(original and original == removed and len(original) == len(cleanup['containers']) and cleanup['processes'] and all(x['stopped'] for x in cleanup['processes']), 'owned cleanup incomplete')
    containers = [x for x in fixture['containers'] if x['backend'] == backend]
    require(len(containers) == 1, 'owned backend container missing/ambiguous')
    return containers[0]['id']


def quota_proof(options):
    quota, resources, case, container = (options[key] for key in ('quota', 'resources', 'case', 'container'))
    expected = case['database_cpus']
    require(quota['container_id'] == container and quota['requested_cpus'] == quota['observed_cpus'] == resources['database_allocated_cpus'] == expected and quota['host_config_nano_cpus'] == int(expected * 1e9), 'owned database quota differs')


def children_proof(options):
    stage, case, workers = options['stage'], options['case'], options['workers']
    children = stage['client_processes']
    require(len(children) == case['client_processes'] and len({c['pid'] for c in children}) == len(children), 'client processes missing/duplicate')
    totals = dict.fromkeys(('attempted', 'succeeded', 'errors', 'indeterminate', 'not_attempted', 'applied_with_error', 'business_operation_timeouts'), 0)
    covered, pooled, maximum, reads, writes = set(), [0] * 256, 0, 0, 0
    for index, child in enumerate(children):
        require(child['phase'] == 'timed' and child['owned_process_exited'] and not child.get('owned_process_exit_error') and child['concurrent_workers'] == workers // len(children) and child['worker_offset'] == index * (workers // len(children)) and 0 <= child['start_lag_ns'] <= 100000000, 'child pool/exit/barrier differs')
        require(child['native_connection_pool_limit'] == (workers // len(children) if stage['path'] == 'direct' else 0), 'native pool differs')
        value = child['result']
        for key in totals:
            require(type(value[key]) is int and value[key] >= 0, 'invalid child counter')
            totals[key] += value[key]
        require(child['business_requests'] == child['reads'] + child['writes'] == value['attempted'] == value['succeeded'] + value['errors'] + value['indeterminate'], 'child operation arithmetic differs')
        reads, writes = reads + child['reads'], writes + child['writes']
        buckets = child['latency_histogram_buckets']
        calculated = helpers.histogram(buckets, value['latency']['max_observed_ns'])
        require(all(value['latency'][key] == val for key, val in calculated.items()), 'child histogram differs')
        pooled = [a + b for a, b in zip(pooled, buckets)]
        maximum = max(maximum, value['latency']['max_observed_ns'])
        require(len(child['expected_record_ranges']) == workers // len(children), 'postflight worker ranges missing')
        for part in child['expected_record_ranges']:
            ids = set(range(part['start'], part['start'] + len(part['revisions'])))
            require(ids and not covered.intersection(ids) and all(0 <= i < case['records'] for i in ids) and all(type(v) is int and v in (0, 1) for v in part['revisions']), 'postflight ranges overlap/malformed')
            covered.update(ids)
    require(len(covered) == case['records'] and all(stage[key] == value for key, value in totals.items()), 'full seeded postflight or totals differ')
    require(reads == stage['reads'] and writes == stage['writes'] and stage['client_requests'] == reads + writes == stage['planned'] == stage['attempted'], 'aggregate read/write calls differ')
    calculated = helpers.histogram(pooled, maximum)
    require(all(stage['latency'][key] == value for key, value in calculated.items()), 'pooled stage histogram differs')


def host_observation(options):
    resources, elapsed = options['resources'], options['elapsed']
    covered = busy = steal = iowait = 0
    client_areas, client_spans = {}, {}
    intervals = []
    capacities = set()
    for before, after in zip(resources['samples'], resources['samples'][1:]):
        first, last = before.get('host', {}), after.get('host', {})
        a, b = first.get('cpu'), last.get('cpu')
        left, right = max(0, before['elapsed_ns']), min(elapsed, after['elapsed_ns'])
        if not a or not b or first.get('unavailable') or last.get('unavailable') or left >= right or first.get('platform') != 'linux' or last.get('platform') != 'linux':
            continue
        delta = b['total_ticks'] - a['total_ticks']
        if delta <= 0 or any(b[key] < a[key] for key in ('busy_ticks', 'steal_ticks', 'iowait_ticks')):
            continue
        capacity = min(last['runtime_cpu_count'], last['sampler_allowed_cpus'], b['cpus_listed'])
        require(capacity > 0 and a['cpus_listed'] == b['cpus_listed'], 'host CPU denominator differs')
        capacities.add(capacity if last['runtime_cpu_count'] == last['sampler_allowed_cpus'] == b['cpus_listed'] else 0)
        span = right - left
        values = {key: (b[key] - a[key]) / delta * 100 for key in ('busy_ticks', 'steal_ticks', 'iowait_ticks')}
        covered += span
        busy += values['busy_ticks'] * span
        steal += values['steal_ticks'] * span
        iowait += values['iowait_ticks'] * span
        intervals.append(dict(start_ns=left, end_ns=right, **values))
        first_clients = {item['pid']: item for item in before.get('client_processes', [])}
        last_clients = {item['pid']: item for item in after.get('client_processes', [])}
        for pid in first_clients.keys() & last_clients.keys():
            first_client, last_client = first_clients[pid], last_clients[pid]
            if first_client.get('error') or last_client.get('error'):
                continue
            cpu_delta = last_client['cpu_time_ns'] - first_client['cpu_time_ns']
            clock_delta = after['elapsed_ns'] - before['elapsed_ns']
            if cpu_delta < 0 or clock_delta <= 0:
                continue
            client_areas[pid] = client_areas.get(pid, 0) + cpu_delta / clock_delta * span
            client_spans[pid] = client_spans.get(pid, 0) + span

    result = dict(host_coverage=covered / elapsed, host_busy_percent=busy / covered if covered else None, host_steal_percent=steal / covered if covered else None, host_iowait_percent=iowait / covered if covered else None, effective_host_cpus=next(iter(capacities)) if len(capacities) == 1 else None, host_intervals=intervals)
    result['host_headroom'] = bool(result['host_coverage'] >= .9 and result['effective_host_cpus'] and result['host_busy_percent'] <= 85 and result['host_steal_percent'] <= 5)
    clients = [dict(pid=pid, cpu_cores=area / client_spans[pid], coverage=client_spans[pid] / elapsed) for pid, area in sorted(client_areas.items())]
    result['client_process_cpu'] = clients
    expected_pids = set(options['client_pids'])
    result['expected_client_pids'] = sorted(expected_pids)
    result['client_headroom'] = bool(clients and {client['pid'] for client in clients} == expected_pids and result['effective_host_cpus'] and all(client['coverage'] >= .9 and client['cpu_cores'] < result['effective_host_cpus'] * .8 for client in clients))

    return result


def qualified(row, formal=False):
    errors = []
    if not row.get('checks_passed') or not row.get('verified') or not row.get('completed') or any(row.get(k, 0) for k in ('failed', 'unknown', 'timeout')):
        errors.append('source/measurement/postflight/operation quality')
    if row.get('db_cpu_coverage', 0) < (.95 if formal else .8) or not row.get('db_cpu_ms_per_completed', 0):
        errors.append('CPU observation quality')
    if row.get('p99_ms', math.inf) > 5000:
        errors.append('p99 exceeds 5 seconds')
    if row['kind'] == 'crawler' and (row.get('offered') != row.get('completed') or row.get('rejected') != 0 or row.get('drain_s', math.inf) > 3 or row.get('max_generator_lag_ms', math.inf) > 50):
        errors.append('all-arrivals/drain/generator SLO')
    row['usable'], row['qualification_errors'] = not errors, errors


def stage_base(options):
    entry, case, path, workers, rd = (options[key] for key in ('entry', 'case', 'path', 'workers', 'round'))
    batch = case['batching'] or case['expected_batching']
    row = dict(label=entry['label'], phase=entry['phase'], case_set=entry['case_set'], case_id=case['id'], kind=case['kind'], path=path, workers=workers, round=rd, database_cpus=case['database_cpus'], rate=case.get('rate'), profile=case.get('profile'), controls=batch, wait_ns=runner.duration_ns(batch['max_wait']), max_bytes=batch['max_bytes'], checks_passed=False, verified=False, error=None)
    return row


def process_write(options):
    folder, entry, case, receipt = (options[key] for key in ('folder', 'entry', 'case', 'receipt'))
    backend = case['backend']
    require(backend != 'all', 'study uses separate backend workloads')
    report = load(prior.one(folder, case['id'] + '/measurement/' + backend + '.json.gz'))
    require(report['schema'] == 4, 'unexpected write report schema')
    prior.evidence.validate_report_provenance(report, receipt)
    runner.audit_live_batching(report, case)
    params, dataset = report['parameters'], report['parameters']['dataset']
    require(params['concurrency_levels'] == case['levels'] and params['batch_sizes'] == [1] and params['paired_rounds'] == case['rounds'] and params['requested_write_percent'] == case['write_percent'] and params['client_process_count'] == case['client_processes'] and params['records_per_business_request'] == 1 and runner.duration_ns(params['warmup_duration']) == runner.duration_ns(case['warmup']) and runner.duration_ns(params['measurement_duration']) == runner.duration_ns(case['duration']), 'write workload parameters differ')
    require(dataset['backend'] == backend and dataset['records'] == case['records'] and dataset['padding_bytes'] == case['payload_bytes'] and bool(dataset.get('lua_mutations')) == case.get('lua_mutations', False), 'write dataset differs')
    require(report['provenance'].get('weir_effective_config'), 'effective configuration missing')
    container = fixture_proof(dict(folder=folder, directory=report['provenance']['fixture_logs'], binary=receipt['server_binary_sha256'], backend=backend))
    require([(p['concurrency'], p['round']) for p in report['pairs']] == [(w, r) for w in case['levels'] for r in range(1, case['rounds'] + 1)], 'write matrix missing/reordered')
    rows = []
    for pair in report['pairs']:
        expected_order = ['direct', 'weir'] if (case['levels'].index(pair['concurrency']) + pair['round'] - 1) % 2 == 0 else ['weir', 'direct']
        require(pair['order'] == expected_order, 'paired path order differs')
        for path in pair['order']:
            stage = pair[path]
            row = stage_base(dict(entry=entry, case=case, path=path, workers=pair['concurrency'], round=pair['round']))
            row.update(verified=stage['verified'], completed=stage['succeeded'], failed=stage['errors'], unknown=stage['indeterminate'], timeout=stage['business_operation_timeouts'], elapsed_s=stage['elapsed_ns'] / 1e9, p99_ms=stage['latency']['p99_upper_bound_ns'] / 1e6)
            row['completed_s'] = stage['succeeded'] / row['elapsed_s'] if row['elapsed_s'] > 0 else None
            try:
                children_proof(dict(stage=stage, case=case, workers=pair['concurrency']))
                quota_proof(dict(quota=report['database_cpu_quota'], resources=stage['resources'], case=case, container=container))
                require(stage['path'] == path and stage['elapsed_ns'] >= runner.duration_ns(case['duration']) and math.isclose(stage['successful_operations_per_second'], row['completed_s'], rel_tol=1e-12) and not stage.get('verification_error') and stage['not_attempted'] == stage['applied_with_error'] == 0, 'stage arithmetic/postflight differs')
                row.update(prior.cpu_observation(stage['resources'], stage['elapsed_ns'], stage['succeeded']))
                row.update(host_observation(dict(resources=stage['resources'], elapsed=stage['elapsed_ns'], client_pids=[child['pid'] for child in stage['client_processes']])))
                if backend == 'search':
                    for key in ('timed_database_counters_before', 'timed_database_counters_after'):
                        counter = stage[key]
                        settings = counter['search_index_settings'][dataset['namespace']]['settings']
                        require(settings['index.refresh_interval'] == '1s' and settings['index.translog.durability'] == 'request' and settings['index.number_of_shards'] == '1' and settings['index.number_of_replicas'] == '0', 'effective Search policy differs')
                        stats = counter['search_index_stats']
                        require(stats['_shards']['failed'] == 0 and set(stats['indices']) == {dataset['namespace']}, 'Search index observation identity differs')
                if path == 'weir':
                    metrics = stage['server_metrics']
                    row['adapter_batch_mean'] = metrics.get('timed_adapter_batch_average')
                    if backend == 'search':
                        row.update(prior.phase_observations(dict(ServerBefore=metrics['before'], ServerAfter=metrics['after'])))
                        if case['write_percent'] == 100:
                            require(row['search_bulk_records'] == stage['writes'], 'actual framed writes differ from completed calls')
                        row['http_body_buckets'] = [x for x in row['search_metric_deltas'] if x['name'] == 'weir_search_http_request_body_bytes_bucket' and x['labels'].get('operation') == 'bulk']
                        if entry['case_set'] == 'select-bytes':
                            matching = [x for x in row['http_body_buckets'] if float(x['labels']['le']) == row['max_bytes']]
                            require(len(matching) == 1 and matching[0]['delta'] == row['search_bulk_attempts'], 'byte target histogram does not contain every actual bulk attempt')
                            row['byte_target_observed_bound'] = True
                pools = [child.get('time_buckets') for child in stage['client_processes']]
                totals = dict(completed=stage['succeeded'], failed=stage['errors'] + stage['indeterminate'], unknown=stage['indeterminate'], timed_out=stage['business_operation_timeouts'], offered=stage['attempted'], rejected=0)
                row['time_buckets'] = capacity_rules.merge_buckets(dict(pools=pools, duration=runner.duration_ns(case['duration']), histogram=helpers.histogram, latency=stage['latency'], totals=totals))
                row.update(capacity_rules.stability(dict(buckets=row['time_buckets'], duration=runner.duration_ns(case['duration']), histogram=helpers.histogram)))
                row['checks_passed'] = not report.get('incomplete') and receipt['exit_code'] == 0 and receipt.get('validation') == 'passed'
            except (KeyError, TypeError, ValueError, ZeroDivisionError) as error:
                row['error'] = str(error)
            qualified(row, entry['phase'] != 'selection')
            rows.append(row)
    return rows


def process_crawler(options):
    folder, entry, case, receipt, auditor = (options[key] for key in ('folder', 'entry', 'case', 'receipt', 'auditor'))
    report = load(prior.one(folder, case['id'] + '/crawler.json.gz'))
    require(report['schema'] == 3, 'unexpected crawler schema')
    audit = prior.audit_report(report, auditor)
    params = report['Parameters']
    require(params['Paths'] == case['paths'] and params['Rates'] == [case['rate']] and params['Concurrency'] == [case['concurrency']] and params['Profiles'] == [case['profile']] and params['Processes'] == case['client_processes'] and params['Visibility'] == case['visibility'] and params['SeedPaddingBytes'] == case['seed_padding_bytes'] and params['Warmup'] == runner.duration_ns(case['warmup']) and params['Duration'] == runner.duration_ns(case['duration']) and params['Rounds'] == case['rounds'], 'crawler workload differs')
    expected = case['batching'] or case['expected_batching']
    controls = dict(AfterUseDefaults=case['batching'] is None, CaptureEffectiveConfig=True,
                    AfterBatchMaxOperations=expected['max_operations'],
                    AfterBatchMaxBytes=expected['max_bytes'], AfterBatchMaxWait=runner.duration_ns(expected['max_wait']),
                    AfterMaxInFlight=expected['max_in_flight'], CPUs=case['database_cpus'],
                    InputShape='single', Modes=['fixed-rate'], PageSize=1, QueuePages=1,
                    Timeout=10_000_000_000, HTTPObserver=case['http_observer'])
    require(all(params[key] == value for key, value in controls.items()), 'crawler effective candidate/fairness parameters differ')
    for name in ('current', 'optimized'):
        require(report['Servers'][name]['sha256'] == receipt['server_binary_sha256'], 'crawler server binary differs')
    container = fixture_proof(dict(folder=folder, directory=report['Fixture'], binary=receipt['server_binary_sha256'], backend='search'))
    rows = []
    for stage in report['Stages']:
        row = stage_base(dict(entry=entry, case=case, path=stage['Path'], workers=stage['Concurrency'], round=stage['Round']))
        row.update(verified=stage['Verified'], completed=stage['Completed'], offered=stage['Offered'], rejected=stage['Rejected'], failed=stage['Failed'], unknown=stage['Indeterminate'], timeout=stage['TimedOut'], elapsed_s=stage['ElapsedNS'] / 1e9, p99_ms=stage['ScheduledToVisible']['p99_upper_bound_ns'] / 1e6, drain_s=stage['DrainNS'] / 1e9, max_generator_lag_ms=stage['GeneratorLagNS'] / 1e6, original_go_audit=audit)
        row['completed_s'] = stage['Completed'] / row['elapsed_s'] if row['elapsed_s'] > 0 else None
        try:
            quota_proof(dict(quota=stage['CPUQuota'], resources=stage['Resources'], case=case, container=container))
            require(stage['ServerSHA256'] == receipt['server_binary_sha256'] and stage['ClientsExited'] and stage['ServerEffectiveConfiguration'] and stage['DrainNS'] == max(0, stage['ElapsedNS'] - runner.duration_ns(case['duration'])), 'crawler boundary/identity differs')
            row.update(prior.cpu_observation(stage['Resources'], stage['ElapsedNS'], stage['Completed']))
            row.update(host_observation(dict(resources=stage['Resources'], elapsed=stage['ElapsedNS'], client_pids=[client['PID'] for client in stage['Clients']])))
            row.update(prior.phase_observations(stage))
            pools = [worker.get('time_buckets') for reply in stage['Clients'] for worker in reply['Workers']]
            totals = dict(completed=stage['Completed'], failed=stage['Failed'], unknown=stage['Indeterminate'], timed_out=stage['TimedOut'], offered=stage['Offered'], rejected=stage['Rejected'])
            row['time_buckets'] = capacity_rules.merge_buckets(dict(pools=pools, duration=runner.duration_ns(case['duration']), histogram=helpers.histogram, latency=stage['ScheduledToVisible'], totals=totals))
            row.update(capacity_rules.stability(dict(buckets=row['time_buckets'], duration=runner.duration_ns(case['duration']), histogram=helpers.histogram, rate=case['rate'])))
            row['checks_passed'] = audit['exit_code'] == 0 and receipt['exit_code'] == 0 and receipt.get('validation') == 'passed'
            if audit['exit_code'] != 0:
                row['error'] = 'original-source crawler audit failed: ' + audit['output']
        except (KeyError, TypeError, ValueError, ZeroDivisionError) as error:
            row['error'] = str(error)
        qualified(row, entry['phase'] != 'selection')
        rows.append(row)
    return rows


def process_run(options):
    entry, auditor = options['entry'], options['auditor']
    folder = FOLDER / 'raw' / entry['label']
    prior.verify_archive(folder)
    workflow = load(prior.one(folder, 'workflow.json.gz'))
    require(workflow['headSha'] == entry['client_source'] and workflow['url'] == entry['workflow_run'] and workflow['status'] == 'completed' and workflow['event'] == 'workflow_dispatch', 'workflow identity differs')
    plan_raw = committed(REPO, entry['client_source'], entry['plan_path'])
    require(checksum(plan_raw) == entry['plan_sha256'], 'committed plan differs')
    plan = json.loads(plan_raw)
    require(plan['server_source'] == entry['server_source'], 'plan server differs')
    retained = load(prior.one(folder, 'batching-tuning/plan.json.gz'))
    require(retained == plan, 'retained dispatched plan differs')
    rows, failures = [], []
    for case in plan['case_sets'][entry['case_set']]:
        try:
            receipt = load(prior.one(folder, case['id'] + '/receipt.json.gz'))
            identity(dict(receipt=receipt, entry=entry, case=case))
            arguments = dict(folder=folder, entry=entry, case=case, receipt=receipt, auditor=auditor)
            rows.extend(process_write(arguments) if case['kind'] == 'write' else process_crawler(arguments))
        except (KeyError, TypeError, ValueError, OSError, subprocess.SubprocessError) as error:
            failures.append(dict(label=entry['label'], case_id=case['id'], error=str(error)))
    result = dict(rows=rows, failures=failures, workflow_conclusion=workflow['conclusion'])
    return result


def source_proof(options):
    manifest, auditor = options['manifest'], options['auditor']
    source = manifest['analysis_source']
    names = manifest['analysis_inputs']
    require(names and len(set(names)) == len(names), 'missing/duplicate analysis input manifest')
    require(set(analysis_inputs()).issubset(names), 'analysis manifest omits an imported repository helper')
    for name in names:
        candidate = REPO / name
        require(candidate.resolve().is_relative_to(REPO.resolve()) and not candidate.is_symlink(), 'unsafe analysis input')
        require(candidate.read_bytes() == committed(REPO, source, name), 'analysis input differs from frozen source: ' + name)
    receipt = load(FOLDER / 'auditor-build.json')
    require(receipt['base_source'] == manifest['selection_client'] and receipt['unmodified_source_archive'] and receipt['binary_sha256'] == checksum(auditor.read_bytes()), 'original auditor receipt differs')
    scope = receipt['source_archive_scope']
    require(scope == ['cmd', 'internal', 'go.mod', 'go.sum'], 'original auditor source scope differs')
    archive = subprocess.check_output(['git', 'archive', '--format=tar', receipt['base_source'], '--', *scope], cwd=REPO)
    require(checksum(archive) == receipt['source_archive_sha256'], 'original auditor source archive differs')
    tree, _ = prior.source_snapshot(REPO, receipt['base_source'])
    for entry in manifest['entries']:
        other, _ = prior.source_snapshot(REPO, entry['client_source'])
        require(tree == other, 'measurement and original auditor Go inputs differ')


def analysis_inputs():
    names = {Path(__file__).resolve().relative_to(REPO).as_posix()}
    for value in list(sys.modules.values()):
        candidate = getattr(value, '__file__', None)
        if candidate and Path(candidate).is_file() and Path(candidate).suffix == '.py' and Path(candidate).resolve().is_relative_to(REPO.resolve()):
            names.add(Path(candidate).resolve().relative_to(REPO).as_posix())
    return sorted(names)


def formal_results(options):
    manifest, rows = options['manifest'], options['rows']
    capacity, rate_bounds, sustained, comparisons = [], [], [], []
    for entry in manifest['entries']:
        if entry['phase'] == 'selection':
            continue
        plan = json.loads(committed(REPO, entry['client_source'], entry['plan_path']))
        cases = plan['case_sets'][entry['case_set']]
        measured = [row for row in rows if row['label'] == entry['label']]
        for case in cases:
            points = [row for row in measured if row['case_id'] == case['id']]
            if not points:
                continue
            if entry['phase'] == 'soak':
                for row in points:
                    if row['path'] in ('weir', 'optimized'):
                        try:
                            decision = capacity_rules.soak(dict(row=row, kind=case['kind'], histogram=helpers.histogram))
                        except (KeyError, TypeError, ValueError) as error:
                            decision = dict(passed=False, errors=[str(error)])
                        sustained.append(dict(label=entry['label'], case_id=case['id'], decision=decision))
            elif case['kind'] == 'write' and case['backend'] == 'search' and case['write_percent'] == 100 and not case.get('lua_mutations') and len(case['levels']) >= 2:
                capacity.append(dict(label=entry['label'], case_id=case['id'], decision=capacity_rules.capacity(dict(rows=points, levels=case['levels']))))
            for row in points:
                if row['path'] not in ('weir', 'optimized'):
                    continue
                native = next((item for item in points if item['path'] == 'direct' and item['round'] == row['round'] and item['workers'] == row['workers']), None)
                qualified_pair = bool(native and native['usable'] and row['usable'])
                comparison = dict(label=entry['label'], case_id=case['id'], workers=row['workers'], round=row['round'], qualified=qualified_pair)
                if qualified_pair:
                    comparison.update(qps_ratio=row['completed_s'] / native['completed_s'], database_cpu_per_unit_ratio=row['db_cpu_ms_per_completed'] / native['db_cpu_ms_per_completed'], actual_database_cpu_ratio=row['db_cpu_quota_percent'] / native['db_cpu_quota_percent'])
                    comparison['comparable_throughput'] = .95 <= comparison['qps_ratio'] <= 1.05
                comparisons.append(comparison)
        crawler_cases = [case for case in cases if case['kind'] == 'crawler']
        if entry['phase'] != 'soak' and len(crawler_cases) >= 2:
            rates = sorted({case['rate'] for case in crawler_cases})
            if measured and len(rates) >= 2:
                rate_bounds.append(dict(label=entry['label'], decision=capacity_rules.crawler_rate_bounds(dict(rows=measured, rates=rates))))
    result = dict(cpu_capacity=capacity, crawler_rate_bounds=rate_bounds, sustained=sustained, comparisons=comparisons)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, default=FOLDER / 'manifest.json')
    parser.add_argument('--auditor', type=Path, required=True)
    parser.add_argument('--output', type=Path, default=FOLDER / 'analysis')
    parser.add_argument('--partial', action='store_true')
    args = parser.parse_args()
    manifest = load(args.manifest)
    source_proof(dict(manifest=manifest, auditor=args.auditor))
    rows, runs = [], []
    for entry in manifest['entries']:
        summary = dict(label=entry['label'], available=False, failures=[])
        try:
            value = process_run(dict(entry=entry, auditor=args.auditor))
            rows.extend(value['rows'])
            summary.update(available=True, failures=value['failures'], workflow_conclusion=value['workflow_conclusion'])
        except (KeyError, TypeError, ValueError, OSError, subprocess.SubprocessError) as error:
            require(args.partial, entry['label'] + ': ' + str(error))
            summary['failures'].append(dict(error=str(error)))
        runs.append(summary)
    plan = json.loads(committed(REPO, manifest['selection_client'], manifest['selection_plan']))
    selection = rules.choose(dict(rows=[r for r in rows if r['phase'] == 'selection'], plan=plan))
    byte_selection = rules.choose_bytes([r for r in rows if r['phase'] == 'selection'])
    result = dict(schema=1, runs=runs, rows=rows, selection=selection, byte_selection=byte_selection, formal=formal_results(dict(manifest=manifest, rows=rows)), complete_inventory=all(run['available'] for run in runs), analysis_sha256=checksum(Path(__file__).read_bytes()), selection_rules_sha256=checksum((FOLDER / 'selection_rules.py').read_bytes()))
    args.output.mkdir(parents=True, exist_ok=False)
    (args.output / 'analysis.json').write_text(json.dumps(result, indent=2, allow_nan=False) + '\n')
    for name, values in (('stages', rows), ('runs', runs)):
        flattened = [{k: json.dumps(v, sort_keys=True) if isinstance(v, (dict, list)) else v for k, v in row.items()} for row in values]
        helpers.csv_write(args.output / (name + '.csv'), flattened)
    print(json.dumps(dict(workflows=len(runs), available=sum(run['available'] for run in runs), stages=len(rows), selected=selection['selected'], bytes=byte_selection['selected'])))


if __name__ == '__main__':
    main()
