#!/usr/bin/env python3
"""Recompute archived single-request measurements from public raw evidence."""
import argparse
import json
import math
from pathlib import Path


def require(ok, description):
    if not ok:
        raise ValueError(description)


def near(actual, expected, description):
    require(math.isclose(actual, expected, rel_tol=1e-9, abs_tol=1e-8),
            f'{description}: {actual} != {expected}')


def quantile(buckets, percent, maximum):
    rank = (sum(buckets) * percent + 99) // 100
    cumulative = 0
    for index, count in enumerate(buckets):
        cumulative += count
        if cumulative < rank:
            continue
        if index == 0:
            return 0
        if index == 255:
            return maximum
        exponent, fraction = divmod(index - 1, 8)
        base = 1 << exponent
        return max((base + ((fraction + 1) * base + 7) // 8 - 1) * 1000, 1000)
    return maximum


def cpu_summary(resources, threshold):
    samples = resources['samples']
    measurement = resources['measurement_elapsed_ns']
    budget = resources['database_allocated_cpus']
    intervals = valid = full = 0
    weighted = 0.0
    for left, right in zip(samples, samples[1:]):
        span = min(right['elapsed_ns'], measurement) - max(left['elapsed_ns'], 0)
        clock = right['elapsed_ns'] - left['elapsed_ns']
        if left.get('database_cpu_counter_clock_unix_ns', 0) and right.get('database_cpu_counter_clock_unix_ns', 0):
            clock = right['database_cpu_counter_clock_unix_ns'] - left['database_cpu_counter_clock_unix_ns']
        if span <= 0 or clock <= 0 or left.get('error') or right.get('error'):
            continue
        if 'database_cpu_time_ns' not in left or 'database_cpu_time_ns' not in right:
            continue
        delta = right['database_cpu_time_ns'] - left['database_cpu_time_ns']
        if delta < 0:
            continue
        cpu = delta / clock / budget * 100
        if 'database_cpu_budget_percent' in right:
            near(right['database_cpu_budget_percent'], cpu, 'interval CPU')
        intervals += 1
        valid += span
        weighted += cpu * span
        if cpu >= threshold:
            full += span
    mean = weighted / valid if valid else 0
    coverage = valid / measurement
    full_fraction = full / measurement
    near(resources['mean_database_cpu_budget_percent'], mean, 'mean CPU')
    near(resources['cpu_sampling_time_coverage'], coverage, 'CPU coverage')
    near(resources['fraction_intervals_at_cpu_threshold'], full_fraction, 'CPU full fraction')
    require(resources['valid_cpu_intervals'] == intervals, 'CPU interval count')
    require(resources['valid_cpu_sampling_elapsed_ns'] == valid, 'CPU valid time')
    qualified = intervals >= 5 and coverage >= .8 and mean >= threshold and full_fraction >= .8
    require(resources['sustained_cpu_saturation'] == qualified, 'CPU qualification')
    return qualified


def stage_summary(stage, options):
    concurrency, records = options["concurrency"], options["records"]
    backend, batch_limit, threshold = options["backend"], options["batch_limit"], options["threshold"]
    requests = stage['client_requests']
    require(requests > 0 and stage['planned'] == stage['attempted'] == stage['succeeded'] == requests,
            'one record per successful business request')
    for key in ['errors', 'indeterminate', 'not_attempted', 'applied_with_error', 'business_operation_timeouts']:
        require(stage.get(key, 0) == 0, key)
    require(stage['verified'] and not stage.get('verification_error'), 'independent persisted postflight')
    near(stage['successful_operations_per_second'], requests * 1e9 / stage['elapsed_ns'], 'stage QPS')
    require(stage['reads'] + stage['writes'] == requests, 'read/write counts')
    children = stage['client_processes']
    require(len(children) == 4 and len({c['pid'] for c in children}) == 4, 'four distinct child PIDs')
    require(sum(c['concurrent_workers'] for c in children) == concurrency, 'child concurrency')
    require(sum(c['business_requests'] for c in children) == requests, 'child request sum')
    require(sum(c['reads'] for c in children) == stage['reads'], 'child reads')
    require(sum(c['writes'] for c in children) == stage['writes'], 'child writes')
    combined = [0] * 256
    maximum = 0
    covered = set()
    for index, child in enumerate(children):
        require(child['owned_process_exited'] and not child.get('owned_process_exit_error'), 'child clean exit')
        require(child['phase'] == 'timed' and child['start_lag_ns'] <= 100_000_000, 'common timed barrier')
        require(child['worker_offset'] == index * (concurrency // 4), 'disjoint worker offset')
        require(child['concurrent_workers'] == concurrency // 4, 'child thread count')
        require(child['native_connection_pool_limit'] == (concurrency // 4 if stage['path'] == 'direct' else 0), 'native pool bound')
        result = child['result']
        require(result['attempted'] == result['succeeded'] == child['business_requests'], 'child outcomes')
        buckets = child['latency_histogram_buckets']
        require(len(buckets) == 256 and sum(buckets) == child['business_requests'], 'child latency samples')
        for i, count in enumerate(buckets):
            combined[i] += count
        maximum = max(maximum, result['latency']['max_observed_ns'])
        for record_range in child['expected_record_ranges']:
            start, revisions = record_range['start'], record_range['revisions']
            require(start >= 0 and start + len(revisions) <= records, 'record bounds')
            require(all(v in (0, 1) for v in revisions), 'real alternating revisions')
            ids = set(range(start, start + len(revisions)))
            require(not (covered & ids), 'no conflicting client record ownership')
            covered.update(ids)
    require(covered == set(range(records)), 'all final record IDs independently verified')
    latency = stage['latency']
    require(latency['samples'] == sum(combined) == requests and latency['max_observed_ns'] == maximum, 'stage pooled histogram')
    for percent in (50, 95, 99):
        require(latency[f'p{percent}_upper_bound_ns'] == quantile(combined, percent, maximum), 'stage latency quantile')
    require(stage['resources']['measurement_elapsed_ns'] == stage['elapsed_ns'], 'shared denominator')
    child_pids = {c['pid'] for c in children}
    for sample in stage['resources']['samples']:
        require({p['pid'] for p in sample['client_processes']} == child_pids, 'per-PID resource samples')
    full = cpu_summary(stage['resources'], threshold)
    if backend == 'mongo':
        require(not stage.get('physical_database_command_observation_unavailable'), 'Mongo physical commands available')
        commands = stage['timed_physical_database_command_deltas']
        before = stage['timed_database_counters_before']['physical_command_totals']
        after = stage['timed_database_counters_after']['physical_command_totals']
        for name in ('find', 'update', 'bulkWrite', 'getMore', 'killCursors'):
            require(commands[name] == after[name] - before[name] >= 0, 'physical command delta')
        if stage['path'] == 'direct':
            require(commands['find'] >= stage['reads'] and commands['update'] >= stage['writes'], 'global command totals account for native one-record business calls; background commands can add counts')
            require(commands['bulkWrite'] == 0, 'no native timed bulk writes')
    else:
        require(bool(stage.get('physical_database_command_observation_unavailable')), 'ES physical HTTP counts explicitly unavailable')
    average = None
    if stage['path'] == 'weir':
        metrics = stage['server_metrics']
        require(not metrics.get('unavailable'), 'server metrics available')
        delta = metrics['timed_counter_deltas']
        require(delta['weir_store_records_total'] == requests, 'one native plan per RPC')
        for method, calls in [('read', stage['reads']), ('mutate', stage['writes'])]:
            require(delta.get('weir_rpc_completions_total:' + method, 0) == calls, 'actual business RPC counts')
        invocations = delta['weir_store_batch_operations_count']
        require(invocations > 0 and delta['weir_store_batch_operations_sum'] == requests, 'adapter batch sum')
        average = requests / invocations
        near(metrics['timed_adapter_batch_average'], average, 'adapter average')
        near(metrics['timed_business_rpcs_per_adapter_invocation'], average, 'RPCs per adapter')
        require(delta['weir_store_rejections_total'] == 0, 'no admission errors')
        require(metrics['configured_backend_concurrency_limit'] == 32, 'backend permits fixed')
        near(metrics['timed_adapter_batch_cumulative_buckets']['+Inf'], invocations, 'batch histogram count')
        require(1 <= average <= batch_limit, 'adapter batch limit')
        if batch_limit == 1:
            near(average, 1, 'disabled aggregation control')
        if backend == 'mongo':
            physical = commands['find'] + commands['bulkWrite']
            if not stage['reads'] or not stage['writes']:
                require(physical >= invocations, 'server-global query/write count accounts for pure adapter groups; background commands may add counts')
            else:
                require(invocations <= physical <= 2 * invocations,
                        'mixed Mongo adapter group may perform separate read and write commands')
    return full, combined, maximum, average


def fixture_summary(root, report, equal_timeouts):
    provenance, params = report['provenance'], report['parameters']
    fixture_name = Path(provenance['fixture_logs']).name
    matches = [p for p in root.rglob('manifest.json') if p.parent.name == fixture_name]
    require(len(matches) == 1, 'one explicitly owned fixture manifest')
    manifest_file = matches[0]
    manifest = json.loads(manifest_file.read_text())
    cleanup = json.loads((manifest_file.parent / 'cleanup.json').read_text())
    require(cleanup['owner'] == manifest['owner'] and not cleanup['errors'], 'owned cleanup identity')
    options = manifest['options']
    if equal_timeouts:
        require(options['BackendTimeout'] == 10_000_000_000, 'actual fixture backend timeout')
    sessions = max(64, max(params['concurrency_levels']))
    batch_limit = int(provenance['weir_max_batch_operations'])
    working = {'mongo': 1312, 'search': 3072}
    memory = ((sessions * 96 + 1024 + sum(working.values()) + 128 + 1023) // 1024) * 1024
    require(options['Backends'] == ['mongo', 'search'] and options['OwnerCount'] == 1 and not options['MongoBinary'], 'shared owned Docker backend topology')
    require(options['StoreConcurrency'] == 32 and options['BatchSize'] == batch_limit and options['IngressSessions'] == sessions, 'fixed permits and independent client/grouping limits')
    require(options['WorkingMemoryMiB'] == working and options['ProcessMemoryMiB'] == memory, 'same real byte budgets in grouping controls')
    require(manifest['binary_sha256'] == provenance['weir_binary_sha256'], 'measured binary hash')
    require(manifest['host_os'] == 'linux' and manifest['host_arch'] == 'amd64', 'primary Linux native server/client')
    config = json.loads((manifest_file.parent / 'node-0-config.json').read_text())
    require(config['memory'] == str(memory) + 'MiB', 'declared memory budget')
    require(config['transport'] == {'max_connections': 64, 'max_sessions': sessions}, 'fixed connection budget and actual RPC ingress')
    routes = json.loads((manifest_file.parent / 'node-0-routes.json').read_text())
    for store in routes['stores']:
        if equal_timeouts:
            require(store['backend_timeout'] == '10s', 'actual owner backend timeout')
        require(store['max_concurrency'] == 32 and store['max_batch_operations'] == batch_limit, 'actual backend limits')
        require(store['working_memory'] == str(working[store['name']]) + 'MiB' and store['max_read_size'] == '2MiB', 'working/read byte budgets')
    containers = {c['id']: c for c in manifest['containers']}
    require(len(containers) == 2 and {c['backend'] for c in containers.values()} == {'mongo', 'search'}, 'one database per backend')
    quota = report['database_cpu_quota']
    require(quota['container_id'] in containers and containers[quota['container_id']]['backend'] == params['dataset']['backend'], 'actual quota belongs to measured owned database')
    for container in containers.values():
        pin = 'mongodb_image' if container['backend'] == 'mongo' else 'elasticsearch_image'
        require(container['image'] == provenance[pin], 'immutable image digest')
        require(container['name'].startswith('weir-tests-' + manifest['owner'] + '-'), 'owned container names')
    require({c['id'] for c in cleanup['containers']} == set(containers), 'all owned containers accounted')
    require(all(c['removed'] for c in cleanup['containers']), 'owned containers removed')
    require(len(cleanup['processes']) == 1 and all(p['stopped'] for p in cleanup['processes']), 'owned Weir stopped')
    return manifest_file


def audit(root, head, server, equal_timeouts):
    summary = {'source': head, 'weir_source': server, 'equal_timeout_policy_verified': equal_timeouts, 'reports': [], 'successful_operations': 0, 'timed_stages': 0}
    receipts = list(root.rglob('measurement-batch-*.receipt.json'))
    require(receipts, 'measurement receipts present')
    for file in receipts:
        receipt = json.loads(file.read_text())
        require(receipt['source'] == head and receipt['source_dirty'] is False, 'clean immutable recipe')
        require(receipt['versions']['weir_source'] == server, 'server lock')
        require(receipt['versions']['sdk'] == 'v0.4.2' and receipt['versions']['protocol'] == 'v0.2.1', 'SDK/protocol version pins')
        require('vcs.revision=' + head in receipt['client_build'] and 'vcs.modified=false' in receipt['client_build'], 'built client identity')
        if equal_timeouts:
            require(receipt['client_operation_timeout'] == receipt['weir_backend_timeout'] == '10s', 'executed equal timeout policy')
    reports = [p for name in ('mongo.json', 'search.json') for p in root.rglob(name)]
    require(reports, 'raw reports present')
    for file in reports:
        report = json.loads(file.read_text())
        require(report['schema'] == 3 and not report.get('incomplete'), 'complete single-request report')
        params, provenance = report['parameters'], report['provenance']
        require(provenance['test_source'] == head and provenance['test_source_dirty'] == 'false', 'measured recipe identity')
        require(provenance['weir_source'] == server, 'measured server identity')
        if equal_timeouts:
            require(params['operation_timeout'] == '10s', 'client operation budget')
            require(provenance['client_operation_timeout'] == provenance['weir_backend_timeout'] == '10s', 'explicit equal backend and caller budgets')
        require(params['records_per_business_request'] == 1 and params['client_process_count'] == 4, 'primary business model')
        require(params['paired_rounds'] == 3 and params['warmup_duration'] == '10s' and params['measurement_duration'] == '20s', 'full duration recipe')
        require(params['concurrency_levels'] in ([8, 32, 128], [8, 32, 128, 512]), 'declared concurrency ladder')
        require(len(report['pairs']) == 3 * len(params['concurrency_levels']), 'complete matrix')
        quota = report['database_cpu_quota']
        require(quota['host_config_nano_cpus'] == 1_000_000_000 and quota['requested_cpus'] == quota['observed_cpus'] == 1, 'actual database 1 CPU quota')
        backend = params['dataset']['backend']
        batch_limit = int(provenance['weir_max_batch_operations'])
        fixture_summary(root, report, equal_timeouts)
        points = {}
        averages = []
        global_command_excesses = []
        for pair in report['pairs']:
            workers = pair['concurrency']
            order = ['direct', 'weir'] if (params['concurrency_levels'].index(workers) + pair['round'] - 1) % 2 == 0 else ['weir', 'direct']
            require(pair['order'] == order and pair['batch_size'] == 1, 'alternating paired single requests')
            for path in ('direct', 'weir'):
                stage = pair[path]
                stage_options = {'concurrency': workers, 'records': params['dataset']['records'], 'backend': backend, 'batch_limit': batch_limit, 'threshold': params['database_cpu_budget_threshold_percent']}
                full, buckets, maximum, average = stage_summary(stage, stage_options)
                summary['successful_operations'] += stage['succeeded']
                summary['timed_stages'] += 1
                key = (workers, path)
                point = points.setdefault(key, {'operations': 0, 'ns': 0, 'rounds': 0, 'full': True, 'histogram': [0] * 256, 'max': 0})
                point['operations'] += stage['succeeded']; point['ns'] += stage['elapsed_ns']; point['rounds'] += 1; point['full'] &= full
                point['histogram'] = [a + b for a, b in zip(point['histogram'], buckets)]; point['max'] = max(point['max'], maximum)
                if average is not None:
                    averages.append(average)
                if backend == 'mongo':
                    commands = stage['timed_physical_database_command_deltas']
                    excess = {}
                    if path == 'direct':
                        excess = {'find': commands['find'] - stage['reads'], 'update': commands['update'] - stage['writes']}
                    elif not stage['reads'] or not stage['writes']:
                        invocations = stage['server_metrics']['timed_counter_deltas']['weir_store_batch_operations_count']
                        excess = {'find_plus_bulkWrite': commands['find'] + commands['bulkWrite'] - invocations, 'update': commands['update']}
                    if any(excess.values()):
                        global_command_excesses.append({'concurrency': workers, 'round': pair['round'], 'path': path, 'excess_over_business_calls': excess, 'scope': 'global serverStatus counters include background database commands; namespace attribution is unavailable'})
        for point in report['points']:
            raw = points[(point['concurrency'], point['path'])]
            require(raw['rounds'] == 3, 'all three paired rounds')
            near(point['successful_operations_per_second'], raw['operations'] * 1e9 / raw['ns'], 'point pooled QPS')
            require(point['all_rounds_cpu_saturated'] == raw['full'] and point['all_rounds_successful_verified'], 'point qualification')
        for comparison in report['observed_business_comparisons']:
            workers = comparison['concurrency']
            direct, weir = points[(workers, 'direct')], points[(workers, 'weir')]
            direct_qps = direct['operations'] * 1e9 / direct['ns']; weir_qps = weir['operations'] * 1e9 / weir['ns']
            near(comparison['observed_weir_throughput_delta_percent'], 100 * (weir_qps / direct_qps - 1), 'business change')
            for path in ('direct', 'weir'):
                raw = points[(workers, path)]
                for percentile in (50, 95, 99):
                    require(comparison[path + '_business_request_latency'][f'p{percentile}_upper_bound_ns'] == quantile(raw['histogram'], percentile, raw['max']), 'comparison pooled latency')
        summary['reports'].append({'file': str(file.relative_to(root)), 'backend': backend, 'writes': params['requested_write_percent'], 'backend_batch_limit': batch_limit, 'business_peaks': report['observed_business_peak_comparison'], 'batch_average_min': min(averages), 'batch_average_max': max(averages), 'global_command_excesses': global_command_excesses, 'database_counter_error_samples': sum(bool(sample.get('database_counter_error')) for pair in report['pairs'] for path in ('direct', 'weir') for sample in pair[path]['resources']['samples'])})
    return summary


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=Path)
    parser.add_argument('--head', required=True)
    parser.add_argument('--server', required=True)
    parser.add_argument('--require-equal-timeouts', action='store_true', help='Require the final explicitly recorded 10s caller/backend policy; legacy observations do not verify this policy')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = audit(args.root, args.head, args.server, args.require_equal_timeouts)
    args.output.write_text(json.dumps(result, indent=2) + '\n')
    print('PASS', len(result['reports']), 'reports;', result['timed_stages'], 'stages;', result['successful_operations'], 'successful operations')
