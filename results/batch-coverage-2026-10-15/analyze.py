#!/usr/bin/env python3
"""Audit the six prospectively fixed batching workflows; retain failed and missing rows."""
import argparse
import functools
import gzip
import hashlib
import importlib.util
import io
import json
import math
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tarfile
import tempfile

FOLDER = Path(__file__).resolve().parent
REPO = FOLDER.parents[1]
CLIENT = '3e180ebd886d44d5d936d9a5f5192f8295a3f2d1'
SERVER = 'b9dfbc207022e0e8ab76118c58cef59e8bac33f3'
CLIENT_REF = 'randy/batch-coverage-client'
PATHS = ('direct', 'current', 'optimized')
CASES = {'immediate': (32, 0, 0), 'bounded32': (32, 5_000_000, 16),
         'bounded128': (128, 5_000_000, 16), 'bounded256': (256, 5_000_000, 16),
         'longer-wait': (128, 20_000_000, 16), 'fewer-slots': (128, 5_000_000, 4)}


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    result = importlib.util.module_from_spec(spec)
    sys.modules[name] = result
    spec.loader.exec_module(result)
    return result


legacy = module('coverage_mechanism_helpers', FOLDER.parent/'batch-mechanism-2026-10-15/analyze.py')
helpers, observations = legacy.helpers, legacy.observations
require, load = helpers.require, helpers.load
evidence = module('coverage_provenance', REPO/'scripts/performance_evidence.py')


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def verify_archive(folder):
    inventory = load(folder/'files.json')
    names = [item['path'] for item in inventory]
    require(names and len(set(names)) == len(names), 'empty/duplicate archive inventory')
    actual = {path.relative_to(folder).as_posix() for path in folder.rglob('*.gz')}
    require(set(names) == actual, 'archive inventory differs from retained gzip files')
    for item in inventory:
        relative = PurePosixPath(item['path'])
        require(not relative.is_absolute() and '..' not in relative.parts and relative.as_posix() == item['path'] and item['path'].endswith('.gz'), 'unsafe archive path')
        path = folder/item['path']
        require(not path.is_symlink() and path.resolve().is_relative_to(folder.resolve()), 'archive escapes its folder')
        compressed = path.read_bytes()
        raw = gzip.decompress(compressed)
        require(digest(compressed) == item['gzip_sha256'] and digest(raw) == item['sha256'] and len(raw) == item['bytes'], 'archive hash/length mismatch: '+item['path'])
    return len(inventory)


def one(folder, suffix):
    matches = [path for path in folder.rglob('*.gz') if path.relative_to(folder).as_posix() == suffix or path.relative_to(folder).as_posix().endswith('/'+suffix)]
    require(len(matches) == 1, 'missing/ambiguous retained artifact: '+suffix)
    return matches[0]


def command_flags(command):
    require(len(command) % 2 == 1 and len(set(command[1::2])) == len(command[1::2]), 'invalid/duplicate command flags')
    return dict(zip(command[1::2], command[2::2]))


@functools.cache
def source_snapshot(repository, revision):
    paths = subprocess.check_output(['git', 'ls-tree', '-r', '--name-only', revision, '--', 'cmd', 'internal', 'pkg', 'api', 'tests', 'go.mod', 'go.sum'], cwd=repository, text=True).splitlines()
    paths = sorted(path for path in paths if path.endswith('.go') or path in ('go.mod', 'go.sum'))
    require('go.mod' in paths, 'source snapshot lacks go.mod')
    archive = subprocess.check_output(['git', 'archive', '--format=tar', revision, '--', *paths], cwd=repository)
    with tarfile.open(fileobj=io.BytesIO(archive)) as stream:
        files = {member.name: stream.extractfile(member).read() for member in stream.getmembers() if member.isfile()}
    tree = hashlib.sha256()
    manifest = []
    for name in paths:
        tree.update(name.encode()+b'\0'+files[name]+b'\0')
        manifest.append(dict(path=name, sha256=digest(files[name])))
    return tree.hexdigest(), manifest


def source_identity(receipt, local=False):
    require(receipt['source'] == SERVER and receipt['source_dirty'] is False and re.fullmatch('[a-f0-9]{64}', receipt['sha256']), 'server source/hash differs or is dirty')
    if local:
        tree, files = source_snapshot(REPO.parent/'weir', SERVER)
        require(receipt['source_tree_sha256'] == tree and receipt['source_files'] == files and receipt['source_diff_sha256'] == digest(b''), 'frozen server Go input tree differs')
    else:
        identity = receipt['identity']
        require(identity['revision'] == SERVER and identity['dirty'] == 'false' and identity['state'] == 'clean-commit', 'server binary revision differs')


def owned_fixture(fixture, cleanup, binary):
    require(fixture['owner'] and cleanup['owner'] == fixture['owner'] and not cleanup['errors'] and fixture['binary_sha256'] == binary and fixture['host_os'] == 'linux', 'fixture identity or cleanup failed')
    expected = {(item['id'], item['name'], item['image']) for item in fixture['containers']}
    actual = {(item['id'], item['name'], item['image']) for item in cleanup['containers'] if item['removed']}
    require(expected and expected == actual and len(expected) == len(cleanup['containers']) and cleanup['processes'] and all(item['stopped'] for item in cleanup['processes']), 'owned processes/containers not completely removed')
    containers = [item for item in fixture['containers'] if item['backend'] == 'search']
    require(len(containers) == 1, 'owned Search container missing/ambiguous')
    return containers[0]['id']


