#!/usr/bin/env python3
"""Verify and aggregate crawler evidence. No failed or overloaded stage is discarded."""
import argparse
import csv
import gzip
import hashlib
import importlib.util
import itertools
import json
import math
from pathlib import Path
import re
import subprocess
import tempfile

SOURCES = dict(current='4fe287e5915766ec3f48c8f42abde6b884d89bdd', optimized='c3101c6628872345d6fb09b977ce853f25936b79')
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
    # Historical proxy body-copy aborts may be missing from old ledgers.
    # Delta arithmetic checks the recorded ledger, not request completeness.
    http_mode = options.get('HTTPObserver', 'enabled')
    if http_mode == 'disabled':
        require(all(stage[key] is None for key in ('HTTPBefore', 'HTTPAfter', 'HTTPDelta')), 'disabled HTTP observer unexpectedly has counters')
    else:
        require(http_mode == 'enabled' and all(isinstance(stage[key], dict) for key in ('HTTPBefore', 'HTTPAfter', 'HTTPDelta')), 'enabled HTTP observer ledger missing')
        for method in set(stage['HTTPBefore']) | set(stage['HTTPAfter']):
            for field in ('Requests', 'RequestBytes', 'ResponseBytes', 'Failed'):
                before_counter = stage['HTTPBefore'].get(method)
                after_counter = stage['HTTPAfter'].get(method)
                before = before_counter.get(field, 0) if before_counter else 0
                after = after_counter.get(field, 0) if after_counter else 0
                require(after >= before and stage['HTTPDelta'][method][field] == after - before, 'physical HTTP delta mismatch')
    for kind, key in enumerate(('ScheduledToVisible', 'DispatchToVisible')):
        calculated = histogram(pooled[kind], maxima[kind])
        require(all(stage[key][name] == value for name, value in calculated.items()), 'pooled histogram mismatch')
    require(stage['ElapsedNS'] >= options['Duration'] and stage['DrainNS'] == stage['ElapsedNS'] - options['Duration'], 'drain denominator mismatch')
    require(math.isclose(stage['CompletedProductsPerSecond'], stage['Completed'] / (stage['ElapsedNS']/1e9), rel_tol=1e-12), 'throughput denominator mismatch')


def identities(bundle, client_source):
    report, receipt, fixture, cleanup, entry = (bundle[key] for key in ('report', 'receipt', 'fixture_manifest', 'cleanup', 'entry'))
    options = report['Parameters']
    expected_sources = dict(SOURCES)
    if entry['role'] == 'cap':
        expected_sources['current'] = SOURCES['optimized']
        require(options['AfterMaxInFlight'] == 16, 'cap isolation experiment must compare 0 versus 16')
        require(receipt['servers'][0]['source'] == receipt['servers'][1]['source'] and receipt['servers'][0]['sha256'] == receipt['servers'][1]['sha256'], 'cap isolation requires identical source and executable hashes')
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
    http_mode = options.get('HTTPObserver', 'enabled')
    require(http_mode in ('enabled', 'disabled'), 'unknown HTTP observer mode')
    if 'HTTPObserver' in options:
        require(flags.get('-http-observer') == http_mode, 'command/report HTTP observer mismatch')
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
    require(len(report['Stages']) == len(cases) * options['Rounds'] * 3, 'incomplete stage matrix')
    for case_index, case in enumerate(cases):
        for round_index in range(options['Rounds']):
            for offset in range(3):
                stage = report['Stages'][(case_index * options['Rounds'] + round_index)*3 + offset]
                expected = (*case, PATHS[(case_index + round_index + offset) % 3], round_index+1)
                actual = tuple(stage[key] for key in ('Profile', 'Mode', 'Concurrency', 'ProductsPerSecond', 'Path', 'Round'))
                require(actual == expected, 'stage matrix/order mismatch')
                server = 'optimized' if stage['Path'] == 'optimized' else 'current'
                require(stage['ServerSHA256'] == report['Servers'][server]['sha256'], 'stage executable mismatch')
                if stage['ElapsedNS'] == 0:
                    require(report.get('incomplete') and stage['StartedNS'] == 0 and not stage['Clients'] and stage['CPUQuota'] is None and all(stage[key] == 0 for key in COUNTS), 'unexpected unmeasured stage')
                    if stage['BeforeWarmupSettling']['settled']:
                        settling(stage['BeforeWarmupSettling'])
                    continue
                if stage['Path'] != 'direct' and expected_sources[server] == SOURCES['optimized']:
                    limit = options['AfterMaxInFlight'] if stage['Path'] == 'optimized' else 0
                    require(all(prom_value(stage[key], 'weir_store_active_executions_limit') == limit for key in ('ServerBefore', 'ServerAfter')), 'live store execution-limit attestation differs')
                require(stage['CPUQuota']['container_id'] == containers[0]['id'] and stage['CPUQuota']['observed_cpus'] == stage['CPUQuota']['requested_cpus'] == 1 and stage['CPUQuota']['host_config_nano_cpus'] == 10**9 and stage['Resources']['database_allocated_cpus'] == 1, 'stage database identity/quota differs')
                server = 'optimized' if stage['Path'] == 'optimized' else 'current'
                require(stage['ServerSHA256'] == report['Servers'][server]['sha256'], 'stage executable mismatch')
                first = settling(stage['BeforeWarmupSettling'])
                require(first == settling(stage['BeforeMeasurementSettling']) == tuple(sorted(stage['DatabaseBefore']['indices'])) == tuple(sorted(stage['DatabaseAfter']['indices'])), 'measured index identity differs')
                arithmetic(stage, options)


