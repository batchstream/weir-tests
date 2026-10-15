#!/usr/bin/env python3
"""Verify frozen crawler pressure calibration and formal measurements; retain every failed stage."""
import argparse
import csv
import gzip
import hashlib
import itertools
import json
import math
from pathlib import Path
import re
import subprocess
import tempfile

import summarize as summary

BASELINE_SOURCE = '0f6fac3edb462e088decff98b67560dae2c23c9a'
CLIENT_SOURCE = '488b4575932265030b81312319b707e45649a028'
CANDIDATE_SOURCE = '745501319ae59a16351294c2eaed58019288c3ad'
RATES = (96, 384, 768)
WORKERS = (512, 2048)
PATHS = ('direct', 'current', 'optimized')
COUNTS = ('Offered', 'Rejected', 'Completed', 'Failed', 'Indeterminate', 'TimedOut', 'Late', 'WithinWindow')
GATES = dict(rounds=3, duration_s=20, warmup_s=10, cpu_coverage=0.8, p99_ms=5000, drain_s=3, generator_lag_ms=50)


def require(condition, message):
    if not condition:
        raise ValueError(message)


def load(path):
    opener = gzip.open if path.suffix == '.gz' else open
    with opener(path, 'rt', encoding='utf-8') as stream:
        return json.load(stream)


def verify_archive(folder):
    inventory = load(folder / 'files.json')
    require(inventory and len({item['path'] for item in inventory}) == len(inventory), 'empty/duplicate raw archive inventory')
    require({item['path'] for item in inventory} == {path.name for path in folder.glob('*.gz')}, 'raw archive inventory differs from files')
    for item in inventory:
        require(Path(item['path']).name == item['path'] and item['path'].endswith('.gz'), 'unsafe archive filename')
        compressed = (folder / item['path']).read_bytes()
        raw = gzip.decompress(compressed)
        require(hashlib.sha256(compressed).hexdigest() == item['gzip_sha256'] and hashlib.sha256(raw).hexdigest() == item['sha256'] and len(raw) == item['bytes'], 'raw archive hash/length mismatch: ' + item['path'])
    log_receipt_path = folder / 'search-log-receipt.json.gz'
    if log_receipt_path.exists():
        log_receipt = load(log_receipt_path)
        fixture = load(folder / 'manifest.json.gz')
        log_bytes = gzip.decompress((folder / 'search-process.log.gz').read_bytes())
        require(log_receipt['owner'] == fixture['owner'] and log_receipt['container_id'] == fixture['containers'][0]['id'] and not log_receipt['error'], 'owned database log receipt mismatch')
        require(len(log_bytes) == log_receipt['captured_bytes'] <= log_receipt['maximum_bytes'], 'owned database log capture length mismatch')
    return len(inventory)


def csv_write(path, values):
    fields = list(dict.fromkeys(key for row in values for key in row))
    with path.open('w', newline='', encoding='utf-8') as stream:
        writer = csv.DictWriter(stream, fieldnames=fields, lineterminator='\n')
        writer.writeheader()
        writer.writerows(values)


def duration(raw):
    units = dict(ns=1, us=1000, ms=10**6, s=10**9, m=60*10**9, h=3600*10**9)
    parts = re.findall(r'([0-9]+(?:\.[0-9]+)?)(ns|us|ms|s|m|h)', raw)
    require(''.join(number + unit for number, unit in parts) == raw, 'invalid command duration')
    return round(sum(float(number) * units[unit] for number, unit in parts))