def quota_proof(quota, container, resources):
    require(quota['container_id'] == container and quota['observed_cpus'] == quota['requested_cpus'] == resources['database_allocated_cpus'] == 1 and quota['host_config_nano_cpus'] == 10**9, 'actual database identity/CPU quota differs')


def batching_proof(configuration, snapshots, controls):
    operations, wait, inflight = controls
    stores = configuration['stores']
    require(len(stores) == 1 and stores[0]['name'] == 'search', 'Search route missing/ambiguous')
    batch = stores[0]['batching']
    require(batch['max_operations'] == operations and batch['max_bytes'] == str(8 << 20)+'B' and batch.get('max_in_flight', 0) == inflight and helpers.duration(batch.get('max_wait', '0s')) == wait, 'raw batching configuration differs')
    for snapshot in snapshots:
        require(not snapshot.get('error'), 'live Store metrics unavailable')
        require(helpers.prom_value(snapshot, 'weir_store_batch_max_wait_seconds') == wait/1e9 and helpers.prom_value(snapshot, 'weir_store_active_executions_limit') == inflight, 'live batching wait/slot control differs')


def cpu_observation(resources, elapsed, completed):
    stage = dict(Resources=resources, ElapsedNS=elapsed)
    pressure = helpers.cpu_pressure(stage)
    cpu_ns, coverage, mean = observations.cpu_cost(stage)
    intervals = []
    for before, after in zip(resources['samples'] or [], (resources['samples'] or [])[1:]):
        left, right = max(0, before['elapsed_ns']), min(elapsed, after['elapsed_ns'])
        clock = after.get('database_cpu_counter_clock_unix_ns', 0)-before.get('database_cpu_counter_clock_unix_ns', 0)
        if clock <= 0:
            clock = after['elapsed_ns']-before['elapsed_ns']
        first, last = before.get('database_cpu_time_ns'), after.get('database_cpu_time_ns')
        if left >= right or clock <= 0 or first is None or last is None or last < first or before.get('error') or after.get('error'):
            continue
        intervals.append(dict(start_ns=left, end_ns=right, db_cpu_percent=(last-first)/clock/resources['database_allocated_cpus']*100))
    halves = []
    for start, end in ((0, elapsed/2), (elapsed/2, elapsed)):
        covered = area = 0
        for interval in intervals:
            span = max(0, min(end, interval['end_ns'])-max(start, interval['start_ns']))
            covered += span
            area += span*interval['db_cpu_percent']
        halves.append(area/covered if covered else None)
    return dict(db_cpu_ns=cpu_ns, db_cpu_coverage=coverage, db_cpu_quota_percent=mean,
                db_cpu_ms_per_completed=cpu_ns/1e6/completed if cpu_ns is not None and completed else None,
                sustained_database_cpu_saturation=pressure['sustained'], cpu_intervals=intervals,
                first_half_cpu_percent=halves[0], second_half_cpu_percent=halves[1],
                fraction_measurement_at_90_percent_cpu=pressure['fraction_measurement_at_90_percent'])


def phase_observations(stage):
    legacy.phase_metric_checks(stage)
    deltas = observations.search_metric_deltas(stage)
    output = dict(search_metric_deltas=deltas)
    for operation in ('metadata', 'mget', 'bulk', 'refresh', 'search', 'other'):
        labels = dict(operation=operation)
        values = {}
        for suffix, name in (('attempts', 'weir_search_http_requests_total'), ('duration_s', 'weir_search_http_request_duration_seconds_sum'), ('duration_count', 'weir_search_http_request_duration_seconds_count'), ('body_bytes', 'weir_search_http_request_body_bytes_sum'), ('body_count', 'weir_search_http_request_body_bytes_count'), ('records', 'weir_search_http_request_records_sum'), ('record_count', 'weir_search_http_request_records_count')):
            values[suffix] = observations.search_metric_sum(deltas, name, labels)
            output['search_'+operation+'_'+suffix] = values[suffix]
        for count in ('duration_count', 'body_count', 'record_count'):
            if values[count] is not None and values['attempts'] is not None:
                require(values[count] == values['attempts'], 'HTTP phase histogram/attempt counts differ')
        output['search_'+operation+'_records_mean'] = values['records']/values['record_count'] if values['records'] is not None and values['record_count'] else None
        output['search_'+operation+'_body_bytes_mean'] = values['body_bytes']/values['body_count'] if values['body_bytes'] is not None and values['body_count'] else None
    return output


def diagnostic_counts(stage, metrics):
    checks = []
    if stage['Path'] == 'direct':
        observed = (stage['HTTPDelta'] or {}).get('POST _update', {}).get('Requests', 0)
        checks.append(dict(phase='native_update', field='attempts', internal=stage['ApplicationRequests']['requests'], observer=observed, equal=observed == stage['ApplicationRequests']['requests']))
    else:
        for operation, route in (('metadata', 'GET index-metadata'), ('mget', 'POST _mget'), ('bulk', 'POST _bulk'), ('refresh', 'POST _refresh')):
            observed = (stage['HTTPDelta'] or {}).get(route, {})
            for field, observer_field in (('attempts', 'Requests'), ('body_bytes', 'RequestBytes')):
                value = metrics.get('search_'+operation+'_'+field)
                count = observed.get(observer_field, 0)
                checks.append(dict(phase=operation, field=field, internal=value, observer=count, equal=value == count if value is not None else None))
    usable = all(item['equal'] is True or item['internal'] is None and item['observer'] == 0 for item in checks)
    if stage['Path'] != 'direct':
        usable = usable and all(any(item['phase'] == 'bulk' and item['field'] == field and item['equal'] is True and item['internal'] > 0 for item in checks) for field in ('attempts', 'body_bytes'))
    return dict(usable=usable, checks=checks)


