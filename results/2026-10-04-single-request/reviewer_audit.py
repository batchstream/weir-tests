#!/usr/bin/env python3
"""Independently recompute single-request evidence and same-job controls."""
import argparse
import base64
import json
import math
import re
from pathlib import Path


SDK = 'v0.4.2'
PROTOCOL = 'v0.2.1'


def require(condition, description):
    if not condition:
        raise ValueError(description)


def near(actual, expected, description):
    require(math.isclose(actual, expected, rel_tol=1e-9, abs_tol=1e-8),
            f'{description}: {actual} != {expected}')


def load(path):
    return json.loads(path.read_text())


def quantile(buckets, percent, maximum):
    rank = (sum(buckets) * percent + 99) // 100
    observed = 0
    for index, count in enumerate(buckets):
        observed += count
        if observed < rank:
            continue
        if index == 0:
            return 0
        if index == 255:
            return maximum
        exponent, fraction = divmod(index - 1, 8)
        base = 1 << exponent
        return max((base + ((fraction + 1) * base + 7) // 8 - 1) * 1000, 1000)
    return maximum


def check_latency(latency, buckets, maximum):
    require(len(buckets) == 256 and all(isinstance(n, int) and n >= 0 for n in buckets), 'valid latency histogram')
    require(latency['samples'] == sum(buckets) and latency['max_observed_ns'] == maximum, 'pooled histogram count/max')
    for percent in (50, 95, 99):
        require(latency[f'p{percent}_upper_bound_ns'] == quantile(buckets, percent, maximum), 'pooled histogram quantile')


def parse_prometheus(raw):
    series = []
    for line in raw.splitlines():
        if not line or line.startswith('#'):
            continue
        match = re.fullmatch(r'([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{(.*)\})?\s+(\S+)(?:\s+\S+)?', line)
        require(match is not None, 'valid raw Prometheus sample')
        name, text, value = match.groups()
        labels = {}
        if text:
            for label in re.finditer(r'([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\.|[^"\\])*)"', text):
                require(label.group(1) not in labels, 'unique raw Prometheus label')
                labels[label.group(1)] = json.loads('"' + label.group(2) + '"')
        entry = {'name': name, 'labels': labels, 'value': float(value)}
        series.append(entry)
    return series


def metric(series, name, labels):
    values = [sample['value'] for sample in series if sample['name'] == name
              and all(sample['labels'].get(key) == value for key, value in labels.items())]
    return sum(values) if values else None


def cpu(resources, threshold):
    elapsed, budget = resources['measurement_elapsed_ns'], resources['database_allocated_cpus']
    require(elapsed > 0 and budget == 1, 'actual CPU denominator')
    intervals = valid = full = 0
    weighted = 0.0
    for left, right in zip(resources['samples'], resources['samples'][1:]):
        span = min(right['elapsed_ns'], elapsed) - max(left['elapsed_ns'], 0)
        clock = right['elapsed_ns'] - left['elapsed_ns']
        if left.get('database_cpu_counter_clock_unix_ns') and right.get('database_cpu_counter_clock_unix_ns'):
            clock = right['database_cpu_counter_clock_unix_ns'] - left['database_cpu_counter_clock_unix_ns']
        if span <= 0 or clock <= 0 or 'database_cpu_time_ns' not in left or 'database_cpu_time_ns' not in right:
            continue
        delta = right['database_cpu_time_ns'] - left['database_cpu_time_ns']
        if delta < 0:
            continue
        value = delta / clock / budget * 100
        if 'database_cpu_budget_percent' in right:
            near(right['database_cpu_budget_percent'], value, 'raw Docker interval CPU')
        if left.get('error') or right.get('error'):
            continue
        intervals += 1
        valid += span
        weighted += value * span
        if value >= threshold:
            full += span
    mean = weighted / valid if valid else 0
    coverage, fraction = valid / elapsed, full / elapsed
    near(resources['mean_database_cpu_budget_percent'], mean, 'duration weighted CPU')
    near(resources['cpu_sampling_time_coverage'], coverage, 'CPU coverage')
    near(resources['fraction_intervals_at_cpu_threshold'], fraction, 'full measurement CPU fraction')
    require(resources['valid_cpu_intervals'] == intervals and resources['valid_cpu_sampling_elapsed_ns'] == valid, 'CPU interval count/time')
    qualified = intervals >= 5 and coverage >= .8 and mean >= threshold and fraction >= .8
    require(resources['sustained_cpu_saturation'] == qualified, 'independent CPU qualification')
    summary = {'qualified': qualified, 'mean_percent': mean, 'coverage': coverage, 'threshold_fraction': fraction,
               'cpu_error_samples': sum(bool(s.get('error')) for s in resources['samples']),
               'database_counter_error_samples': sum(bool(s.get('database_counter_error')) for s in resources['samples']),
               'optional_storage_unavailable_samples': sum(bool(s['database_counters'].get('storage_observation_unavailable')) for s in resources['samples'])}
    return summary


def check_server(stage, backend, limit):
    metrics = stage['server_metrics']
    require(not metrics.get('unavailable'), 'available server metrics')
    before = parse_prometheus(metrics['before']['raw'])
    after = parse_prometheus(metrics['after']['raw'])
    delta = metrics['timed_counter_deltas']
    for name, value in delta.items():
        if name.startswith('weir_rpc_completions_total:'):
            base, method = name.split(':')
            labels = {'method': method}
        else:
            base, labels = name, {'store': backend}
        start, finish = metric(before, base, labels), metric(after, base, labels)
        require(start is not None and finish is not None and finish >= start, 'raw metric present without reset')
        near(value, finish - start, 'counter recomputed from raw Prometheus')
    labels = {'store': backend}
    require(metric(before, 'weir_store_concurrency_limit', labels) == metric(after, 'weir_store_concurrency_limit', labels)
            == metrics['configured_backend_concurrency_limit'] == 32, 'stable actual configured permits')
    for method, calls in [('read', stage['reads']), ('mutate', stage['writes'])]:
        require(delta.get('weir_rpc_completions_total:' + method, 0) == calls, 'one measured record per actual RPC')
    count = delta['weir_store_batch_operations_count']
    require(count > 0 and delta['weir_store_records_total'] == delta['weir_store_batch_operations_sum'] == stage['client_requests'], 'all admitted records accounted')
    require(delta['weir_store_rejections_total'] == 0, 'zero measured rejections')
    average = stage['client_requests'] / count
    near(metrics['timed_adapter_batch_average'], average, 'batch average')
    near(metrics['timed_business_rpcs_per_adapter_invocation'], average, 'RPC/adapter ratio')
    require(1 <= average <= limit, 'actual batch count bound')
    buckets = metrics['timed_adapter_batch_cumulative_buckets']
    for boundary, value in buckets.items():
        labels = {'store': backend, 'le': boundary}
        start, finish = metric(before, 'weir_store_batch_operations_bucket', labels), metric(after, 'weir_store_batch_operations_bucket', labels)
        require(start is not None and finish is not None, 'raw batch histogram available')
        near(value, finish - start, 'raw batch histogram delta')
    near(buckets['+Inf'], count, 'histogram invocation count')
    ordered = [value for key, value in sorted(buckets.items(), key=lambda pair: float(pair[0]))]
    require(all(a <= b for a, b in zip(ordered, ordered[1:])), 'cumulative histogram monotonicity')
    if limit == 1:
        near(average, 1, 'one-record grouping control')
        near(buckets['1'], count, 'all control invocations contain one record')
    summary = {'invocations': count, 'records': stage['client_requests'], 'average': average,
               'one_record_invocations': buckets['1'], 'multi_record_invocations': count - buckets['1']}
    return summary


def check_stage(stage, options):
    concurrency, backend, limit = options['concurrency'], options['backend'], options['limit']
    records, threshold = options['records'], options['threshold']
    requests = stage['client_requests']
    require(requests > 0 and stage['planned'] == stage['attempted'] == stage['succeeded'] == requests == stage['reads'] + stage['writes'], 'single-record successful business counts')
    require(stage['verified'] and not stage.get('verification_error'), 'persisted postflight passed')
    for key in ('errors', 'indeterminate', 'not_attempted', 'applied_with_error', 'business_operation_timeouts'):
        require(stage[key] == 0, 'zero ' + key)
    require(stage['elapsed_ns'] >= 20_000_000_000, 'includes common measurement window and all joins')
    near(stage['successful_operations_per_second'], requests * 1e9 / stage['elapsed_ns'], 'stage QPS')
    children = stage['client_processes']
    require(len(children) == 4 and len({c['pid'] for c in children}) == 4 and all(c['pid'] > 0 for c in children), 'four separate client PIDs')
    require(sum(c['business_requests'] for c in children) == requests and sum(c['reads'] for c in children) == stage['reads'] and sum(c['writes'] for c in children) == stage['writes'], 'child operation ownership')
    buckets, maximum, covered = [0] * 256, 0, set()
    for index, child in enumerate(children):
        require(child['owned_process_exited'] and not child.get('owned_process_exit_error'), 'owned client exited')
        require(child['phase'] == 'timed' and 0 <= child['start_lag_ns'] <= 100_000_000, 'common barrier and no late worker')
        require(child['worker_offset'] == index * (concurrency // 4) and child['concurrent_workers'] == concurrency // 4, 'disjoint worker ownership')
        require(child['native_connection_pool_limit'] == (concurrency // 4 if stage['path'] == 'direct' else 0), 'native independent pool bound')
        result = child['result']
        require(result['planned'] == result['attempted'] == result['succeeded'] == child['business_requests'], 'child successful requests')
        require(child['business_requests'] == child['reads'] + child['writes'], 'child one-record calls')
        for key in ('errors', 'indeterminate', 'not_attempted', 'applied_with_error', 'business_operation_timeouts'):
            require(result[key] == 0, 'zero child ' + key)
        child_buckets = child['latency_histogram_buckets']
        require(sum(child_buckets) == child['business_requests'], 'each business call has one latency sample')
        check_latency(result['latency'], child_buckets, result['latency']['max_observed_ns'])
        buckets = [a + b for a, b in zip(buckets, child_buckets)]
        maximum = max(maximum, result['latency']['max_observed_ns'])
        for record_range in child['expected_record_ranges']:
            start, revisions = record_range['start'], record_range['revisions']
            require(start >= 0 and start + len(revisions) <= records and all(n in (0, 1) for n in revisions), 'real final revisions')
            ids = set(range(start, start + len(revisions)))
            require(not covered.intersection(ids), 'no overlapping client record partition')
            covered.update(ids)
    require(covered == set(range(records)), 'every stored record independently covered')
    check_latency(stage['latency'], buckets, maximum)
    resources = stage['resources']
    require(resources['measurement_elapsed_ns'] == stage['elapsed_ns'], 'common resource measurement denominator')
    pids = {child['pid'] for child in children}
    for sample in resources['samples']:
        require({c['pid'] for c in sample['client_processes']} == pids, 'actual per-process resource identities')
        require(not any(c.get('error') for c in sample['client_processes']), 'client resource observations available')
    cpu_evidence = cpu(resources, threshold)
    excess = {}
    if backend == 'mongo':
        require(not stage.get('physical_database_command_observation_unavailable'), 'Mongo physical observation available')
        before, after = stage['timed_database_counters_before'], stage['timed_database_counters_after']
        commands = stage['timed_physical_database_command_deltas']
        for name in ('find', 'update', 'bulkWrite', 'getMore', 'killCursors'):
            require(commands[name] == after['physical_command_totals'][name] - before['physical_command_totals'][name] >= 0, 'actual global command delta')
        if stage['path'] == 'direct':
            require(commands['find'] >= stage['reads'] and commands['update'] >= stage['writes'] and commands['bulkWrite'] == 0, 'native single-operation calls, no native BulkWrite')
            excess = {'find': commands['find'] - stage['reads'], 'update': commands['update'] - stage['writes']}
    else:
        require(bool(stage.get('physical_database_command_observation_unavailable')), 'ES wire command totals explicitly unavailable')
    adapter = check_server(stage, backend, limit) if stage['path'] == 'weir' else None
    if backend == 'mongo' and adapter is not None and (not stage['reads'] or not stage['writes']):
        excess = {'find_plus_bulkWrite': commands['find'] + commands['bulkWrite'] - adapter['invocations'], 'update': commands['update']}
        require(excess['find_plus_bulkWrite'] >= 0, 'pure adapter physical groups accounted')
    summary = {'requests': requests, 'elapsed_ns': stage['elapsed_ns'], 'buckets': buckets, 'max': maximum,
               'cpu': cpu_evidence, 'adapter': adapter, 'global_command_excess': excess}
    return summary


def nearest_run(report_file, root):
    for parent in [report_file.parent] + list(report_file.parents):
        if parent == root.parent:
            break
        candidate = parent / 'run.json'
        if candidate.exists():
            return candidate
    return None


def recipe(report_file, report, expected):
    limit = int(report['provenance']['weir_max_batch_operations'])
    path = report_file.parent.parent / f'measurement-batch-{limit}.receipt.json'
    require(path.exists(), 'exact executed measurement receipt')
    receipt = load(path)
    require(receipt['source'] == expected['head'] and receipt['source_dirty'] is False, 'clean measured recipe')
    require(receipt['client_operation_timeout'] == receipt['weir_backend_timeout'] == '10s', 'explicit common operation/backend timeout receipt')
    versions = receipt['versions']
    require(versions['weir_source'] == expected['server'] and versions['sdk'] == SDK and versions['protocol'] == PROTOCOL, 'official dependency pins')
    require(versions['weir_version'] == expected['version'] and versions['weir_sum'] == expected['sum'], 'official immutable server module checksum')
    require('vcs.revision=' + expected['head'] in receipt['client_build'] and 'vcs.modified=false' in receipt['client_build'], 'actual built client revision')
    require('\tdep\tgithub.com/batchstream/weir-go\t' + SDK + '\th1:2YyYIXgREc4lOind9xIGAQedleGf10KsxscsZR9rI8w=' in receipt['client_build']
            and '\tdep\tgithub.com/batchstream/weir-protocol\t' + PROTOCOL + '\th1:fENLtcrDR7sSco2R/Xeq46X6rhJYli8fdmsFWl4iX0Q=' in receipt['client_build'], 'built official SDK/protocol modules')
    command = receipt['command']
    options = dict(zip(command[1::2], command[2::2]))
    require(options['-mode'] == 'saturation' and options['-backend'] == 'all' and options['-database-cpus'] == '1' and options['-store-concurrency'] == '32', 'exact executed recipe')
    params = report['parameters']
    require(options['-backend-batch-limit'] == str(limit) and options['-client-processes'] == '4' and options['-batch-sizes'] == '1', 'one-record client recipe')
    require(options['-concurrency-levels'] == ','.join(map(str, params['concurrency_levels'])) and options['-write-percent'] == str(params['requested_write_percent']), 'executed ladder and load')
    require(options['-rounds'] == '3' and options['-warmup-duration'] == '10s' and options['-duration'] == '20s' and options['-records'] == '2048' and options['-payload-bytes'] == '1024', 'exact data and duration recipe')
    return receipt


def fixture(root, report):
    provenance, params = report['provenance'], report['parameters']
    matches = [p for p in root.rglob('manifest.json') if p.parent.name == Path(provenance['fixture_logs']).name]
    require(len(matches) == 1, 'one owned fixture manifest')
    path = matches[0]
    manifest, cleanup = load(path), load(path.parent / 'cleanup.json')
    options = manifest['options']
    limit = int(provenance['weir_max_batch_operations'])
    sessions = max(64, max(params['concurrency_levels']))
    working = {'mongo': 1312, 'search': 3072}
    memory = ((sessions * 96 + 1024 + sum(working.values()) + 128 + 1023) // 1024) * 1024
    require(options['Backends'] == ['mongo', 'search'] and options['OwnerCount'] == 1 and not options['DiscoveryOnly'] and not options['MongoBinary'], 'owned Docker topology')
    require(options['StoreConcurrency'] == 32 and options['BatchSize'] == limit and options['IngressSessions'] == sessions and options['DatabaseCPUs'] == 1, 'independent grouping/permit/session settings')
    require(options['BackendTimeout'] == 10_000_000_000, 'actual fixture backend budget equals client operation budget')
    require(options['WorkingMemoryMiB'] == working and options['ProcessMemoryMiB'] == memory, 'actual byte budgets')
    require(manifest['binary_sha256'] == provenance['weir_binary_sha256'] and manifest['host_os'] == 'linux' and manifest['host_arch'] == 'amd64', 'production binary identity')
    config = load(path.parent / 'node-0-config.json')
    require(config['memory'] == str(memory) + 'MiB' and config['transport'] == {'max_connections': 64, 'max_sessions': sessions}, 'actual process admission configuration')
    routes = load(path.parent / 'node-0-routes.json')
    require({store['name'] for store in routes['stores']} == {'mongo', 'search'}, 'exact store set')
    for store in routes['stores']:
        require(store['max_concurrency'] == 32 and store['max_batch_operations'] == limit and store['working_memory'] == str(working[store['name']]) + 'MiB' and store['max_read_size'] == '2MiB', 'actual per-store configuration')
        require(store['backend_timeout'] == '10s', 'actual per-store backend timeout')
    containers = {c['id']: c for c in manifest['containers']}
    quota = report['database_cpu_quota']
    require(quota['container_id'] in containers and containers[quota['container_id']]['backend'] == params['dataset']['backend'], 'CPU quota actual owned identity')
    require(quota['requested_cpus'] == quota['observed_cpus'] == 1 and quota['host_config_nano_cpus'] == 1_000_000_000, 'actual 1 CPU quota')
    require(len(containers) == 2 and {c['backend'] for c in containers.values()} == {'mongo', 'search'}, 'exact owned database set')
    for container in containers.values():
        pin = 'mongodb_image' if container['backend'] == 'mongo' else 'elasticsearch_image'
        require(container['image'] == provenance[pin] and container['name'].startswith('weir-tests-' + manifest['owner'] + '-'), 'owned immutable database image')
    require(cleanup['owner'] == manifest['owner'] and not cleanup['errors'] and {c['id'] for c in cleanup['containers']} == set(containers), 'complete owned cleanup identity')
    require(all(c['removed'] for c in cleanup['containers']) and len(cleanup['processes']) == 1 and all(p['stopped'] for p in cleanup['processes']), 'all owned resources cleaned')
    return manifest


def check_selected(actual, expected, label):
    if expected is None:
        require(actual is None, label + ' absent unless qualified')
        return
    require(actual is not None and set(actual) == set(expected), label + ' complete point')
    for key, value in expected.items():
        if isinstance(value, float):
            near(actual[key], value, label + ' ' + key)
        else:
            require(actual[key] == value, label + ' ' + key)


def check_report_header(report, expected):
    params, provenance = report['parameters'], report['provenance']
    require(report['schema'] == 3, 'single-request report schema')
    require(provenance['test_source'] == expected['head'] and provenance['test_source_dirty'] == 'false' and provenance['weir_source'] == expected['server'] and provenance['sdk'] == SDK and provenance['protocol'] == PROTOCOL, 'fixed public measured identities')
    require(provenance['client_operation_timeout'] == provenance['weir_backend_timeout'] == params['operation_timeout'] == '10s', 'explicit identical native/client/Store request budgets')
    require(params['paired_rounds'] == 3 and params['batch_sizes'] == [1] and params['client_process_count'] == 4 and params['records_per_business_request'] == 1 and params['dataset']['records'] == 2048 and params['dataset']['padding_bytes'] == 1024, 'full one-record recipe')
    require(params['warmup_duration'] == '10s' and params['measurement_duration'] == '20s'
            and params['database_cpu_budget_threshold_percent'] == 90, 'unchanged duration and CPU qualification threshold')
    levels = params['concurrency_levels']
    require(levels in ([8, 32, 128], [8, 32, 128, 512]), 'declared measured ladder')
    require(levels == expected['levels'], 'exact predeclared comparison ladder')
    return params


def checked_workflow(root, path, expected):
    run_path = nearest_run(path, root)
    run = load(run_path) if run_path else None
    stress = expected.get('dataset') == 'stress' and expected.get('allow_failed_stress_workflow') is True
    require(run is not None or not stress, 'stress actual workflow metadata present')
    if run is not None:
        require(run['headSha'] == expected['head'] and run['status'] == 'completed', 'actual completed run source')
        require(run['conclusion'] == 'success' or (stress and run['conclusion'] == 'failure'), 'successful workflow or explicit failed stress workflow')
    return run_path, run


def check_report(root, path, expected):
    report = load(path)
    require(not report.get('incomplete'), 'complete single-request report')
    params = check_report_header(report, expected)
    provenance, levels = report['provenance'], params['concurrency_levels']
    expected_pairs = {(c, round_) for c in levels for round_ in (1, 2, 3)}
    actual_pairs = [(p['concurrency'], p['round']) for p in report['pairs']]
    require(len(actual_pairs) == len(expected_pairs) and set(actual_pairs) == expected_pairs, 'exact unique concurrency/round pair set')
    backend, limit = params['dataset']['backend'], int(provenance['weir_max_batch_operations'])
    receipt, manifest = recipe(path, report, expected), fixture(root, report)
    run_path, run = checked_workflow(root, path, expected)
    aggregates, stages = {}, []
    for pair in report['pairs']:
        workers, round_ = pair['concurrency'], pair['round']
        order = ['direct', 'weir'] if (levels.index(workers) + round_ - 1) % 2 == 0 else ['weir', 'direct']
        require(pair['batch_size'] == 1 and pair['order'] == order, 'actual alternating single-record stages')
        for name in ('direct', 'weir'):
            stage = pair[name]
            require(stage['path'] == name, 'actual path identity')
            options = {'concurrency': workers, 'backend': backend, 'limit': limit, 'records': 2048, 'threshold': params['database_cpu_budget_threshold_percent']}
            evidence = check_stage(stage, options)
            key = (workers, name)
            entry = aggregates.setdefault(key, {'operations': 0, 'ns': 0, 'cpu': [], 'histogram': [0] * 256, 'max': 0, 'adapter_records': 0, 'adapter_invocations': 0})
            entry['operations'] += evidence['requests']
            entry['ns'] += evidence['elapsed_ns']
            entry['cpu'].append(evidence['cpu'])
            entry['histogram'] = [a + b for a, b in zip(entry['histogram'], evidence['buckets'])]
            entry['max'] = max(entry['max'], evidence['max'])
            if evidence['adapter']:
                entry['adapter_records'] += evidence['adapter']['records']
                entry['adapter_invocations'] += evidence['adapter']['invocations']
            stage_record = {'concurrency': workers, 'round': round_, 'path': name, 'successful_operations': evidence['requests'], 'cpu': evidence['cpu'], 'adapter': evidence['adapter'], 'global_command_excess': evidence['global_command_excess']}
            stages.append(stage_record)
    points = []
    for index, workers in enumerate(levels):
        for name in ('direct', 'weir'):
            entry = aggregates[(workers, name)]
            qps = entry['operations'] * 1e9 / entry['ns']
            plateau = False
            if index + 1 < len(levels):
                next_entry = aggregates[(levels[index + 1], name)]
                plateau = next_entry['operations'] * 1e9 / next_entry['ns'] <= qps * 1.1
            full = all(value['qualified'] for value in entry['cpu'])
            point = {'path': name, 'concurrency': workers, 'batch_size': 1, 'successful_operations_per_second': qps,
                     'mean_database_cpu_budget_percent': sum(value['mean_percent'] for value in entry['cpu']) / 3,
                     'all_rounds_successful_verified': True, 'all_rounds_cpu_saturated': full,
                     'throughput_plateau_at_next_concurrency': plateau, 'database_saturation_demonstrated': full and plateau}
            points.append(point)
    actual_points = report['points']
    require(len(actual_points) == len(points) and len({(p['concurrency'], p['path']) for p in actual_points}) == len(points), 'exact point set without duplicates')
    for actual, expected in zip(actual_points, points):
        check_selected(actual, expected, 'derived point')
    business = report['observed_business_comparisons']
    require([value['concurrency'] for value in business] == levels, 'one business comparison per level')
    for value in business:
        workers = value['concurrency']
        for name in ('direct', 'weir'):
            entry = aggregates[(workers, name)]
            near(value[name + '_successful_operations_per_second'], entry['operations'] * 1e9 / entry['ns'], 'business pooled QPS')
            check_latency(value[name + '_business_request_latency'], entry['histogram'], entry['max'])
            require(value[name + '_all_rounds_database_cpu_saturated'] == all(c['qualified'] for c in entry['cpu']), 'business CPU qualification')
        ratio = value['weir_successful_operations_per_second'] / value['direct_successful_operations_per_second']
        near(value['observed_weir_direct_throughput_ratio'], ratio, 'business ratio')
        near(value['observed_weir_throughput_delta_percent'], 100 * (ratio - 1), 'business change')
    peaks = report['observed_business_peak_comparison']
    chosen, saturated = {}, {}
    for name in ('direct', 'weir'):
        eligible = [p for p in points if p['path'] == name]
        chosen[name] = max(eligible, key=lambda p: p['successful_operations_per_second'])
        eligible = [p for p in eligible if p['database_saturation_demonstrated']]
        saturated[name] = max(eligible, key=lambda p: p['successful_operations_per_second']) if eligible else None
        check_selected(peaks[name + '_observed_peak'], chosen[name], 'verified observed peak')
    ratio = chosen['weir']['successful_operations_per_second'] / chosen['direct']['successful_operations_per_second']
    near(peaks['observed_weir_direct_peak_throughput_ratio'], ratio, 'observed peak ratio')
    near(peaks['observed_weir_peak_throughput_delta_percent'], 100 * (ratio - 1), 'observed peak change')
    capacity = report['database_saturated_comparisons']
    require(len(capacity) == 1 and capacity[0]['batch_size'] == 1, 'one capacity comparison')
    comparison = capacity[0]
    for name in ('direct', 'weir'):
        check_selected(comparison.get(name + '_database_saturated_point'), saturated[name], 'highest qualified saturated point')
    if all(saturated.values()):
        ratio = saturated['weir']['successful_operations_per_second'] / saturated['direct']['successful_operations_per_second']
        near(comparison['weir_direct_saturated_throughput_ratio'], ratio, 'CPU-saturated plateau ratio')
        near(comparison['weir_saturated_throughput_delta_percent'], 100 * (ratio - 1), 'CPU-saturated plateau change')
        require(not comparison.get('unavailable'), 'qualified capacity comparison')
    else:
        require(comparison.get('unavailable') and 'weir_direct_saturated_throughput_ratio' not in comparison and 'weir_saturated_throughput_delta_percent' not in comparison, 'capacity unavailable unless both paths qualified')
    result = {'file': str(path.relative_to(root)), 'backend': backend, 'write_percent': params['requested_write_percent'], 'batch_limit': limit,
              'concurrency_levels': levels, 'timed_stages': len(stages), 'successful_operations': sum(s['successful_operations'] for s in stages),
              'derived_points': points, 'observed_business_peaks': peaks, 'database_saturated_comparison': comparison, 'stages': stages,
              'scope': 'Observed business peaks are measured maxima within the ladder, not database capacity. Global Mongo commands include unassigned background traffic; ES physical HTTP command totals are unavailable. Optional host-device storage counters do not prove I/O saturation.'}
    if run is not None:
        result['workflow'] = {'run_id': run['databaseId'], 'conclusion': run['conclusion'], 'head': run['headSha']}
    private = {'report': report, 'receipt': receipt, 'manifest': manifest, 'run': run, 'run_path': str(run_path) if run_path else None, 'aggregates': aggregates}
    return result, private


def audit_root(root, kind, expected):
    require(kind in ('primary', 'control', 'pr-matrix') and not expected.get('allow_failed_stress_workflow'), 'ordinary audit cannot opt in to failed stress workflows')
    all_files = sorted(p for p in root.rglob('*.json') if p.name in ('mongo.json', 'search.json'))
    excluded = [p for p in all_files if any(part.endswith('-failed') for part in p.relative_to(root).parts)]
    files = [p for p in all_files if p not in excluded]
    require(files, 'raw reports present')
    results, private = [], {}
    for path in files:
        result, details = check_report(root, path, expected)
        key = (result['backend'], result['write_percent'], result['batch_limit'])
        require(key not in private, 'one report per backend/load/grouping mode')
        private[key] = details
        results.append(result)
    limits = (1, 32) if kind == 'control' else (32,)
    expected_reports = {(backend, write, limit) for backend in ('mongo', 'search') for write in (0, 10, 100) for limit in limits}
    require(set(private) == expected_reports, 'complete backend/load/grouping report set')
    summary = {'source': expected['head'], 'weir_source': expected['server'], 'weir_version': expected['version'], 'weir_sum': expected['sum'],
               'predeclared_concurrency_levels': expected['levels'], 'dataset': kind, 'reports': results,
               'timed_stages': sum(r['timed_stages'] for r in results), 'successful_operations': sum(r['successful_operations'] for r in results),
               'explicitly_excluded_failed_archives': [str(p.relative_to(root)) for p in excluded],
               'scope': 'Only raw evidence is recomputed. Unqualified observed QPS is not relabeled database capacity.'}
    return summary, private


def failed_warmup(stage, workers):
    children = stage['client_processes']
    require(stage['client_requests'] == stage['attempted'] == stage['succeeded'] == 0 and not stage['verified'], 'failed warmup is not a successful timed stage')
    require(len(children) == 4 and len({child['pid'] for child in children}) == 4, 'failed warmup retains all four actual child identities')
    totals = dict(attempted=0, succeeded=0, errors=0, indeterminate=0, business_operation_timeouts=0)
    child_rows, maximum = [], 0
    for index, child in enumerate(children):
        require(child['pid'] > 0 and child['phase'] == 'warmup' and 0 <= child['start_lag_ns'] <= 100_000_000, 'actual warmup child and common barrier')
        require(child['worker_offset'] == index * (workers // 4) and child['concurrent_workers'] == workers // 4, 'failed warmup worker partition')
        require(child['native_connection_pool_limit'] == (workers // 4 if stage['path'] == 'direct' else 0), 'failed warmup actual pool bound')
        result = child['result']
        require(result['path'] == stage['path'] and result['planned'] == result['attempted'] == child['business_requests'] == child['reads'] + child['writes'], 'failed warmup request accounting')
        require(result['attempted'] == result['succeeded'] + result['errors'] + result['indeterminate'], 'warmup success/error/UNKNOWN remain distinct')
        require(result['not_attempted'] == 0 and result['applied_with_error'] == 0 and result['business_operation_timeouts'] <= result['errors'] + result['indeterminate'], 'warmup failures retain uncertainty')
        require(sum(child['latency_histogram_buckets']) == result['attempted'], 'warmup latency samples include every offered request')
        check_latency(result['latency'], child['latency_histogram_buckets'], result['latency']['max_observed_ns'])
        near(result['successful_operations_per_second'], result['succeeded'] * 1e9 / result['elapsed_ns'], 'warmup child QPS')
        for key in totals:
            totals[key] += result[key]
        maximum = max(maximum, result['latency']['max_observed_ns'])
        child_rows.append({'pid': child['pid'], 'counts': {key: result[key] for key in totals},
                           'owned_process_exited': child['owned_process_exited'], 'owned_process_exit_error': child.get('owned_process_exit_error', ''),
                           'error_examples': result.get('error_examples', [])})
    require(totals['errors'] + totals['indeterminate'] > 0, 'incomplete report has actual warmup failures')
    summary = {'phase': 'warmup', 'path': stage['path'], 'concurrency': workers, 'counts': totals,
               'max_observed_latency_ns': maximum, 'children': child_rows, 'capacity_evidence': False,
               'scope': 'Warmup outcomes are not timed postflight-verified throughput. UNKNOWN mutation effects remain unconfirmed. Nonzero exit errors and unsuccessful child exit flags remain raw observations.'}
    return summary


def check_partial_stress(root, path, expected):
    report = load(path)
    require(expected.get('dataset') == 'stress' and expected.get('allow_failed_stress_workflow') is True and report.get('incomplete'), 'explicit incomplete stress report')
    params = check_report_header(report, expected)
    backend, levels = params['dataset']['backend'], params['concurrency_levels']
    limit = int(report['provenance']['weir_max_batch_operations'])
    recipe(path, report, expected)
    fixture(root, report)
    _, run = checked_workflow(root, path, expected)
    require(run['conclusion'] == 'failure', 'incomplete stress retains failed whole workflow')
    expected_pairs = [(workers, round_) for workers in levels for round_ in (1, 2, 3)]
    pairs = report['pairs']
    require(0 < len(pairs) <= len(expected_pairs), 'incomplete measured pair prefix')
    require([(p['concurrency'], p['round']) for p in pairs] == expected_pairs[:len(pairs)], 'incomplete report is an exact ordered prefix without duplicate pairs')
    stages, failures, observed = [], [], set()
    for pair in pairs:
        workers, round_ = pair['concurrency'], pair['round']
        order = ['direct', 'weir'] if (levels.index(workers) + round_ - 1) % 2 == 0 else ['weir', 'direct']
        require(pair['batch_size'] == 1 and pair['order'] == order, 'partial actual alternating path order')
        for name in order:
            stage = pair[name]
            require(stage['path'] == name, 'partial actual path identity')
            if stage['client_requests'] > 0:
                require(not failures, 'successful stages cannot follow terminal failed warmup')
                options = {'concurrency': workers, 'backend': backend, 'limit': limit, 'records': 2048, 'threshold': params['database_cpu_budget_threshold_percent']}
                evidence = check_stage(stage, options)
                observed.add((workers, round_, name))
                stages.append({'concurrency': workers, 'round': round_, 'path': name, 'successful_operations': evidence['requests'],
                               'qps_recomputed': evidence['requests'] * 1e9 / evidence['elapsed_ns'], 'latency': stage['latency'],
                               'cpu': evidence['cpu'], 'adapter': evidence['adapter'], 'global_command_excess': evidence['global_command_excess']})
            elif stage.get('client_processes'):
                require(not failures and pair is pairs[-1], 'one terminal failed warmup')
                failure = failed_warmup(stage, workers)
                failure['round'] = round_
                failures.append(failure)
            else:
                require(failures and stage['attempted'] == stage['succeeded'] == 0 and not stage['verified'], 'unstarted stage is unavailable')
    require(len(failures) == 1, 'explicit failed warmup evidence present')
    missing = [{'concurrency': workers, 'round': round_, 'path': name} for workers, round_ in expected_pairs for name in ('direct', 'weir') if (workers, round_, name) not in observed]
    result = {'file': str(path.relative_to(root)), 'backend': backend, 'write_percent': params['requested_write_percent'], 'status': 'INCOMPLETE',
              'workflow': {'run_id': run['databaseId'], 'conclusion': run['conclusion'], 'head': run['headSha']},
              'incomplete_reason': report['incomplete'], 'successful_timed_stages': stages, 'failed_warmups': failures,
              'missing_expected_timed_stages': missing, 'database_saturated_comparison': 'unavailable: incomplete stress report',
              'scope': 'Only individually successful timed stages are checked; missing paths and failed warmup cannot become a paired capacity comparison or a complete observed peak.'}
    return result


def audit_stress(root, expected):
    stress_expected = dict(expected)
    stress_expected.update(dataset='stress', allow_failed_stress_workflow=True, levels=[8, 32, 128, 512])
    files = sorted(path for path in root.rglob('*.json') if path.name in ('mongo.json', 'search.json'))
    require(len(files) == 6, 'all three stress workloads and both backend reports present')
    complete, partial, keys, workflows = [], [], set(), {}
    for path in files:
        report = load(path)
        key = (report['parameters']['dataset']['backend'], report['parameters']['requested_write_percent'])
        require(key not in keys, 'one stress report per backend/workload')
        keys.add(key)
        if report.get('incomplete'):
            result = check_partial_stress(root, path, stress_expected)
            partial.append(result)
        else:
            result, _ = check_report(root, path, stress_expected)
            complete.append(result)
        workflow = result['workflow']
        require(workflow['run_id'] not in workflows or workflows[workflow['run_id']] == workflow, 'one actual conclusion per stress workflow')
        workflows[workflow['run_id']] = workflow
    require(keys == {(backend, write) for backend in ('mongo', 'search') for write in (0, 10, 100)} and len(workflows) == 3, 'complete stress workload identities, not complete success')
    summary = {'source': expected['head'], 'weir_source': expected['server'], 'weir_version': expected['version'], 'weir_sum': expected['sum'],
               'status': 'INCOMPLETE_STRESS' if partial else 'COMPLETE_STRESS', 'predeclared_concurrency_levels': stress_expected['levels'],
               'actual_workflows': list(workflows.values()), 'complete_backend_reports': complete, 'incomplete_backend_reports': partial,
               'complete_report_timed_stages': sum(r['timed_stages'] for r in complete),
               'complete_report_successful_operations': sum(r['successful_operations'] for r in complete),
               'total_individually_successful_timed_stages': sum(r['timed_stages'] for r in complete) + sum(len(r['successful_timed_stages']) for r in partial),
               'total_individually_successful_timed_operations': sum(r['successful_operations'] for r in complete) + sum(s['successful_operations'] for r in partial for s in r['successful_timed_stages']),
               'scope': 'A complete backend can be checked within a failed workflow through this explicit stress-only entry point. The whole workflow failure and incomplete backend remain visible; no complete six-report PASS or incomplete paired capacity is asserted.'}
    return summary


def normalized_command(command):
    normalized = list(command)
    for option in ('-backend-batch-limit', '-output'):
        index = normalized.index(option)
        normalized[index + 1] = '<controlled-value>'
    return normalized


def normalized_parameters(parameters):
    normalized = json.loads(json.dumps(parameters))
    for key in ('namespace', 'weir_seed'):
        normalized['dataset'].pop(key, None)
    return normalized


def compare_controls(private, expected):
    comparisons = []
    for backend in ('mongo', 'search'):
        for write in (0, 10, 100):
            one, grouped = private[(backend, write, 1)], private[(backend, write, 32)]
            left, right = one['receipt'], grouped['receipt']
            require(normalized_command(left['command']) == normalized_command(right['command']), 'control changes only grouping limit and owned output')
            require(left['client_binary_sha256'] == right['client_binary_sha256'] and left['client_build'] == right['client_build'] and left['host_memory_bytes'] == right['host_memory_bytes'] and left['versions'] == right['versions'], 'same client build/memory/pins')
            require(left['client_operation_timeout'] == right['client_operation_timeout'] == left['weir_backend_timeout'] == right['weir_backend_timeout'] == '10s', 'control timeout policy identical')
            require(normalized_parameters(one['report']['parameters']) == normalized_parameters(grouped['report']['parameters']), 'same data/ladder/workload policy')
            lp, rp = one['report']['provenance'], grouped['report']['provenance']
            for key in ('weir_binary_sha256', 'weir_source', 'sdk', 'protocol', 'host_cpus', 'database_docker_cpu_quota', 'mongodb_image', 'elasticsearch_image', 'weir_memory_budget', 'weir_mongo_working_memory_mib', 'weir_search_working_memory_mib', 'weir_store_concurrency', 'weir_ingress_max_connections', 'weir_ingress_max_sessions', 'weir_backend_timeout', 'client_operation_timeout'):
                require(lp[key] == rp[key], 'same control provenance ' + key)
            require(one['manifest']['owner'] != grouped['manifest']['owner'], 'fresh independent owned fixtures')
            same_run = one['run_path'] is not None and one['run_path'] == grouped['run_path'] and one['run']['databaseId'] == grouped['run']['databaseId']
            one_job = same_run and len(one['run']['jobs']) == 1 and one['run']['jobs'][0]['conclusion'] == 'success'
            levels = one['report']['parameters']['concurrency_levels']
            rows = []
            for workers in levels:
                row = {'concurrency': workers}
                for name in ('direct', 'weir'):
                    a, b = one['aggregates'][(workers, name)], grouped['aggregates'][(workers, name)]
                    aq, bq = a['operations'] * 1e9 / a['ns'], b['operations'] * 1e9 / b['ns']
                    values = {'limit_1_qps': aq, 'limit_32_qps': bq, 'limit_32_over_1_ratio': bq / aq, 'change_percent': 100 * (bq / aq - 1),
                              'limit_1_p99_upper_bound_ns': quantile(a['histogram'], 99, a['max']), 'limit_32_p99_upper_bound_ns': quantile(b['histogram'], 99, b['max']),
                              'limit_1_all_rounds_cpu_qualified': all(c['qualified'] for c in a['cpu']), 'limit_32_all_rounds_cpu_qualified': all(c['qualified'] for c in b['cpu'])}
                    if name == 'weir':
                        values['limit_1_actual_batch_average'] = a['adapter_records'] / a['adapter_invocations']
                        values['limit_32_actual_batch_average'] = b['adapter_records'] / b['adapter_invocations']
                    row[name] = values
                rows.append(row)
            result = {'backend': backend, 'write_percent': write, 'same_single_job_runner_demonstrated': one_job,
                      'run_id': one['run']['databaseId'] if one_job else None,
                      'unavailable': None if one_job else 'Same single-job runner evidence is absent; control changes remain observations without demonstrated shared hardware.',
                      'comparisons': rows,
                      'scope': 'Weir limit 1 versus 32 within one job measures aggregation effect at matched concurrency; native baselines are rerun and their variation is disclosed. The sequential fresh-fixture modes are not randomized or simultaneous. QPS does not imply maximum DB capacity without each path qualification and plateau.'}
            comparisons.append(result)
    output = {'source': expected['head'], 'weir_source': expected['server'], 'weir_version': expected['version'], 'weir_sum': expected['sum'], 'controls': comparisons}
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--primary', type=Path, required=True)
    parser.add_argument('--control', type=Path, required=True)
    parser.add_argument('--matrix', type=Path)
    parser.add_argument('--stress', type=Path, help='separate four-level stress evidence; preserves failed/incomplete outcomes')
    parser.add_argument('--stress-output', type=Path)
    parser.add_argument('--head', required=True)
    parser.add_argument('--server', required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--sum', required=True)
    parser.add_argument('--primary-levels', required=True)
    parser.add_argument('--control-levels', required=True)
    parser.add_argument('--matrix-levels', default='8,32,128')
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--control-output', type=Path, required=True)
    args = parser.parse_args()
    require(re.fullmatch(r'[0-9a-f]{40}', args.head) and re.fullmatch(r'[0-9a-f]{40}', args.server), 'explicit immutable full SHA expectations')
    require(args.version.endswith('-' + args.server[:12]), 'explicit immutable module version refers to expected source')
    require(args.sum.startswith('h1:') and len(base64.b64decode(args.sum[3:], validate=True)) == 32, 'explicit module checksum expectation')
    expected = {'head': args.head, 'server': args.server, 'version': args.version, 'sum': args.sum,
                'levels': [int(value) for value in args.primary_levels.split(',')]}
    primary, _ = audit_root(args.primary.resolve(), 'primary', expected)
    control_expected = dict(expected)
    control_expected['levels'] = [int(value) for value in args.control_levels.split(',')]
    control, private = audit_root(args.control.resolve(), 'control', control_expected)
    output = {'primary': primary, 'control_raw': control}
    if args.matrix:
        matrix_expected = dict(expected)
        matrix_expected['levels'] = [int(value) for value in args.matrix_levels.split(',')]
        matrix, _ = audit_root(args.matrix.resolve(), 'pr-matrix', matrix_expected)
        output['pr_matrix'] = matrix
    control_output = compare_controls(private, control_expected)
    stress = None
    if args.stress:
        require(args.stress_output is not None, 'explicit separate stress audit output')
        stress = audit_stress(args.stress.resolve(), expected)
    args.output.write_text(json.dumps(output, indent=2) + '\n')
    args.control_output.write_text(json.dumps(control_output, indent=2) + '\n')
    if stress is not None:
        args.stress_output.write_text(json.dumps(stress, indent=2) + '\n')
    print('PASS independent report/peak/plateau/raw metrics and control audit:', primary['timed_stages'], '+', control['timed_stages'], 'timed stages')


if __name__ == '__main__':
    main()