def histogram(buckets, maximum):
    require(len(buckets) == 256 and all(isinstance(n, int) and n >= 0 for n in buckets), 'invalid histogram buckets')
    count = sum(buckets)
    result = dict(samples=count, max_observed_ns=maximum)
    for percentile in (50, 95, 99):
        rank, cumulative, value = (count * percentile + 99) // 100, 0, 0
        if count:
            for index, size in enumerate(buckets):
                cumulative += size
                if cumulative < rank:
                    continue
                if index == 255:
                    value = maximum
                elif index:
                    exponent, fraction = divmod(index - 1, 8)
                    base = 1 << exponent
                    value = max((base + ((fraction + 1) * base + 7) // 8 - 1) * 1000, 1000)
                break
        result[f'p{percentile}_upper_bound_ns'] = value
    return result


def settling(receipt):
    require(receipt['settled'] and receipt['TimeoutNS'] == 30*10**9 and receipt['RequiredQuietNS'] >= 2*10**9 and receipt['PollNS'] == 10**9, 'settling policy mismatch')
    previous, names, quiet, last_time = None, None, None, -1
    for sample in receipt['samples']:
        elapsed, stats = sample['elapsed_ns'], sample['index_stats']
        require(not sample.get('error') and last_time < elapsed <= receipt['wait_ns'], 'invalid settling sample')
        require(stats['_shards']['failed'] == 0 and len(stats['indices']) == 2, 'settling indices incomplete')
        indices = tuple(sorted(stats['indices']))
        require(names in (None, indices) and indices[0].endswith('-offers') and indices[1].endswith('-products') and indices[0][:-7] == indices[1][:-9], 'settling index identity changed')
        names, counters, idle = indices, [], True
        for name in indices:
            primary = stats['indices'][name]['primaries']
            idle = idle and primary['indexing']['index_current'] == 0 and primary['merges']['current'] == 0
            counters.extend(primary['merges'][key] for key in ('total', 'total_time_in_millis', 'total_docs', 'total_size_in_bytes'))
            counters.extend(primary['flush'][key] for key in ('total', 'total_time_in_millis'))
        quiet = elapsed if idle and (quiet is None or counters != previous) else quiet
        if not idle:
            quiet = None
        previous, last_time = counters, elapsed
    require(quiet is not None and last_time - quiet >= receipt['RequiredQuietNS'], 'raw settling tail does not establish the required quiet interval')
    return names


def prom_value(snapshot, name):
    values = []
    pattern = re.compile(re.escape(name) + r'\{([^}]*)\}\s+(\S+)(?:\s+\d+)?')
    label_pattern = re.compile(r'([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\.|[^"\\])*)"')
    for line in (snapshot.get('raw') or '').splitlines():
        match = pattern.fullmatch(line.strip())
        if not match:
            continue
        labels = {key: json.loads('"' + value + '"') for key, value in label_pattern.findall(match[1])}
        if labels.get('store') != 'search':
            continue
        value = float(match[2])
        require(math.isfinite(value), 'nonfinite store metric: ' + name)
        values.append(value)
    require(len(values) <= 1, 'ambiguous store metric: ' + name)
    return values[0] if values else None


def adapter_metrics(stage):
    result = dict(observed_adapter_records=None, observed_adapter_executions=None, observed_adapter_records_per_execution=None)
    if stage['Path'] == 'direct' or stage['ElapsedNS'] <= 0:
        return result
    for name, field in (('weir_store_batch_operations_sum', 'observed_adapter_records'), ('weir_store_batch_operations_count', 'observed_adapter_executions')):
        before = prom_value(stage['ServerBefore'], name)
        after = prom_value(stage['ServerAfter'], name)
        if before is not None and after is not None:
            require(after >= before, 'store batch counter decreased')
            result[field] = after - before
    if result['observed_adapter_executions'] and result['observed_adapter_records'] is not None:
        result['observed_adapter_records_per_execution'] = result['observed_adapter_records'] / result['observed_adapter_executions']
    return result


def application_ledger(raw):
    require(isinstance(raw, dict), 'missing application-call ledger')
    fields = ('requests', 'records', 'minimum_records', 'maximum_records')
    require(all(type(raw[key]) is int and raw[key] >= 0 for key in fields), 'invalid application-call count')
    counts = raw['records_per_request'] or {}
    require(isinstance(counts, dict) and all(type(value) is int and value > 0 for value in counts.values()), 'invalid application-call histogram')
    expected = {'1': raw['requests']} if raw['requests'] else {}
    require(counts == expected and raw['records'] == raw['requests'], 'a client call did not contain exactly one record')
    cardinality = 1 if raw['requests'] else 0
    require(raw['minimum_records'] == raw['maximum_records'] == cardinality, 'application-call cardinality bounds differ')
    result = dict(requests=raw['requests'], records=raw['records'], minimum_records=cardinality, maximum_records=cardinality, records_per_request=counts)
    return result


def verify_client_calls(clients, stage, options, phase):
    require(len(clients) == options['Processes'], 'client-call phase process count differs')
    totals = dict(requests=0, records=0, completed=0, failed=0, unknown=0, timeout=0, rejected=0)
    pids = set()
    for process, reply in enumerate(clients):
        require(reply['Phase'] == phase and reply['InputShape'] == 'single' and reply['PID'] > 0 and reply['PID'] not in pids, 'client-call phase identity differs')
        pids.add(reply['PID'])
        threads = stage['Concurrency'] // options['Processes']
        require(len(reply['Workers']) == threads, 'client-call phase worker count differs')
        pool = 3 * threads if stage['Path'] == 'direct' else 0
        require(reply['NativePoolLimit'] == pool, 'native per-process pool differs')
        client_counts = application_ledger(reply['ApplicationRequests'])
        client_requests, client_records = 0, 0
        for index, worker in enumerate(reply['Workers']):
            require(worker['worker'] == process * threads + index and worker['pid'] == reply['PID'], 'client-call worker identity differs')
            counts = application_ledger(worker['application_requests'])
            completed_calls = sum(4 + int((worker['worker'] + page) % 100 == 0) for page in worker['applied_pages'] or [])
            planned = worker['requested_logical_mutations']
            require(completed_calls <= counts['records'] <= planned, 'actual application calls omit successful work or exceed planned work')
            if worker['Failed'] == 0:
                require(counts['records'] == planned == completed_calls, 'successful single-record causal phases have missing/extra calls')
            require(worker['Offered'] == worker['Rejected'] + worker['Completed'] + worker['Failed'] and worker['Completed'] == len(worker['applied_pages'] or []), 'client-call phase business accounting differs')
            client_requests += counts['requests']
            client_records += counts['records']
            for key, field in (('completed', 'Completed'), ('failed', 'Failed'), ('unknown', 'Indeterminate'), ('timeout', 'TimedOut'), ('rejected', 'Rejected')):
                totals[key] += worker[field]
        require(client_counts['requests'] == client_requests and client_counts['records'] == client_records, 'client application-call aggregation differs')
        totals['requests'] += client_requests
        totals['records'] += client_records
    return totals


def verify_application_calls(stage, options):
    require(stage['InputShape'] == options['InputShape'] == 'single', 'stage input shape differs')
    stage_counts = application_ledger(stage['ApplicationRequests'])
    warmup_clients = stage['WarmupClients']
    warmup = verify_client_calls(warmup_clients, stage, options, 'warmup') if warmup_clients else None
    evidence = dict(warmup=warmup, timed=None)
    if stage['ElapsedNS'] == 0:
        require(stage_counts['requests'] == 0, 'unmeasured stage unexpectedly has timed application-call counters')
        return evidence
    require(warmup is not None and warmup['failed'] == warmup['unknown'] == warmup['timeout'] == 0, 'timed stage lacks a successful warmup')
    timed = verify_client_calls(stage['Clients'], stage, options, 'timed')
    require(stage_counts['requests'] == timed['requests'] and stage_counts['records'] == timed['records'], 'stage application-call aggregation differs')
    require(all(before['PID'] == after['PID'] for before, after in zip(warmup_clients, stage['Clients'])), 'client process changed between warmup and timing')
    evidence['timed'] = timed
    return evidence


def arithmetic(stage, options):
    sums = dict.fromkeys(COUNTS, 0)
    pooled = [[0]*256, [0]*256]
    maxima, pids, seen_workers = [0, 0], set(), set()
    for process, reply in enumerate(stage['Clients']):
        require(reply['Phase'] == 'timed' and reply['PID'] > 0 and reply['PID'] not in pids, 'client identity mismatch')
        pids.add(reply['PID'])
        require(len(reply['Workers']) == stage['Concurrency'] // options['Processes'], 'worker count mismatch')
        for index, worker in enumerate(reply['Workers']):
            require(worker['pid'] == reply['PID'] and worker['worker'] == process * len(reply['Workers']) + index and worker['worker'] not in seen_workers, 'worker identity mismatch')
            seen_workers.add(worker['worker'])
            require(all(isinstance(worker[key], int) and worker[key] >= 0 for key in COUNTS), 'negative/nonintegral operation count')
            require(worker['Offered'] == worker['Rejected'] + worker['Completed'] + worker['Failed'], 'offered accounting mismatch')
            require(worker['Completed'] == worker['WithinWindow'] + worker['Late'] == len(worker['applied_pages'] or []) * options['PageSize'], 'completion accounting mismatch')
            pages = worker['applied_pages'] or []
            require(pages == sorted(set(pages)) and all(page >= 0 for page in pages), 'invalid acknowledged page ordering')
            require(0 <= worker['queue_peak_pages'] <= options['QueuePages'], 'queue bound exceeded')
            require(worker['Indeterminate'] <= worker['Failed'] and worker['TimedOut'] <= worker['Failed'], 'failure subset accounting mismatch')
            for kind, prefix in enumerate(('planned', 'dispatch')):
                latency = worker[f'{prefix}_to_visible_latency']
                buckets = worker[f'{prefix}_to_visible_buckets']
                calculated = histogram(buckets, latency['max_observed_ns'])
                require(all(latency[key] == value for key, value in calculated.items()), 'worker histogram summary mismatch')
                require(latency['samples'] * options['PageSize'] == worker['Completed'] + worker['Failed'], 'latency dropped a dispatched page')
                pooled[kind] = [a+b for a, b in zip(pooled[kind], buckets)]
                maxima[kind] = max(maxima[kind], latency['max_observed_ns'])
            for key in COUNTS:
                sums[key] += worker[key]
    require(len(pids) == options['Processes'] and all(stage[key] == value for key, value in sums.items()), 'pooled count mismatch')
    require(stage['RequestedMutations'] == sum(worker['requested_logical_mutations'] for reply in stage['Clients'] for worker in reply['Workers']), 'logical mutation count mismatch')
    require(options['HTTPObserver'] == 'disabled' and all(stage[key] is None for key in ('HTTPBefore', 'HTTPAfter', 'HTTPDelta')), 'single-record observer data must remain explicitly unavailable')
    for kind, key in enumerate(('ScheduledToVisible', 'DispatchToVisible')):
        calculated = histogram(pooled[kind], maxima[kind])
        require(all(stage[key][name] == value for name, value in calculated.items()), 'pooled histogram mismatch')
    require(stage['ElapsedNS'] >= options['Duration'] and stage['DrainNS'] == stage['ElapsedNS'] - options['Duration'], 'drain denominator mismatch')
    require(math.isclose(stage['CompletedProductsPerSecond'], stage['Completed'] / (stage['ElapsedNS']/1e9), rel_tol=1e-12), 'throughput denominator mismatch')


def identities(bundle, client_source, expected_sources):
    report, receipt, fixture, cleanup, entry = (bundle[key] for key in ('report', 'receipt', 'fixture_manifest', 'cleanup', 'entry'))
    options = report['Parameters']
    require(options['AfterMaxInFlight'] == 0, 'both Weir paths must use unlimited cap0')
    require(len(receipt['servers']) == 2, 'missing baseline/optimized server receipts')
    require(receipt['servers'][0]['source'] != receipt['servers'][1]['source'] and receipt['servers'][0]['sha256'] != receipt['servers'][1]['sha256'], 'source optimization requires distinct immutable sources and executable hashes')
    role = entry['role']
    require(role in ('calibration', 'formal'), 'unknown experiment role')
    rates = list(RATES) if role == 'calibration' else [entry['rate']]
    rounds, seconds, warmup = (1, 5, 2) if role == 'calibration' else (3, 20, 10)
    require(options['Rates'] == rates and entry['workers'] in WORKERS, 'frozen offered rate/worker level differs')
    require(options['Rounds'] == rounds and options['Duration'] == seconds * 10**9 and options['Warmup'] == warmup * 10**9, 'frozen rounds or durations differ')
    require(options['InputShape'] == 'single' and options['PageSize'] == 1 and options['Concurrency'] == [entry['workers']] and options['Profiles'] == ['two-step'] and options['Modes'] == ['fixed-rate'], 'single-record workload dimensions differ')
    require(receipt['source'] == client_source and receipt['source_dirty'] is False and re.fullmatch('[0-9a-f]{40}', client_source), 'client source mismatch/dirty')
    require(receipt['workflow_run'] == entry['workflow_run'] and receipt['host']['system'] == 'Linux', 'workflow/host mismatch')
    require(receipt['client_binary_sha256'] == receipt['client_binary_after_sha256'] and re.fullmatch('[0-9a-f]{64}', receipt['client_binary_sha256']), 'client executable changed')
    require(len(receipt['servers']) == len(receipt['server_binary_after_sha256']) == 2, 'missing server binary receipt')
    for index, name in enumerate(('current', 'optimized')):
        server = receipt['servers'][index]
        require(server['source'] == expected_sources[name] and server['identity']['revision'] == expected_sources[name] and server['source_dirty'] is False, 'server immutable source mismatch')
        require(server['identity']['dirty'] == 'false' and server['identity']['state'] == 'clean-commit' and server['identity']['target'].startswith('linux/'), 'dirty/non-Linux server identity')
        require(re.fullmatch('[0-9a-f]{64}', server['sha256']), 'invalid binary digest')
        require(server['sha256'] == receipt['server_binary_after_sha256'][index] == report['Servers'][name]['sha256'], 'server executable changed')
        require(server == report['Servers'][name]['receipt'], 'report/runner receipt differs')
    command = receipt['command']
    require(len(command) % 2 == 1 and len(set(command[1::2])) == len(command[1::2]), 'invalid/duplicate command flags')
    flags = dict(zip(command[1::2], command[2::2]))
    integers = {'-after-max-in-flight': 'AfterMaxInFlight', '-client-processes': 'Processes', '-page-size': 'PageSize', '-queue-pages': 'QueuePages', '-rounds': 'Rounds'}
    for flag, field in integers.items():
        require(int(flags[flag]) == options[field], 'command/report integer mismatch: ' + flag)
    for flag, field in (('-duration', 'Duration'), ('-warmup', 'Warmup'), ('-timeout', 'Timeout')):
        require(duration(flags[flag]) == options[field], 'command/report duration mismatch')
    for flag, field in (('-rates', 'Rates'), ('-concurrency-levels', 'Concurrency')):
        values = [] if flags[flag] in ('', 'none') else [int(value) for value in flags[flag].split(',')]
        require(values == (options[field] or []), 'command/report matrix mismatch')
    require(flags['-profiles'].split(',') == options['Profiles'] and flags['-output'] == options['Output'], 'command/report profile/output mismatch')
    require(flags.get('-input-shape') == receipt['input_shape'] == options['InputShape'] == 'single', 'runner/command/report input shape mismatch')
    require(receipt['http_observer'] == 'disabled', 'runner HTTP observer mode differs')
    require(options['HTTPObserver'] == flags.get('-http-observer') == 'disabled', 'single-record comparison must explicitly bypass the HTTP observer')
    require(float(flags['-database-cpus']) == options['CPUs'] == fixture['options']['DatabaseCPUs'] == 1, 'database quota differs')
    require(fixture['host_os'] == 'linux' and fixture['options']['BatchSize'] == 32 and fixture['options']['PendingRecords'] == 4096 and fixture['options']['SearchRetryAttempts'] == 4, 'controlled fixture policy differs')
    require(options['Processes'] == 4 and options['QueuePages'] == 1 and options['Timeout'] == 10*10**9, 'controlled client policy differs')
    require(cleanup['owner'] == fixture['owner'] and fixture['owner'] and not cleanup['errors'], 'fixture ownership/cleanup error')
    containers = fixture['containers']
    require(len(containers) == len(cleanup['containers']) == 1 and containers[0]['backend'] == 'search', 'unexpected database fixture')
    for key in ('id', 'name', 'image'):
        require(containers[0][key] == cleanup['containers'][0][key], 'cleanup container identity mismatch')
    require(cleanup['containers'][0]['removed'] is True and cleanup['processes'] and all(item['stopped'] is True for item in cleanup['processes']), 'owned process/container remains')
    require(any(node['ProcessID'] in [process['pid'] for process in cleanup['processes']] for node in fixture['nodes']), 'last fixture process absent from cleanup')
    # Cleanup records the final process only. ReplaceServer synchronously waits for
    # each previous process in code; its termination has no separate OS receipt.
    require(fixture['binary_sha256'] in [server['sha256'] for server in receipt['servers']], 'fixture server binary absent from receipt')
    modes = options.get('Modes') or ['fixed-rate', 'saturation']
    if 'Modes' in options:
        require(flags['-modes'].split(',') == modes, 'command/report mode mismatch')
    cases = []
    for profile in options['Profiles']:
        for mode in modes:
            if mode == 'fixed-rate':
                cases.extend((profile, mode, options['Concurrency'][-1], rate) for rate in options['Rates'] or [])
            elif mode == 'saturation':
                cases.extend((profile, mode, workers, 0) for workers in options['Concurrency'])
            else:
                raise ValueError('unknown crawler mode')
    expected_stages = []
    for case_index, case in enumerate(cases):
        for round_index in range(options['Rounds']):
            for offset in range(3):
                expected_stages.append((*case, PATHS[(case_index + round_index + offset) % 3], round_index+1))
    require(len(report['Stages']) <= len(expected_stages), 'extra stage in matrix')
    require(len(report['Stages']) == len(expected_stages) or report.get('incomplete'), 'truncated matrix without an incomplete report')
    stage_evidence = []
    for position, stage in enumerate(report['Stages']):
        evidence = dict(warmup=None, timed=None, checks_passed=False, check_error=None)
        try:
            actual = tuple(stage[key] for key in ('Profile', 'Mode', 'Concurrency', 'ProductsPerSecond', 'Path', 'Round'))
            require(actual == expected_stages[position], 'stage matrix/order mismatch')
            server = 'optimized' if stage['Path'] == 'optimized' else 'current'
            require(stage['ServerSHA256'] == report['Servers'][server]['sha256'], 'stage executable mismatch')
            evidence.update(verify_application_calls(stage, options))
            if stage['ElapsedNS'] == 0:
                require(report.get('incomplete') and stage['StartedNS'] == 0 and not stage['Clients'] and stage['CPUQuota'] is None and all(stage[key] == 0 for key in COUNTS), 'unexpected unmeasured stage')
                if stage['BeforeWarmupSettling']['settled']:
                    settling(stage['BeforeWarmupSettling'])
                evidence['check_error'] = 'stage did not reach timed measurement'
            else:
                if stage['Path'] != 'direct':
                    require(all(prom_value(stage[key], 'weir_store_active_executions_limit') == 0 for key in ('ServerBefore', 'ServerAfter')), 'live store execution-limit attestation differs')
                require(stage['CPUQuota']['container_id'] == containers[0]['id'] and stage['CPUQuota']['observed_cpus'] == stage['CPUQuota']['requested_cpus'] == 1 and stage['CPUQuota']['host_config_nano_cpus'] == 10**9 and stage['Resources']['database_allocated_cpus'] == 1, 'stage database identity/quota differs')
                first = settling(stage['BeforeWarmupSettling'])
                require(first == settling(stage['BeforeMeasurementSettling']) == tuple(sorted(stage['DatabaseBefore']['indices'])) == tuple(sorted(stage['DatabaseAfter']['indices'])), 'measured index identity differs')
                arithmetic(stage, options)
                require(stage['VerifiedDocuments'] == entry['workers'] * 6 + 1 if stage['Verified'] else stage['VerifiedDocuments'] == 0, 'seeded oracle record count differs')
                evidence['checks_passed'] = True
        except (ValueError, KeyError, TypeError, ZeroDivisionError) as error:
            evidence['check_error'] = str(error)
        stage_evidence.append(evidence)
    return stage_evidence


def aggregate(rows):
    keys = ('label', 'client_source', 'role', 'workflow_run', 'input_shape', 'profile', 'mode', 'offered_products_s', 'workers', 'path', 'after_max_in_flight')
    result = []
    for identity, grouped in itertools.groupby(sorted(rows, key=lambda row: tuple(row[key] for key in keys)), key=lambda row: tuple(row[key] for key in keys)):
        group = list(grouped)
        item = dict(zip(keys, identity))
        for key in ('completed', 'offered', 'rejected', 'failed', 'unknown', 'timeout', 'elapsed_s', 'sampled_cpu_ns', 'covered_cpu_s', 'application_call_attempts', 'application_records_submitted'):
            item[key] = sum(row[key] for row in group) if all(row[key] is not None for row in group) else None
        item['path_label'] = group[0]['path_label']
        item['http_observer'] = group[0]['http_observer']
        item['http_counter_status'] = group[0]['http_counter_status']
        item['attested_max_in_flight'] = group[0]['attested_max_in_flight']
        for field in ('observed_adapter_records', 'observed_adapter_executions'):
            item[field] = sum(row[field] for row in group) if all(row[field] is not None for row in group) else None
        item['observed_adapter_records_per_execution'] = item['observed_adapter_records'] / item['observed_adapter_executions'] if item['observed_adapter_executions'] and item['observed_adapter_records'] is not None else None
        item['rounds'] = len(group)
        item['formal_measurement'] = len(group) == 3 and all(row['formal_measurement'] for row in group)
        item['formal_all_arrivals'] = len(group) == 3 and all(row['formal_all_arrivals'] for row in group)
        item['application_call_attempts_per_completed_product'] = item['application_call_attempts'] / item['completed'] if item['completed'] and item['application_call_attempts'] is not None else None
        item['application_records_per_call'] = item['application_records_submitted'] / item['application_call_attempts'] if item['application_call_attempts'] else None
        item['completed_products_s'] = item['completed'] / item['elapsed_s'] if item['elapsed_s'] else None
        item['db_cpu_coverage'] = item['covered_cpu_s'] / item['elapsed_s'] if item['elapsed_s'] else 0
        item['sampled_cpu_ns'] = sum(row['sampled_cpu_ns'] or 0 for row in group) if item['covered_cpu_s'] else None
        item['db_cpu_quota_percent'] = item['sampled_cpu_ns'] / (item['covered_cpu_s'] * 1e9) * 100 if item['covered_cpu_s'] else None
        item['sampled_db_cpu_ms_per_product'] = item['sampled_cpu_ns'] / 1e6 / item['completed'] if item['completed'] and item['sampled_cpu_ns'] is not None else None
        for key in ('indexed_documents', 'get_operations', 'refreshes', 'index_time_ms', 'refresh_time_ms', 'merge_time_ms', 'database_http_requests', 'database_http_request_body_bytes', 'database_http_response_body_bytes'):
            total = sum(row[key] for row in group) if all(row[key] is not None for row in group) else None
            item[key] = total
            item[key + '_per_product'] = total / item['completed'] if item['completed'] and total is not None else None
        for key in ('p99_planned_to_visible_ms', 'drain_s', 'max_generator_lag_ms'):
            values = [row[key] for row in group if row[key] is not None]
            item['worst_' + key] = max(values) if values else None
        result.append(item)
    return result


def ratios(rows):
    lookup = {(row['label'], row['profile'], row['mode'], row['offered_products_s'], row['workers'], row['round'], row['path']): row for row in rows}
    result = []
    for row in rows:
        candidates = [(name, row['profile']) for name in PATHS if PATHS.index(name) < PATHS.index(row['path'])]
        for path, profile in candidates:
            baseline = lookup.get((row['label'], profile, row['mode'], row['offered_products_s'], row['workers'], row['round'], path))
            if baseline is None:
                continue
            item = dict(label=row['label'], workflow_run=row['workflow_run'], mode=row['mode'], offered_products_s=row['offered_products_s'], workers=row['workers'], round=row['round'], path=row['path'], profile=row['profile'], input_shape=row['input_shape'], path_label=row['path_label'], baseline_label=baseline['path_label'], baseline_path=path, baseline_profile=profile, both_formal_measurement=row['formal_measurement'] and baseline['formal_measurement'], both_formal_all_arrivals=row['formal_all_arrivals'] and baseline['formal_all_arrivals'])
            for metric in ('completed_products_s', 'db_cpu_quota_percent', 'sampled_db_cpu_ms_per_product', 'p99_planned_to_visible_ms', 'indexed_documents_per_product', 'get_operations_per_product', 'database_http_requests_per_product', 'observed_adapter_records_per_execution', 'application_call_attempts_per_completed_product'):
                first, last = baseline[metric], row[metric]
                item[metric + '_ratio'] = last/first if first and last is not None else None
            result.append(item)
    return result


def compare_paired_rounds(paired):
    keys = ('label', 'workflow_run', 'input_shape', 'profile', 'baseline_profile', 'mode', 'offered_products_s', 'workers', 'path', 'baseline_path')
    comparisons = []
    for identity, grouped in itertools.groupby(sorted(paired, key=lambda item: tuple(item[key] for key in keys)), key=lambda item: tuple(item[key] for key in keys)):
        group = sorted(grouped, key=lambda item: item['round'])
        result = dict(zip(keys, identity))
        result['path_label'] = group[0]['path_label']
        result['baseline_label'] = group[0]['baseline_label']
        result['paired_rounds'] = len(group)
        complete = len(group) == 3 and [item['round'] for item in group] == [1, 2, 3]
        result['complete_three_rounds'] = complete
        result['all_rounds_formal_measurement'] = complete and all(item['both_formal_measurement'] for item in group)
        result['all_rounds_all_arrivals'] = complete and all(item['both_formal_all_arrivals'] for item in group)
        qps = [item['completed_products_s_ratio'] for item in group]
        cpu = [item['sampled_db_cpu_ms_per_product_ratio'] for item in group]
        result['all_rounds_qps_within_five_percent'] = complete and all(value is not None and 0.95 <= value <= 1.05 for value in qps)
        result['all_rounds_cpu_ms_per_product_decreased'] = complete and all(value is not None and value < 1 for value in cpu)
        for field, values in (('completed_products_s_ratio', qps), ('sampled_db_cpu_ms_per_product_ratio', cpu)):
            available = [value for value in values if value is not None]
            result[field + '_minimum'] = min(available) if available else None
            result[field + '_maximum'] = max(available) if available else None
        result['supports_lower_database_cpu_at_comparable_completed_throughput'] = result['all_rounds_all_arrivals'] and result['all_rounds_qps_within_five_percent'] and result['all_rounds_cpu_ms_per_product_decreased']
        comparisons.append(result)
    return comparisons


def cpu_pressure(stage):
    resources = stage['Resources']
    elapsed, quota = stage['ElapsedNS'], resources['database_allocated_cpus']
    count, covered, full, area = 0, 0, 0, 0.0
    for before, after in zip(resources['samples'] or [], (resources['samples'] or [])[1:]):
        span = min(after['elapsed_ns'], elapsed) - max(before['elapsed_ns'], 0)
        clock = after['elapsed_ns'] - before['elapsed_ns']
        if after.get('database_cpu_counter_clock_unix_ns', 0) > 0 and before.get('database_cpu_counter_clock_unix_ns', 0) > 0:
            clock = after['database_cpu_counter_clock_unix_ns'] - before['database_cpu_counter_clock_unix_ns']
        first, last = before.get('database_cpu_time_ns'), after.get('database_cpu_time_ns')
        if span <= 0 or clock <= 0 or first is None or last is None or last < first or before.get('error') or after.get('error'):
            continue
        require(quota > 0, 'invalid database CPU denominator')
        percent = (last-first) / clock / quota * 100
        count += 1
        covered += span
        area += percent * span
        if percent >= 90:
            full += span
    coverage = covered / elapsed if elapsed > 0 else 0
    mean = area / covered if covered else 0
    fraction = full / elapsed if elapsed > 0 else 0
    saturated = count >= 5 and coverage >= 0.8 and mean >= 90 and fraction >= 0.8
    require(resources['valid_cpu_intervals'] == count and resources['valid_cpu_sampling_elapsed_ns'] == covered and resources['measurement_elapsed_ns'] == elapsed, 'recorded CPU interval accounting differs')
    for name, expected in (('mean_database_cpu_budget_percent', mean), ('fraction_intervals_at_cpu_threshold', fraction), ('cpu_sampling_time_coverage', coverage)):
        require(math.isclose(resources[name], expected, rel_tol=1e-12, abs_tol=1e-9), 'recorded CPU pressure metric differs: ' + name)
    require(resources['sustained_cpu_saturation'] == saturated, 'recorded sustained CPU gate differs')
    result = dict(valid_cpu_intervals=count, covered_cpu_ns=covered, mean_cpu_percent=mean, coverage=coverage, fraction_measurement_at_90_percent=fraction, sustained=saturated)
    return result


def select_formal(rows):
    for workers in WORKERS:
        eligible = [row for row in rows if row['role'] == 'calibration' and row['workers'] == workers and row['native_selection_eligible']]
        if not eligible:
            continue
        selected = sorted(eligible, key=lambda row: (-row['completed_products_s'], row['offered_products_s']))[0]
        q = math.floor(selected['completed_products_s'])
        rates = sorted({max(1, 4*q//5), max(1, q), max(1, 6*q//5)})
        result = dict(status='selected_from_native_calibration', workers=workers, workflow_run=selected['workflow_run'], label=selected['label'], native_offered_products_s=selected['offered_products_s'], native_completed_products_s_including_drain=selected['completed_products_s'], q=q, rates=rates, selected_whole_run_go_audit_pass=selected['whole_run_go_audit_pass'], calibration_is_capacity_evidence=False)
        return result
    result = dict(status='no_eligible_sustained_cpu_native_stage', workers=None, rates=[], formal_dispatch_permitted_by_plan=False, calibration_is_capacity_evidence=False)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--auditor', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    manifest, rows, runs = load(args.manifest), [], []
    require(manifest['experiment'] == 'crawler-pressure-2026-10-15' and manifest['client_source'] == CLIENT_SOURCE, 'experiment/client source differs')
    sources = manifest['server_sources']
    require(sources == dict(current=BASELINE_SOURCE, optimized=CANDIDATE_SOURCE), 'frozen server sources differ')
    require(manifest['client_ref'] == 'randy/crawler-pressure-client', 'frozen client branch differs')
    require(manifest['calibration'] == dict(worker_levels=list(WORKERS), rates=list(RATES), duration_ns=5*10**9, warmup_ns=2*10**9, rounds=1, planned_timed_stages=18), 'frozen calibration controls differ')
    require(manifest['controls'] == dict(input_shape='single', profiles=['two-step'], http_observer='disabled', modes=['fixed-rate'], processes=4, page_size=1, queue_pages=1, timeout_ns=10*10**9, after_max_in_flight=0, database_cpus=1, database_memory_bytes=1536*2**20, database_heap_bytes=512*2**20, batch_size=32, pending_records=4096), 'frozen common controls differ')
    require(manifest['formal']['duration_ns'] == 20*10**9 and manifest['formal']['warmup_ns'] == 10*10**9 and manifest['formal']['rounds'] == 3, 'frozen formal duration/rounds differ')
    if manifest['formal_entries']:
        require(manifest['formal']['selection']['workers'] == 512 and manifest['formal']['selection']['rates'] == [45, 57, 68], 'formal selection differs from the frozen pre-dispatch result')
        require(sorted(entry['rate'] for entry in manifest['formal_entries']) == [45, 57, 68] and all(entry['workers'] == 512 and entry['role'] == 'formal' for entry in manifest['formal_entries']), 'formal worker/rate matrix differs')
    entries = manifest['entries'] + manifest['formal_entries']
    require(len(manifest['entries']) == 2 and sorted(entry['workers'] for entry in manifest['entries']) == list(WORKERS), 'calibration worker matrix differs')
    require(all(entry['role'] == 'calibration' and entry['rates'] == list(RATES) for entry in manifest['entries']), 'calibration role/rates differ')
    require(len({entry['label'] for entry in entries}) == len(entries) and len({entry['workflow_run'] for entry in entries}) == len(entries), 'duplicate labels/workflows')
    auditor_receipt = load(args.manifest.parent / 'auditor-build.json')
    auditor_digest = hashlib.sha256(args.auditor.read_bytes()).hexdigest()
    require(auditor_receipt['base_source'] == CLIENT_SOURCE and re.fullmatch('[0-9a-f]{64}', auditor_receipt['binary_sha256']) and auditor_receipt['unmodified_source_archive'] is True, 'declared historical auditor source differs')
    path_labels = dict(direct='Native single _update', current='Baseline Weir cap0', optimized='Metadata-filtered Weir cap0')
    for entry in entries:
        require(entry['workflow_run'] and re.fullmatch(r'https://github.com/batchstream/weir-tests/actions/runs/[0-9]+', entry['workflow_run']), 'missing/invalid workflow URL')
        dispatch = [item for item in manifest['dispatch_attempts'] if item['workflow_run'] == entry['workflow_run']]
        require(len(dispatch) == 1 and dispatch[0]['head_sha'] == CLIENT_SOURCE and dispatch[0]['head_branch'] == manifest['client_ref'] and dispatch[0]['role'] == entry['role'] and dispatch[0]['workers'] == entry['workers'], 'dispatch ledger source/role/worker level differs')
        if entry['role'] == 'formal':
            require(dispatch[0]['rate'] == entry['rate'], 'dispatch ledger formal rate differs')
        bundle = dict(entry=entry)
        report_path = args.manifest.parent / entry['report']
        archive_count = verify_archive(report_path.parent)
        workflow = load(report_path.parent / 'workflow-run.json.gz')
        require(workflow['headSha'] == CLIENT_SOURCE and workflow['headBranch'] == manifest['client_ref'] and workflow['url'] == entry['workflow_run'], 'archived GitHub workflow identity differs')
        for name in ('receipt', 'report', 'fixture_manifest', 'cleanup'):
            bundle[name] = load(args.manifest.parent / entry[name])
        report = bundle['report']
        source_error = None
        try:
            evidence = identities(bundle, CLIENT_SOURCE, sources)
        except (ValueError, KeyError, TypeError) as error:
            source_error = str(error)
            evidence = [dict(warmup=None, timed=None, checks_passed=False, check_error='run provenance failed: ' + source_error) for _ in report['Stages']]
        with tempfile.NamedTemporaryFile(mode='w', suffix='.json', encoding='utf-8') as temporary:
            json.dump(report, temporary)
            temporary.flush()
            audit = subprocess.run([str(args.auditor.resolve()), 'audit', temporary.name], capture_output=True, text=True, timeout=180)
        expected_stages = 9
        run = dict(label=entry['label'], role=entry['role'], workers=entry['workers'], workflow_run=entry['workflow_run'], workflow_conclusion=workflow['conclusion'], client_source=CLIENT_SOURCE, archive_files_verified=archive_count, provenance_verified=source_error is None, provenance_error=source_error, complete_matrix=len(report['Stages']) == expected_stages, observed_stages=len(report['Stages']), complete_report=not report.get('incomplete') and not report.get('cleanup_error'), report_incomplete=report.get('incomplete'), report_cleanup_error=report.get('cleanup_error'), runner_exit=bundle['receipt']['exit_code'], go_audit_exit=audit.returncode, go_audit_output=(audit.stdout+audit.stderr).strip(), host=bundle['receipt']['host'], host_cpu_model=bundle['receipt'].get('host_cpu_model'), database_image=bundle['fixture_manifest']['containers'][0]['image'])
        runs.append(run)
        for row, stage, proof in zip(summary.rows(report), report['Stages'], evidence):
            observed = stage['ElapsedNS'] > 0
            pressure_error, pressure = None, None
            if observed:
                try:
                    pressure = cpu_pressure(stage)
                except (ValueError, KeyError, TypeError, ZeroDivisionError) as error:
                    pressure_error = str(error)
            calls = application_ledger(stage['ApplicationRequests'])
            row.update(label=entry['label'], client_source=CLIENT_SOURCE, input_shape='single', path_label=path_labels[row['path']], role=entry['role'], workflow_run=entry['workflow_run'], after_max_in_flight=0, timed_observed=observed, http_observer='disabled', http_counter_status='unavailable', provenance_verified=source_error is None, stage_checks_passed=proof['checks_passed'], stage_check_error=proof['check_error'], cpu_pressure_check_error=pressure_error, whole_run_go_audit_pass=audit.returncode == 0, independent_expected_hash_verified=audit.returncode == 0, live_field_oracle_recorded_pass=stage['Verified'])
            row['timed_stage_checks_passed'] = observed and proof['checks_passed'] and stage['Verified'] and stage['ClientsExited'] and all(stage[key] == 0 for key in ('Failed', 'Indeterminate', 'TimedOut')) and not stage.get('verification_error') and not stage.get('client_exit_errors')
            row['warmup_ledger_verified'] = proof['warmup'] is not None
            for field in ('requests', 'records', 'completed', 'failed', 'unknown', 'timeout', 'rejected'):
                row['warmup_' + field] = proof['warmup'][field] if proof['warmup'] else None
            row.update(application_call_attempts=calls['requests'] if observed else None, application_records_submitted=calls['records'] if observed else None, application_minimum_records=calls['minimum_records'] if observed else None, application_maximum_records=calls['maximum_records'] if observed else None)
            row['application_call_attempts_per_completed_product'] = calls['requests'] / row['completed'] if observed and row['completed'] else None
            row['application_records_per_call'] = calls['records'] / calls['requests'] if observed and calls['requests'] else None
            row['native_http_update_attempts'] = calls['requests'] if observed and stage['Path'] == 'direct' else None
            row['weir_mutate_call_attempts'] = calls['requests'] if observed and stage['Path'] != 'direct' else None
            row.update(adapter_metrics(stage))
            row['attested_max_in_flight'] = prom_value(stage['ServerBefore'], 'weir_store_active_executions_limit') if observed and stage['Path'] != 'direct' else None
            cpu_ns, coverage, _ = summary.cpu_cost(stage)
            row['sampled_cpu_ns'], row['covered_cpu_s'] = cpu_ns, coverage * row['elapsed_s']
            row['sustained_database_cpu_saturation'] = pressure['sustained'] if pressure else False
            row['fraction_measurement_at_90_percent_cpu'] = pressure['fraction_measurement_at_90_percent'] if pressure else None
            row['valid_cpu_intervals'] = pressure['valid_cpu_intervals'] if pressure else None
            for key in ('database_http_requests', 'database_http_request_body_bytes', 'database_http_response_body_bytes'):
                row[key] = None
            if not observed:
                for key in ('p95_planned_to_visible_ms', 'p99_planned_to_visible_ms', 'max_generator_lag_ms', 'max_dispatch_lag_ms', 'drain_s'):
                    row[key] = None
            row['formal_measurement'] = entry['role'] == 'formal' and source_error is None and bundle['receipt']['exit_code'] == 0 and audit.returncode == 0 and row['qualified'] and row['timed_stage_checks_passed'] and pressure_error is None and report['Parameters']['Rounds'] == 3 and report['Parameters']['Duration'] == 20*10**9 and report['Parameters']['Warmup'] == 10*10**9 and coverage >= 0.8
            row['formal_all_arrivals'] = row['formal_measurement'] and row['rejected'] == 0 and row['offered'] == row['completed'] and row['p99_planned_to_visible_ms'] <= 5000 and row['drain_s'] <= 3 and row['max_generator_lag_ms'] <= 50
            row['native_selection_eligible'] = entry['role'] == 'calibration' and stage['Path'] == 'direct' and source_error is None and row['timed_stage_checks_passed'] and row['independent_expected_hash_verified'] and pressure_error is None and row['sustained_database_cpu_saturation'] and row['max_generator_lag_ms'] <= 50
            for key in ('indexed_documents', 'get_operations', 'database_http_requests'):
                row[key + '_per_product'] = row[key]/row['completed'] if row['completed'] and row[key] is not None else None
            rows.append(row)
    selection = select_formal(rows)
    if manifest['formal']['selection'] is not None:
        require(manifest['formal']['selection'] == selection, 'frozen formal selection differs from native-only rule')
    if manifest['formal_entries']:
        require(selection['status'] == 'selected_from_native_calibration' and manifest['formal']['selection'] == selection, 'formal dispatch lacks a frozen eligible native selection')
        require(sorted(entry['rate'] for entry in manifest['formal_entries']) == selection['rates'] and all(entry['workers'] == selection['workers'] and entry['role'] == 'formal' for entry in manifest['formal_entries']), 'formal entries differ from native-only selection')
    groups, paired = aggregate(rows), ratios(rows)
    comparisons = compare_paired_rounds(paired)
    for item in comparisons:
        native = [row for row in rows if row['label'] == item['label'] and row['offered_products_s'] == item['offered_products_s'] and row['path'] == 'direct']
        item['native_sustained_database_cpu_in_every_formal_round'] = len(native) == 3 and all(row['sustained_database_cpu_saturation'] and row['formal_measurement'] for row in native)
        item['supports_lower_database_cpu_under_demonstrated_native_pressure'] = item['supports_lower_database_cpu_at_comparable_completed_throughput'] and item['native_sustained_database_cpu_in_every_formal_round']
    frozen_sources = {name: hashlib.sha256(Path(__file__).with_name(name).read_bytes()).hexdigest() for name in ('analyze.py', 'summarize.py', 'plot.py', 'archive.py', 'test_analysis.py')}
    output = dict(schema=1, frozen_analysis_sources=frozen_sources, auditor_sha256=auditor_digest, auditor_declared_source=auditor_receipt['base_source'], auditor_historical_build_sha256=auditor_receipt['binary_sha256'], auditor_matches_historical_binary=auditor_digest == auditor_receipt['binary_sha256'], client_source=CLIENT_SOURCE, server_sources=sources, experimental_acceptance_gates=GATES, cpu_pressure_gate=dict(minimum_intervals=5, coverage=0.8, mean_percent=90, fraction_measurement_at_90_percent=0.8), formal_selection=selection, runs=runs, groups=groups, paired_round_ratios=paired, comparisons=comparisons, rows=rows, notes=['Calibration is diagnostic and cannot support capacity or performance claims.', 'CPU is clipped to the full stage including drain, weighted over sampled coverage without extrapolation.', 'Same configured quota does not establish equal actual load; compare paths only within one workflow and round.', 'Original whole-report Go audit failures remain failures; independently complete native stages need a separately recorded oracle proof before selection.', 'Disabled HTTP request and byte counters remain unavailable.', 'Full-field verification covers every seeded key; the working set grows with workers.', 'The historical auditor receipt declares source A; the caller supplies a rebuilt auditor. Its actual hash and historical-match flag are recorded, because cross-platform binaries need not have identical bytes.'])
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output / 'analysis.json').write_text(json.dumps(output, indent=2, allow_nan=False) + '\n')
    for filename, values in (('groups.csv', groups), ('paired-rounds.csv', paired), ('comparisons.csv', comparisons), ('stages.csv', rows)):
        csv_write(args.output / filename, values)
    print(json.dumps(dict(runs=len(runs), audit_failed_runs=sum(run['go_audit_exit'] != 0 for run in runs), stages=len(rows), timed_observations=sum(row['timed_observed'] for row in rows), eligible_native_stages=sum(row['native_selection_eligible'] for row in rows), formal_selection=selection)))


if __name__ == '__main__':
    main()