def formal_gate(row):
    qualified = row['role'] == 'formal' and row['run_checks_passed'] and row['stage_checks_passed'] and row['verified'] and row['failed'] == row['unknown'] == row['timeout'] == 0 and row['db_cpu_coverage'] >= .8
    row['formal_measurement'] = bool(qualified)
    row['formal_all_arrivals'] = bool(qualified and row['kind'] == 'crawler' and row['rejected'] == 0 and row['offered'] == row['completed'] and row['p99_ms'] <= 5000 and row['drain_s'] <= 3 and row['max_generator_lag_ms'] <= 50)


def crawler_identity(bundle, manifest):
    report, receipt, entry = bundle['report'], bundle['receipt'], bundle['entry']
    require(report['schema'] == 2 and receipt['source'] == CLIENT and receipt['source_dirty'] is False, 'crawler schema/source differs')
    require(receipt['client_binary_sha256'] == receipt['client_binary_after_sha256'] and re.fullmatch('[a-f0-9]{64}', receipt['client_binary_sha256']), 'crawler client binary changed')
    require(len(receipt['servers']) == len(receipt['server_binary_after_sha256']) == 2, 'crawler server receipts missing')
    for index, path in enumerate(('current', 'optimized')):
        server = receipt['servers'][index]
        source_identity(server)
        require(server == report['Servers'][path]['receipt'] and server['sha256'] == receipt['server_binary_after_sha256'][index] == report['Servers'][path]['sha256'], 'crawler server binary/report receipt differs')
    require(receipt['servers'][0]['sha256'] == receipt['servers'][1]['sha256'], 'same-server paths used different binaries')
    opts = report['Parameters']
    partial = entry['profile'] == 'partial-update'
    controls = dict(InputShape='single', Profiles=[entry['profile']], Modes=['fixed-rate'], Rates=[entry['rate']], Concurrency=[512], Processes=4, PageSize=1, QueuePages=1, Timeout=10**10, CPUs=1, AfterMaxInFlight=16, AfterBatchMaxOperations=128, AfterBatchMaxBytes=8 << 20, AfterBatchMaxWait=5_000_000, Visibility='application_acknowledged' if partial else 'query_visible', SeedPaddingBytes=16 << 10 if partial else 0, Warmup=entry['warmup_ns'], Duration=entry['duration_ns'], Rounds=entry['rounds'], HTTPObserver=entry['http_observer'])
    require(all(opts[key] == value for key, value in controls.items()), 'crawler fixed controls differ')
    flags = command_flags(receipt['command'])
    expected = {'-input-shape': 'single', '-http-observer': entry['http_observer'], '-profiles': entry['profile'], '-modes': 'fixed-rate', '-rates': str(entry['rate']), '-concurrency-levels': '512', '-client-processes': '4', '-page-size': '1', '-queue-pages': '1', '-database-cpus': '1', '-after-max-in-flight': '16', '-after-batch-max-operations': '128', '-after-batch-max-bytes': str(8 << 20), '-visibility': controls['Visibility'], '-seed-padding-bytes': str(controls['SeedPaddingBytes'])}
    require(all(flags[key] == value for key, value in expected.items()), 'crawler command fixed controls differ')
    for flag, key in (('-timeout', 'Timeout'), ('-warmup', 'Warmup'), ('-duration', 'Duration'), ('-after-batch-max-wait', 'AfterBatchMaxWait')):
        require(helpers.duration(flags[flag]) == opts[key], 'crawler command duration differs')
    environ = dict(CRAWLER_PROFILES=entry['profile'], CRAWLER_LEVELS='512', CRAWLER_PAGE_SIZE='1', CRAWLER_VISIBILITY=controls['Visibility'], CRAWLER_SEED_PADDING_BYTES=str(controls['SeedPaddingBytes']), CRAWLER_BATCH_MAX_OPERATIONS='128', CRAWLER_BATCH_MAX_BYTES=str(8 << 20), CRAWLER_MAX_IN_FLIGHT='16', CRAWLER_BATCH_MAX_WAIT='5ms')
    require(receipt['workload_controls'] == legacy.controls_receipt.workload_controls(environ), 'crawler logical units/retry/visibility receipt differs')
    expected_stages = [(PATHS[(round_index+offset)%3], round_index+1) for round_index in range(entry['rounds']) for offset in range(3)]
    require([(stage['Path'], stage['Round']) for stage in report['Stages']] == expected_stages[:len(report['Stages'])], 'crawler stage rotation differs')
    container = owned_fixture(bundle['fixture'], bundle['cleanup'], receipt['servers'][0]['sha256'])
    require(bundle['config']['transport']['max_pending_records'] == 4096 and bundle['fixture']['options']['PendingRecords'] == 4096 and bundle['fixture']['options']['SearchRetryAttempts'] == 4, 'crawler transport/retry limits differ')
    return container