def aggregate(rows):
    keys = ('label', 'client_source', 'role', 'workflow_run', 'profile', 'mode', 'offered_products_s', 'workers', 'path', 'after_max_in_flight')
    result = []
    for identity, grouped in itertools.groupby(sorted(rows, key=lambda row: tuple(row[key] for key in keys)), key=lambda row: tuple(row[key] for key in keys)):
        group = list(grouped)
        item = dict(zip(keys, identity))
        for key in ('completed', 'offered', 'rejected', 'failed', 'unknown', 'timeout', 'elapsed_s', 'sampled_cpu_ns', 'covered_cpu_s'):
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
        item['completed_products_s'] = item['completed'] / item['elapsed_s'] if item['elapsed_s'] else None
        item['db_cpu_coverage'] = item['covered_cpu_s'] / item['elapsed_s'] if item['elapsed_s'] else 0
        item['sampled_cpu_ns'] = sum(row['sampled_cpu_ns'] or 0 for row in group) if item['covered_cpu_s'] else None
        item['db_cpu_quota_percent'] = item['sampled_cpu_ns'] / (item['covered_cpu_s'] * 1e9) * 100 if item['covered_cpu_s'] else None
        item['sampled_db_cpu_ms_per_product'] = item['sampled_cpu_ns'] / 1e6 / item['completed'] if item['completed'] and item['sampled_cpu_ns'] is not None else None
        for key in ('indexed_documents', 'get_operations', 'refreshes', 'index_time_ms', 'refresh_time_ms', 'merge_time_ms', 'POST__bulk_requests', 'POST__mget_requests', 'POST__refresh_requests', 'GET_index-metadata_requests', 'database_http_requests', 'database_http_request_body_bytes', 'database_http_response_body_bytes'):
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
        if row['profile'] == 'combined':
            candidates.append((row['path'], 'two-step'))
        for path, profile in candidates:
            baseline = lookup.get((row['label'], profile, row['mode'], row['offered_products_s'], row['workers'], row['round'], path))
            if baseline is None:
                continue
            item = dict(label=row['label'], workflow_run=row['workflow_run'], mode=row['mode'], offered_products_s=row['offered_products_s'], workers=row['workers'], round=row['round'], path=row['path'], profile=row['profile'], baseline_path=path, baseline_profile=profile, both_formal_measurement=row['formal_measurement'] and baseline['formal_measurement'], both_formal_all_arrivals=row['formal_all_arrivals'] and baseline['formal_all_arrivals'])
            for metric in ('completed_products_s', 'db_cpu_quota_percent', 'sampled_db_cpu_ms_per_product', 'p99_planned_to_visible_ms', 'indexed_documents_per_product', 'get_operations_per_product', 'database_http_requests_per_product', 'observed_adapter_records_per_execution'):
                first, last = baseline[metric], row[metric]
                item[metric + '_ratio'] = last/first if first and last is not None else None
            result.append(item)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--repo', type=Path, default=Path.cwd())
    parser.add_argument('--auditor', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    spec = importlib.util.spec_from_file_location('crawler_summary', args.repo / 'scripts/summarize_crawler.py')
    summary = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(summary)
    manifest, rows, runs = load(args.manifest), [], []
    require(manifest['entries'] and len({entry['label'] for entry in manifest['entries']}) == len(manifest['entries']), 'empty/duplicate run labels')
    for entry in manifest['entries']:
        require(entry['role'] in ('calibration', 'main', 'business', 'cap'), 'unknown experiment role')
        bundle = dict(entry=entry)
        report_path = args.manifest.parent / entry['report']
        archived_files = verify_archive(report_path.parent) if report_path.suffix == '.gz' else None
        for name in ('receipt', 'report', 'fixture_manifest', 'cleanup'):
            bundle[name] = load(args.manifest.parent / entry[name])
        client_source = entry.get('client_source', manifest['client_source'])
        identities(bundle, client_source)
        report = bundle['report']
        with tempfile.NamedTemporaryFile(mode='w', suffix='.json', encoding='utf-8') as temporary:
            json.dump(report, temporary)
            temporary.flush()
            audit = subprocess.run([str(args.auditor.resolve()), 'audit', temporary.name], capture_output=True, text=True, timeout=60)
        server_sources = {name: server['receipt']['source'] for name, server in report['Servers'].items()}
        path_labels = dict(direct='Native atomic bulk', current='Current Weir', optimized='Optimized Weir')
        if entry['role'] == 'cap':
            path_labels.update(current='Weir cap0', optimized='Weir cap16')
        source_checks = dict(label=entry['label'], archive_files_verified=archived_files, client_source=client_source, http_observer=report['Parameters'].get('HTTPObserver', 'enabled'), server_sources=server_sources, path_labels=path_labels, provenance_verified=True, complete_report=not report.get('incomplete') and not report.get('cleanup_error'), report_incomplete=report.get('incomplete'), report_cleanup_error=report.get('cleanup_error'), runner_exit=bundle['receipt']['exit_code'], workflow_run=entry['workflow_run'], role=entry['role'], go_audit_exit=audit.returncode, go_audit_output=(audit.stdout + audit.stderr).strip(), host=bundle['receipt']['host'], host_cpu_model=bundle['receipt'].get('host_cpu_model'), database_image=bundle['fixture_manifest']['containers'][0]['image'])
        runs.append(source_checks)
        for row, stage in zip(summary.rows(report), report['Stages']):
            observed = stage['ElapsedNS'] > 0
            http_mode = report['Parameters'].get('HTTPObserver', 'enabled')
            row.update(label=entry['label'], client_source=client_source, path_label=path_labels[row['path']], role=entry['role'], workflow_run=entry['workflow_run'], after_max_in_flight=report['Parameters']['AfterMaxInFlight'], timed_observed=observed, http_observer=http_mode, http_counter_status='recorded proxy ledger' if observed and http_mode == 'enabled' else 'unavailable', timed_stage_checks_passed=observed and stage['Verified'] and stage['ClientsExited'] and all(stage[key] == 0 for key in ('Failed', 'Indeterminate', 'TimedOut')) and not stage.get('verification_error') and not stage.get('client_exit_errors'), live_field_oracle_recorded_pass=stage['Verified'])
            row.update(adapter_metrics(stage))
            row['attested_max_in_flight'] = prom_value(stage['ServerBefore'], 'weir_store_active_executions_limit') if observed and stage['Path'] != 'direct' else None
            cpu_ns, coverage, _ = summary.cpu_cost(stage)
            require(0 <= coverage <= 1, 'invalid CPU sampling coverage')
            row['sampled_cpu_ns'], row['covered_cpu_s'] = cpu_ns, coverage * row['elapsed_s']
            traffic = [value for key, value in (stage['HTTPDelta'] or {}).items() if not key.endswith(' monitor')]
            row['database_http_requests'] = sum(value['Requests'] for value in traffic)
            row['database_http_request_body_bytes'] = sum(value['RequestBytes'] for value in traffic)
            row['database_http_response_body_bytes'] = sum(value['ResponseBytes'] for value in traffic)
            if not observed or http_mode == 'disabled':
                for key in list(row):
                    if key.startswith(('POST_', 'GET_', 'database_http_')):
                        row[key] = None
            if not observed:
                for key in ('p95_planned_to_visible_ms', 'p99_planned_to_visible_ms', 'max_generator_lag_ms', 'max_dispatch_lag_ms', 'drain_s'):
                    row[key] = None
            row['formal_measurement'] = entry['role'] != 'calibration' and bundle['receipt']['exit_code'] == 0 and audit.returncode == 0 and row['qualified'] and all(row[key] == 0 for key in ('failed', 'unknown', 'timeout')) and report['Parameters']['Rounds'] == GATES['rounds'] and report['Parameters']['Duration'] >= GATES['duration_s'] * 1e9 and report['Parameters']['Warmup'] >= GATES['warmup_s'] * 1e9 and coverage >= GATES['cpu_coverage']
            row['formal_all_arrivals'] = row['formal_measurement'] and row['mode'] == 'fixed-rate' and row['rejected'] == 0 and row['offered'] == row['completed'] and row['p99_planned_to_visible_ms'] <= GATES['p99_ms'] and row['drain_s'] <= GATES['drain_s'] and row['max_generator_lag_ms'] <= GATES['generator_lag_ms']
            for key in ('indexed_documents', 'get_operations', 'database_http_requests'):
                row[key + '_per_product'] = row[key]/row['completed'] if row['completed'] and row[key] is not None else None
            rows.append(row)
    groups, paired = aggregate(rows), ratios(rows)
    output = dict(schema=1, client_source=manifest['client_source'], default_server_sources=SOURCES, experimental_acceptance_gates=GATES, cpu_note='Sampled CPU only, weighted by covered time; not extrapolated to unsampled intervals. Rejected arrivals have no completed latency. All rows and negative ratios are retained; comparisons are within one workflow run.', qualification_note='Formal measurement validates source/audit, duration, rounds, error-free dispatched business work and CPU coverage. Formal all-arrivals applies only to fixed-rate rows and additionally rejects dropped arrivals, excess latency, drain and generator lag. Saturation is observed throughput at the fixed database quota; it does not establish maximum capacity or equivalent all-arrivals latency.', http_note='Disabled observers provide no HTTP request/byte counts; those values remain null. Recorded proxy ledgers are observer measurements, not independent completeness proofs; historical proxy body-copy aborts may be missing from their counts.', cleanup_note='Cleanup receipts attest only the final owned process and removed database container. Earlier server replacement waits synchronously in ReplaceServer/stopProcess; there is no separate historical OS termination receipt.', runs=runs, groups=groups, paired_round_ratios=paired, rows=rows)
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output / 'analysis.json').write_text(json.dumps(output, indent=2, allow_nan=False) + '\n')
    csv_write(args.output / 'groups.csv', groups)
    csv_write(args.output / 'paired-rounds.csv', paired)
    csv_write(args.output / 'stages.csv', rows)
    totals = dict(runs=len(runs), audit_failed_runs=sum(run['go_audit_exit'] != 0 for run in runs), stages=len(rows), timed_observations=sum(row['timed_observed'] for row in rows), formal_measurements=sum(row['formal_measurement'] for row in rows), formal_all_arrivals=sum(row['formal_all_arrivals'] for row in rows), groups=len(groups), paired_rounds=len(paired))
    print(json.dumps(totals))


if __name__ == '__main__':
    main()