def crawler_stage(stage, report, entry, container):
    require(stage['Profile'] == entry['profile'] and stage['Mode'] == 'fixed-rate' and stage['ProductsPerSecond'] == entry['rate'] and stage['Concurrency'] == 512 and stage['ElapsedNS'] >= entry['duration_ns'], 'crawler stage matrix/duration differs')
    controls = CASES['bounded128'] if stage['Path'] == 'optimized' else CASES['immediate']
    require(stage['BatchMaxWaitNS'] == controls[1], 'stage collection wait differs')
    batching_proof(stage['ServerConfiguration'], (stage['ServerBefore'], stage['ServerAfter']), controls)
    warmup = legacy.call_proof(stage['WarmupClients'], stage, 'warmup')
    require(warmup['failed'] == warmup['unknown'] == warmup['timeout'] == 0, 'crawler warmup failed')
    calls = legacy.call_proof(stage['Clients'], stage, 'timed')
    require(all(before['PID'] == after['PID'] for before, after in zip(stage['WarmupClients'], stage['Clients'])), 'crawler warmup/timed process changed')
    require(stage['ApplicationRequests']['records'] == calls['records'] and stage['ApplicationRequests']['requests'] == calls['requests'], 'crawler actual calls differ')
    require(stage['DrainNS'] == stage['ElapsedNS']-entry['duration_ns'] and math.isclose(stage['CompletedProductsPerSecond'], stage['Completed']/(stage['ElapsedNS']/1e9), rel_tol=1e-12), 'crawler completion/drain denominator differs')
    quota_proof(stage['CPUQuota'], container, stage['Resources'])
    require(helpers.settling(stage['BeforeWarmupSettling']) == helpers.settling(stage['BeforeMeasurementSettling']) == tuple(sorted(stage['DatabaseBefore']['indices'])) == tuple(sorted(stage['DatabaseAfter']['indices'])), 'crawler index/settling identity differs')
    require(stage['VerifiedDocuments'] == (3073 if stage['Verified'] else 0), 'crawler full-field oracle count differs')


def audit_report(report, auditor):
    with tempfile.NamedTemporaryFile(mode='w', suffix='.json') as temporary:
        json.dump(report, temporary)
        temporary.flush()
        result = subprocess.run([str(auditor.resolve()), 'audit', temporary.name], capture_output=True, text=True, timeout=180)
    return dict(exit_code=result.returncode, output=(result.stdout+result.stderr).strip())


def write_clients(result, workers):
    children = result['client_processes'] or []
    require(len(children) == 4 and len({child['pid'] for child in children}) == 4, 'write client processes missing/duplicate')
    counts = ('attempted', 'succeeded', 'errors', 'indeterminate', 'not_attempted', 'applied_with_error', 'business_operation_timeouts', 'logical_request_document_bytes', 'logical_response_document_bytes')
    summed = dict.fromkeys(counts, 0)
    covered, pooled = set(), [0]*256
    maximum = 0
    for index, child in enumerate(children):
        require(child['phase'] == 'timed' and child['pid'] > 0 and child['concurrent_workers'] == workers//4 and child['worker_offset'] == index*(workers//4) and child['native_connection_pool_limit'] == (workers//4 if result['path'] == 'direct' else 0) and child['owned_process_exited'] and not child.get('owned_process_exit_error'), 'write child identity/pool/exit differs')
        value = child['result']
        require(value['attempted'] == value['succeeded']+value['errors']+value['indeterminate'] and child['business_requests'] == value['attempted'] == child['writes'] and child['reads'] == 0, 'write child operation arithmetic differs')
        for key in counts:
            require(type(value[key]) is int and value[key] >= 0, 'invalid write child counter')
            summed[key] += value[key]
        buckets = child['latency_histogram_buckets']
        calculated = helpers.histogram(buckets, value['latency']['max_observed_ns'])
        require(all(value['latency'][key] == val for key, val in calculated.items()) and sum(buckets) == value['attempted'], 'write child latency arithmetic differs')
        pooled = [first+last for first, last in zip(pooled, buckets)]
        maximum = max(maximum, value['latency']['max_observed_ns'])
        require(len(child['expected_record_ranges']) == workers//4, 'write child omitted worker-owned postflight ranges')
        for part in child['expected_record_ranges']:
            require(part['revisions'] and all(type(revision) is int and revision in (0, 1) for revision in part['revisions']), 'invalid expected postflight revision')
            ids = set(range(part['start'], part['start']+len(part['revisions'])))
            require(not covered.intersection(ids) and all(0 <= record < 8192 for record in ids), 'overlapping/out-of-bounds postflight records')
            covered.update(ids)
    require(len(covered) == 8192 and all(result[key] == value for key, value in summed.items()), 'write aggregate or complete seeded postflight coverage differs')
    require(result['planned'] == result['attempted'] == result['client_requests'] == result['writes'] and result['reads'] == 0, 'write aggregate request counts differ')
    calculated = helpers.histogram(pooled, maximum)
    require(all(result['latency'][key] == value for key, value in calculated.items()), 'write pooled latency differs')


def write_identity(report, receipt, entry, case):
    require(report['schema'] == 3 and receipt['test_source'] == CLIENT and receipt['test_source_dirty'] is False and receipt['case'] == case and receipt['phase'] == entry['role'], 'write schema/source/case differs')
    source_identity(receipt['server_receipt'], local=True)
    tree, files = source_snapshot(REPO, CLIENT)
    require(receipt['client_source_tree_sha256'] == tree and receipt['client_source_files'] == files, 'write frozen client Go inputs differ')
    require(receipt['client_binary_sha256'] == receipt['client_binary_after_sha256'] and re.fullmatch('[a-f0-9]{64}', receipt['client_binary_sha256']) and receipt['server_binary_sha256'] == receipt['server_binary_before_sha256'] == receipt['server_binary_after_sha256'] == receipt['server_receipt']['sha256'], 'write binary changed')
    evidence.validate_report_provenance(report, receipt)
    params = report['parameters']
    require(params['concurrency_levels'] == [128, 512] and params['batch_sizes'] == [1] and params['paired_rounds'] == entry['rounds'] and params['requested_write_percent'] == 100 and params['client_process_count'] == 4 and params['records_per_business_request'] == 1 and params['database_cpu_budget_threshold_percent'] == 90 and helpers.duration(params['warmup_duration']) == entry['warmup_ns'] and helpers.duration(params['measurement_duration']) == entry['duration_ns'] and helpers.duration(params['operation_timeout']) == 10**10, 'write workload controls differ')
    dataset = params['dataset']
    require(dataset['backend'] == dataset['store'] == 'search' and dataset['records'] == 8192 and dataset['padding_bytes'] == 1024 and not dataset.get('lua_mutations'), 'pure-write dataset differs')
    operations, wait, inflight = CASES[case]
    provenance = report['provenance']
    expected = dict(test_source=CLIENT, test_source_dirty='false', database_docker_cpu_quota='1', weir_batching_max_operations_override=str(operations), weir_batching_max_bytes_override=str(8 << 20), weir_batching_max_in_flight_override=str(inflight), weir_transport_max_pending_records_override='4096')
    require(all(provenance[key] == value for key, value in expected.items()) and helpers.duration(provenance['weir_batching_max_wait_override']) == wait, 'write provenance batching/quota differs')
    flags = command_flags(receipt['command'])
    expected_flags = {'-mode': 'saturation', '-backend': 'search', '-client-processes': '4', '-concurrency-levels': '128,512', '-batch-sizes': '1', '-records': '8192', '-payload-bytes': '1024', '-write-percent': '100', '-database-cpus': '1', '-pending-records': '4096', '-backend-batch-limit': str(operations), '-backend-batch-bytes': str(8 << 20), '-backend-max-in-flight': str(inflight), '-rounds': str(entry['rounds'])}
    require(all(flags[key] == value for key, value in expected_flags.items()), 'write command differs')
    for flag, value in (('-backend-batch-wait', wait), ('-duration', entry['duration_ns']), ('-warmup-duration', entry['warmup_ns'])):
        require(helpers.duration(flags[flag]) == value, 'write command duration differs')
    expected_pairs = [(workers, round_number) for workers in (128, 512) for round_number in range(1, entry['rounds']+1)]
    require([(pair['concurrency'], pair['round']) for pair in report['pairs']] == expected_pairs[:len(report['pairs'])] and all(pair['batch_size'] == 1 and pair['order'] == (['direct', 'weir'] if ((0 if pair['concurrency'] == 128 else 1)+pair['round']-1)%2 == 0 else ['weir', 'direct']) for pair in report['pairs']), 'write paired stage matrix/order differs')


def write_points(report):
    if report.get('incomplete'):
        return
    points = report['points']
    require(len(points) == 4 and {(point['path'], point['concurrency']) for point in points} == {(path, workers) for path in ('direct', 'weir') for workers in (128, 512)}, 'write aggregate points missing/duplicate')
    for point in points:
        results = [pair[point['path']] for pair in report['pairs'] if pair['concurrency'] == point['concurrency']]
        require(len(results) == report['parameters']['paired_rounds'] and point['batch_size'] == 1, 'write aggregate point round count differs')
        qps = sum(result['succeeded'] for result in results)*1e9/sum(result['elapsed_ns'] for result in results)
        mean = sum(result['resources']['mean_database_cpu_budget_percent'] for result in results)/len(results)
        verified = all(result['verified'] and result['planned'] > 0 and result['attempted'] == result['planned'] == result['succeeded'] and result['errors'] == result['indeterminate'] == result['not_attempted'] == 0 and result['elapsed_ns'] > 0 for result in results)
        saturated = all(result['resources']['sustained_cpu_saturation'] for result in results)
        require(math.isclose(point['successful_operations_per_second'], qps, rel_tol=1e-12) and math.isclose(point['mean_database_cpu_budget_percent'], mean, rel_tol=1e-12) and point['all_rounds_successful_verified'] == verified and point['all_rounds_cpu_saturated'] == saturated, 'write aggregate point arithmetic differs')
        following = next((item for item in points if item['path'] == point['path'] and item['concurrency'] == 512), None) if point['concurrency'] == 128 else None
        plateau = bool(following and verified and following['all_rounds_successful_verified'] and qps > 0 and following['successful_operations_per_second'] <= qps*1.1)
        require(point['throughput_plateau_at_next_concurrency'] == plateau and point['database_saturation_demonstrated'] == bool(verified and saturated and plateau), 'reported write aggregate plateau differs; independent per-round capacity remains separate')


def comparisons(rows, entries):
    output = []
    for entry in entries:
        if entry['role'] != 'formal':
            continue
        pairs_to_check = [('direct', 'weir')] if entry['kind'] == 'write' else [('direct', 'current'), ('direct', 'optimized'), ('current', 'optimized')]
        for workers in ((128, 512) if entry['kind'] == 'write' else (512,)):
            for baseline, candidate in pairs_to_check:
                paired = []
                for round_number in (1, 2, 3):
                    sides = [next((row for row in rows if row['label'] == entry['label'] and row['workers'] == workers and row['round'] == round_number and row['path'] == path), None) for path in (baseline, candidate)]
                    first, last = sides
                    ratio = lambda key: last[key]/first[key] if first and last and first.get(key) and last.get(key) is not None else None
                    qualified = bool(first and last and all(row['formal_measurement'] if entry['kind'] == 'write' else row['formal_all_arrivals'] for row in sides))
                    paired.append(dict(round=round_number, both_qualified=qualified, qps_ratio=ratio('completed_s'), cpu_ms_per_completed_ratio=ratio('db_cpu_ms_per_completed'), actual_db_cpu_ratio=ratio('db_cpu_quota_percent')))
                strict = all(pair['both_qualified'] and pair['qps_ratio'] is not None and .95 <= pair['qps_ratio'] <= 1.05 and pair['cpu_ms_per_completed_ratio'] is not None and pair['cpu_ms_per_completed_ratio'] < 1 for pair in paired)
                equal_load = entry['kind'] == 'write' and all(pair['both_qualified'] and pair['qps_ratio'] is not None and pair['qps_ratio'] > 1 and pair['actual_db_cpu_ratio'] is not None and pair['actual_db_cpu_ratio'] <= 1 for pair in paired)
                capacity = False
                if entry['kind'] == 'write' and workers == 128:
                    capacity = all(capacity_gate(rows, entry['label'], path) for path in (baseline, candidate))
                output.append(dict(label=entry['label'], kind=entry['kind'], workers=workers, baseline=baseline, candidate=candidate, paired_rounds=paired, supports_lower_cpu_at_comparable_completed_qps=strict if entry['kind'] == 'crawler' else False, higher_write_qps_at_no_more_measured_db_cpu=equal_load, both_paths_database_bottleneck_and_plateau=capacity, supports_equal_load_capacity_increase=bool(capacity and equal_load)))
    return output


def capacity_gate(rows, label, path):
    for round_number in (1, 2, 3):
        first = next((row for row in rows if row['label'] == label and row['path'] == path and row['workers'] == 128 and row['round'] == round_number), None)
        following = next((row for row in rows if row['label'] == label and row['path'] == path and row['workers'] == 512 and row['round'] == round_number), None)
        if not first or not following or not first['formal_measurement'] or not following['formal_measurement'] or not first['sustained_database_cpu_saturation'] or not first['completed_s'] or following['completed_s'] > first['completed_s']*1.1:
            return False
    return True


def validate_manifest(manifest):
    require(manifest['client_source'] == CLIENT and manifest['client_ref'] == CLIENT_REF and manifest['server_sources'] == dict(current=SERVER, optimized=SERVER), 'frozen source identities differ')
    entries = manifest['entries']
    expected = {('calibration', 'write', None), ('formal', 'write', None), ('diagnostic', 'crawler', 'partial-update'), ('diagnostic', 'crawler', 'two-step'), ('formal', 'crawler', 'partial-update'), ('formal', 'crawler', 'two-step')}
    require(len(entries) == 6 and len({entry['label'] for entry in entries}) == 6 and {(entry['role'], entry['kind'], entry.get('profile')) for entry in entries} == expected, 'six-workflow inventory differs')
    for entry in entries:
        formal = entry['role'] == 'formal'
        require(entry['warmup_ns'] == (60 if formal else 2)*10**9 and entry['duration_ns'] == (60 if formal else 5)*10**9 and entry['rounds'] == (3 if formal else 1), 'fixed stage duration/rounds differ')
        expected_stages = (12 if formal else 24) if entry['kind'] == 'write' else (9 if formal else 3)
        require(entry['planned_timed_stages'] == expected_stages and re.fullmatch('[a-z0-9-]+', entry['label']), 'fixed stage count/label differs')
        if entry['kind'] == 'crawler':
            require(entry['rate'] == (512 if entry['profile'] == 'partial-update' else 68) and entry['http_observer'] == ('disabled' if formal else 'enabled'), 'crawler rate/observer differs')


def workflow_proof(folder, entry, manifest):
    files = [folder/name for name in ('workflow.json', 'workflow-run.json') if (folder/name).exists()]
    if files:
        require(len(files) == 1, 'ambiguous workflow metadata')
        workflow = load(files[0])
    else:
        names = ('workflow.json.gz', 'workflow-run.json.gz')
        matches = [path for name in names for path in folder.glob(name)]
        require(len(matches) == 1, 'workflow gzip metadata missing/ambiguous')
        workflow = load(matches[0])
    require(workflow['headSha'] == CLIENT and workflow['headBranch'] == CLIENT_REF and workflow['url'] == entry['workflow_run'] and workflow['status'] == 'completed', 'workflow source/identity/completion differs')
    dispatched = [item for item in load(FOLDER/'dispatch-receipts.json') if item['label'] == entry['label'] and item['url'] == entry['workflow_run']]
    require(len(dispatched) == 1 and dispatched[0]['head_sha'] == CLIENT and dispatched[0]['request']['ref'] == CLIENT_REF and dispatched[0]['created_at'] == workflow['createdAt'], 'prospective dispatch receipt differs')
    return workflow


def new_row(entry, path, workers, round_number, case=None):
    return dict(label=entry['label'], role=entry['role'], kind=entry['kind'], case=case, workflow_run=entry['workflow_run'], path=path, workers=workers, round=round_number, run_checks_passed=False, stage_checks_passed=False, stage_check_error=None, verified=False, failed=None, unknown=None, timeout=None, completed=None, completed_s=None, db_cpu_coverage=0, db_cpu_ms_per_completed=None, db_cpu_quota_percent=None, sustained_database_cpu_saturation=False, formal_measurement=False, formal_all_arrivals=False)


def process_crawler(folder, entry, manifest, auditor):
    bundle = dict(entry=entry, **{key: load(one(folder, name)) for key, name in dict(report='crawler.json.gz', receipt='crawler.receipt.json.gz', fixture='manifest.json.gz', cleanup='cleanup.json.gz', routes='node-0-routes.json.gz', config='node-0-config.json.gz').items()})
    report, receipt = bundle['report'], bundle['receipt']
    require(receipt['workflow_run'] == entry['workflow_run'], 'crawler receipt workflow differs')
    error, container = None, None
    try:
        container = crawler_identity(bundle, manifest)
    except (ValueError, KeyError, TypeError) as failure:
        error = str(failure)
    audit = audit_report(report, auditor)
    complete = len(report['Stages']) == entry['planned_timed_stages'] and not report.get('incomplete') and not report.get('cleanup_error')
    passed = complete and error is None and audit['exit_code'] == 0 and receipt['exit_code'] == 0
    run = dict(provenance_error=error, complete=complete, runner_exit=receipt['exit_code'], original_go_audit=audit, checks_passed=passed)
    rows, diagnostics = [], []
    for stage in report['Stages']:
        row = new_row(entry, stage['Path'], stage['Concurrency'], stage['Round'])
        row.update(run_checks_passed=passed, verified=stage['Verified'], failed=stage['Failed'], unknown=stage['Indeterminate'], timeout=stage['TimedOut'], completed=stage['Completed'], rejected=stage['Rejected'], offered=stage['Offered'], p99_ms=stage['ScheduledToVisible']['p99_upper_bound_ns']/1e6, drain_s=stage['DrainNS']/1e9, max_generator_lag_ms=stage['GeneratorLagNS']/1e6, elapsed_s=stage['ElapsedNS']/1e9, verified_documents=stage['VerifiedDocuments'], expected_documents_sha256=stage['ExpectedDocumentsSHA256'])
        row['completed_s'] = stage['Completed']/(stage['ElapsedNS']/1e9) if stage['ElapsedNS'] > 0 else None
        try:
            crawler_stage(stage, report, entry, container)
            row.update(cpu_observation(stage['Resources'], stage['ElapsedNS'], stage['Completed']))
            metrics = phase_observations(stage)
            row.update(metrics)
            if stage['Path'] != 'direct' and stage['Completed'] > 0:
                require(metrics['search_bulk_attempts'] is not None and metrics['search_bulk_attempts'] > 0 and metrics['search_bulk_body_bytes'] is not None and metrics['search_bulk_body_bytes'] > 0, 'active bulk attempt/body measurements unavailable')
            row.update(helpers.adapter_metrics(stage))
            row['stage_checks_passed'] = True
            if entry['http_observer'] == 'enabled':
                diagnostics.append(dict(label=entry['label'], path=stage['Path'], round=stage['Round'], **diagnostic_counts(stage, metrics)))
            else:
                require(all(stage[key] is None for key in ('HTTPBefore', 'HTTPAfter', 'HTTPDelta')), 'formal observer unexpectedly enabled')
        except (ValueError, KeyError, TypeError, ZeroDivisionError) as failure:
            row['stage_checks_passed'] = False
            row['stage_check_error'] = str(failure)
        formal_gate(row)
        rows.append(row)
    return run, rows, diagnostics


def process_write(folder, entry):
    rows, cases = [], []
    names = list(CASES) if entry['role'] == 'calibration' else ['bounded128']
    for case in names:
        try:
            report = load(one(folder, case+'/search.json.gz'))
            receipt = load(one(folder, case+'.receipt.json.gz'))
        except (ValueError, KeyError, OSError) as failure:
            cases.append(dict(case=case, checks_passed=False, provenance_error=str(failure), complete=False, runner_exit=None, reported_points=None))
            continue
        run_error, container = None, None
        try:
            require(receipt['workflow_run'] == entry['workflow_run'], 'write receipt workflow differs')
            write_identity(report, receipt, entry, case)
            write_points(report)
            fixture_folder = report['provenance']['fixture_logs'].strip('/')
            fixture = load(one(folder, fixture_folder+'/manifest.json.gz'))
            cleanup = load(one(folder, fixture_folder+'/cleanup.json.gz'))
            routes = load(one(folder, fixture_folder+'/node-0-routes.json.gz'))
            config = load(one(folder, fixture_folder+'/node-0-config.json.gz'))
            container = owned_fixture(fixture, cleanup, receipt['server_binary_sha256'])
            require(config['transport']['max_pending_records'] == 4096, 'write raw transport limit differs')
            require(len(routes['stores']) == 1, 'write route missing')
            # Raw route and both live boundary gauges are checked on each Weir stage.
        except (ValueError, KeyError, TypeError) as failure:
            run_error = str(failure)
        complete = not report.get('incomplete') and len(report['pairs']) == 2*entry['rounds']
        passed = complete and run_error is None and receipt['exit_code'] == 0
        cases.append(dict(case=case, checks_passed=passed, provenance_error=run_error, complete=complete, runner_exit=receipt['exit_code'], reported_points=report.get('points')))
        for pair in report['pairs']:
            for path in pair['order']:
                result = pair[path]
                row = new_row(entry, path, pair['concurrency'], pair['round'], case)
                row.update(run_checks_passed=passed, verified=result['verified'], failed=result['errors'], unknown=result['indeterminate'], timeout=result['business_operation_timeouts'], completed=result['succeeded'], elapsed_s=result['elapsed_ns']/1e9, p99_ms=result['latency']['p99_upper_bound_ns']/1e6)
                row['completed_s'] = result['succeeded']/(result['elapsed_ns']/1e9) if result['elapsed_ns'] > 0 else None
                try:
                    require(result['path'] == path and result['elapsed_ns'] >= entry['duration_ns'] and math.isclose(result['successful_operations_per_second'], row['completed_s'], rel_tol=1e-12), 'write stage path/time/throughput differs')
                    require(not result.get('verification_error') and result['planned'] > 0 and result['not_attempted'] == 0 and result['applied_with_error'] == 0, 'write stage incomplete/postflight failed')
                    write_clients(result, pair['concurrency'])
                    quota_proof(report['database_cpu_quota'], container, result['resources'])
                    row.update(cpu_observation(result['resources'], result['elapsed_ns'], result['succeeded']))
                    if path == 'weir':
                        metrics = result['server_metrics']
                        batching_proof(routes, (metrics['before'], metrics['after']), CASES[case])
                        stage = dict(ServerBefore=metrics['before'], ServerAfter=metrics['after'])
                        row.update(phase_observations(stage))
                        require(row['search_bulk_attempts'] is not None and row['search_bulk_attempts'] > 0 and row['search_bulk_records'] == result['attempted'], 'pure-write actual bulk attempts/records unavailable or differ from single-call attempts')
                        row['reported_adapter_batch_average'] = metrics.get('timed_adapter_batch_average')
                    row['stage_checks_passed'] = True
                except (ValueError, KeyError, TypeError, ZeroDivisionError) as failure:
                    row['stage_check_error'] = str(failure)
                formal_gate(row)
                rows.append(row)
    return dict(checks_passed=all(case['checks_passed'] for case in cases), cases=cases), rows, []


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, default=FOLDER/'manifest.json')
    parser.add_argument('--auditor', type=Path, required=True)
    parser.add_argument('--output', type=Path, default=FOLDER/'analysis')
    parser.add_argument('--partial', action='store_true', help='include every planned workflow; mark unavailable or unfinished evidence explicitly')
    args = parser.parse_args()
    manifest = load(args.manifest)
    validate_manifest(manifest)
    for name in ('scripts/summarize_crawler.py', 'scripts/run_crawler.py', 'scripts/performance_evidence.py'):
        require((REPO/name).read_bytes() == subprocess.check_output(['git', 'show', CLIENT+':'+name], cwd=REPO), 'analysis helper differs from original client: '+name)
    auditor_receipt = load(FOLDER/'auditor-build.json')
    require(auditor_receipt['base_source'] == CLIENT and auditor_receipt['unmodified_source_archive'] and auditor_receipt['binary_sha256'] == digest(args.auditor.read_bytes()), 'original crawler auditor build/source/binary differs')
    source_archive = subprocess.check_output(['git', 'archive', '--format=tar', CLIENT, '--', *auditor_receipt['source_archive_scope']], cwd=REPO)
    require(digest(source_archive) == auditor_receipt['source_archive_sha256'], 'original auditor source archive hash differs from immutable client')
    runs, rows, diagnostics = [], [], []
    for entry in manifest['entries']:
        folder = FOLDER/'raw'/entry['label']
        run = dict(label=entry['label'], role=entry['role'], kind=entry['kind'], workflow_run=entry['workflow_run'], evidence_available=False, checks_passed=False, error=None)
        if not entry['workflow_run'] or not (folder/'files.json').exists():
            require(args.partial, 'planned workflow evidence missing: '+entry['label'])
            run['error'] = 'workflow not dispatched or retained evidence not yet available'
            runs.append(run)
            continue
        try:
            run['archive_files'] = verify_archive(folder)
            workflow = workflow_proof(folder, entry, manifest)
            run['workflow_conclusion'] = workflow['conclusion']
            analysis, stage_rows, diagnostic_rows = process_write(folder, entry) if entry['kind'] == 'write' else process_crawler(folder, entry, manifest, args.auditor)
            run.update(analysis, evidence_available=True)
            run['checks_passed'] = bool(run['checks_passed'] and workflow['conclusion'] == 'success')
            for row in stage_rows:
                row['run_checks_passed'] = row['run_checks_passed'] and run['checks_passed']
                formal_gate(row)
            rows.extend(stage_rows)
            diagnostics.extend(diagnostic_rows)
        except (ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError) as failure:
            run['error'] = str(failure)
        runs.append(run)
    compared = comparisons(rows, manifest['entries'])
    output = dict(schema=1, server_source=SERVER, client_source=CLIENT, original_auditor_source=CLIENT, original_auditor_sha256=digest(args.auditor.read_bytes()), partial=args.partial, complete_workflow_inventory=len(runs) == 6 and all(run['evidence_available'] for run in runs), runs=runs, diagnostic_count_checks=diagnostics, comparisons=compared, rows=rows, analysis_source_sha256=digest(Path(__file__).read_bytes()))
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output/'analysis.json').write_text(json.dumps(output, indent=2, allow_nan=False)+'\n')
    for name, values in (('runs', runs), ('stages', rows), ('comparisons', compared)):
        flattened = [{key: json.dumps(value, sort_keys=True) if isinstance(value, (dict, list)) else value for key, value in row.items()} for row in values]
        helpers.csv_write(args.output/(name+'.csv'), flattened)
    table = ['| Workflow | Evidence available | Source / archive / cleanup checks | Qualified formal stages | Error |', '| --- | --- | --- | ---: | --- |']
    for run in runs:
        qualified = sum(row['formal_measurement'] for row in rows if row['label'] == run['label'])
        table.append('| '+run['label']+' | '+str(run['evidence_available'])+' | '+str(run['checks_passed'])+' | '+str(qualified)+' | '+str(run.get('error') or run.get('provenance_error') or '').replace('|', '\\|')+' |')
    (args.output/'acceptance.md').write_text('\n'.join(table)+'\n')
    print(json.dumps(dict(workflows=len(runs), available=sum(run['evidence_available'] for run in runs), stages=len(rows), qualified_formal=sum(row['formal_measurement'] for row in rows))))


if __name__ == '__main__':
    main()
