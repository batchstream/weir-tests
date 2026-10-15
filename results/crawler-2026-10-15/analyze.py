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
    for method in set(stage['HTTPBefore']) | set(stage['HTTPAfter']):
        for field in ('Requests', 'RequestBytes', 'ResponseBytes', 'Failed'):
            before = stage['HTTPBefore'].get(method, {}).get(field, 0)
            after = stage['HTTPAfter'].get(method, {}).get(field, 0)
            require(after >= before and stage['HTTPDelta'][method][field] == after - before, 'physical HTTP delta mismatch')
    for kind, key in enumerate(('ScheduledToVisible', 'DispatchToVisible')):
        calculated = histogram(pooled[kind], maxima[kind])
        require(all(stage[key][name] == value for name, value in calculated.items()), 'pooled histogram mismatch')
    require(stage['ElapsedNS'] >= options['Duration'] and stage['DrainNS'] == stage['ElapsedNS'] - options['Duration'], 'drain denominator mismatch')
    require(math.isclose(stage['CompletedProductsPerSecond'], stage['Completed'] / (stage['ElapsedNS']/1e9), rel_tol=1e-12), 'throughput denominator mismatch')


def identities(bundle, client_source):
    report, receipt, fixture, cleanup, entry = (bundle[key] for key in ('report', 'receipt', 'fixture_manifest', 'cleanup', 'entry'))
    options = report['Parameters']
    require(receipt['source'] == client_source and receipt['source_dirty'] is False and re.fullmatch('[0-9a-f]{40}', client_source), 'client source mismatch/dirty')
    require(receipt['workflow_run'] == entry['workflow_run'] and receipt['host']['system'] == 'Linux', 'workflow/host mismatch')
    require(receipt['client_binary_sha256'] == receipt['client_binary_after_sha256'] and re.fullmatch('[0-9a-f]{64}', receipt['client_binary_sha256']), 'client executable changed')
    require(len(receipt['servers']) == len(receipt['server_binary_after_sha256']) == 2, 'missing server binary receipt')
    for index, name in enumerate(('current', 'optimized')):
        server = receipt['servers'][index]
        require(server['source'] == SOURCES[name] and server['identity']['revision'] == SOURCES[name] and server['source_dirty'] is False, 'server immutable source mismatch')
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
        require([int(value) for value in flags[flag].split(',') if value] == (options[field] or []), 'command/report matrix mismatch')
    require(flags['-profiles'].split(',') == options['Profiles'] and flags['-output'] == options['Output'], 'command/report profile/output mismatch')
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
    cases = [(profile, mode, workers, rate) for profile in options['Profiles'] for mode, workers, rate in ([('fixed-rate', options['Concurrency'][-1], rate) for rate in options['Rates'] or []] + [('saturation', workers, 0) for workers in options['Concurrency']])]
    require(len(report['Stages']) == len(cases) * options['Rounds'] * 3, 'incomplete stage matrix')
    for case_index, case in enumerate(cases):
        for round_index in range(options['Rounds']):
            for offset in range(3):
                stage = report['Stages'][(case_index * options['Rounds'] + round_index)*3 + offset]
                expected = (*case, PATHS[(case_index + round_index + offset) % 3], round_index+1)
                actual = tuple(stage[key] for key in ('Profile', 'Mode', 'Concurrency', 'ProductsPerSecond', 'Path', 'Round'))
                require(actual == expected, 'stage matrix/order mismatch')
                require(stage['CPUQuota']['container_id'] == containers[0]['id'] and stage['CPUQuota']['observed_cpus'] == stage['CPUQuota']['requested_cpus'] == 1 and stage['CPUQuota']['host_config_nano_cpus'] == 10**9 and stage['Resources']['database_allocated_cpus'] == 1, 'stage database identity/quota differs')
                server = 'optimized' if stage['Path'] == 'optimized' else 'current'
                require(stage['ServerSHA256'] == report['Servers'][server]['sha256'], 'stage executable mismatch')
                first = settling(stage['BeforeWarmupSettling'])
                require(first == settling(stage['BeforeMeasurementSettling']) == tuple(sorted(stage['DatabaseBefore']['indices'])) == tuple(sorted(stage['DatabaseAfter']['indices'])), 'measured index identity differs')
                arithmetic(stage, options)


def aggregate(rows):
    keys = ('label', 'role', 'workflow_run', 'profile', 'mode', 'offered_products_s', 'workers', 'path', 'after_max_in_flight')
    result = []
    for identity, grouped in itertools.groupby(sorted(rows, key=lambda row: tuple(row[key] for key in keys)), key=lambda row: tuple(row[key] for key in keys)):
        group = list(grouped)
        item = dict(zip(keys, identity))
        for key in ('completed', 'offered', 'rejected', 'failed', 'unknown', 'timeout', 'elapsed_s', 'sampled_cpu_ns', 'covered_cpu_s'):
            item[key] = sum(row[key] for row in group) if all(row[key] is not None for row in group) else None
        item['rounds'] = len(group)
        item['formal_measurement'] = len(group) == 3 and all(row['formal_measurement'] for row in group)
        item['formal_all_arrivals'] = len(group) == 3 and all(row['formal_all_arrivals'] for row in group)
        item['completed_products_s'] = item['completed'] / item['elapsed_s']
        item['db_cpu_coverage'] = item['covered_cpu_s'] / item['elapsed_s']
        item['sampled_cpu_ns'] = sum(row['sampled_cpu_ns'] or 0 for row in group) if item['covered_cpu_s'] else None
        item['db_cpu_quota_percent'] = item['sampled_cpu_ns'] / (item['covered_cpu_s'] * 1e9) * 100 if item['covered_cpu_s'] else None
        item['sampled_db_cpu_ms_per_product'] = item['sampled_cpu_ns'] / 1e6 / item['completed'] if item['completed'] and item['sampled_cpu_ns'] is not None else None
        for key in ('indexed_documents', 'get_operations', 'refreshes', 'index_time_ms', 'refresh_time_ms', 'merge_time_ms', 'POST__bulk_requests', 'POST__mget_requests', 'POST__refresh_requests', 'GET_index-metadata_requests', 'database_http_requests', 'database_http_request_body_bytes', 'database_http_response_body_bytes'):
            total = sum(row[key] for row in group) if all(row[key] is not None for row in group) else None
            item[key] = total
            item[key + '_per_product'] = total / item['completed'] if item['completed'] and total is not None else None
        for key in ('p99_planned_to_visible_ms', 'drain_s', 'max_generator_lag_ms'):
            item['worst_' + key] = max(row[key] for row in group)
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
            for metric in ('completed_products_s', 'db_cpu_quota_percent', 'sampled_db_cpu_ms_per_product', 'p99_planned_to_visible_ms', 'indexed_documents_per_product', 'get_operations_per_product', 'database_http_requests_per_product'):
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
        for name in ('receipt', 'report', 'fixture_manifest', 'cleanup'):
            bundle[name] = load(args.manifest.parent / entry[name])
        identities(bundle, manifest['client_source'])
        report = bundle['report']
        with tempfile.NamedTemporaryFile(mode='w', suffix='.json', encoding='utf-8') as temporary:
            json.dump(report, temporary)
            temporary.flush()
            audit = subprocess.run([str(args.auditor.resolve()), 'audit', temporary.name], capture_output=True, text=True, timeout=60)
        source_checks = dict(label=entry['label'], workflow_run=entry['workflow_run'], role=entry['role'], go_audit_exit=audit.returncode, go_audit_output=(audit.stdout + audit.stderr).strip(), host=bundle['receipt']['host'], host_cpu_model=bundle['receipt'].get('host_cpu_model'), database_image=bundle['fixture_manifest']['containers'][0]['image'])
        runs.append(source_checks)
        for row, stage in zip(summary.rows(report), report['Stages']):
            row.update(label=entry['label'], role=entry['role'], workflow_run=entry['workflow_run'], after_max_in_flight=report['Parameters']['AfterMaxInFlight'])
            cpu_ns, coverage, _ = summary.cpu_cost(stage)
            require(0 <= coverage <= 1, 'invalid CPU sampling coverage')
            row['sampled_cpu_ns'], row['covered_cpu_s'] = cpu_ns, coverage * row['elapsed_s']
            traffic = [value for key, value in stage['HTTPDelta'].items() if not key.endswith(' monitor')]
            row['database_http_requests'] = sum(value['Requests'] for value in traffic)
            row['database_http_request_body_bytes'] = sum(value['RequestBytes'] for value in traffic)
            row['database_http_response_body_bytes'] = sum(value['ResponseBytes'] for value in traffic)
            row['formal_measurement'] = entry['role'] != 'calibration' and bundle['receipt']['exit_code'] == 0 and audit.returncode == 0 and row['qualified'] and all(row[key] == 0 for key in ('failed', 'unknown', 'timeout')) and report['Parameters']['Rounds'] == GATES['rounds'] and report['Parameters']['Duration'] >= GATES['duration_s'] * 1e9 and report['Parameters']['Warmup'] >= GATES['warmup_s'] * 1e9 and coverage >= GATES['cpu_coverage']
            row['formal_all_arrivals'] = row['formal_measurement'] and row['mode'] == 'fixed-rate' and row['rejected'] == 0 and row['offered'] == row['completed'] and row['p99_planned_to_visible_ms'] <= GATES['p99_ms'] and row['drain_s'] <= GATES['drain_s'] and row['max_generator_lag_ms'] <= GATES['generator_lag_ms']
            for key in ('indexed_documents', 'get_operations', 'database_http_requests'):
                row[key + '_per_product'] = row[key]/row['completed'] if row['completed'] and row[key] is not None else None
            rows.append(row)
    groups, paired = aggregate(rows), ratios(rows)
    output = dict(schema=1, client_source=manifest['client_source'], server_sources=SOURCES, experimental_acceptance_gates=GATES, cpu_note='Sampled CPU only, weighted by covered time; not extrapolated to unsampled intervals. Rejected arrivals have no completed latency. All rows and negative ratios are retained; comparisons are within one workflow run.', qualification_note='Formal measurement validates source/audit, duration, rounds, error-free dispatched business work and CPU coverage. Formal all-arrivals applies only to fixed-rate rows and additionally rejects dropped arrivals, excess latency, drain and generator lag. Saturation is observed throughput at the fixed database quota; it does not establish maximum capacity or equivalent all-arrivals latency.', cleanup_note='Cleanup receipts attest only the final owned process and removed database container. Earlier server replacement waits synchronously in ReplaceServer/stopProcess; there is no separate historical OS termination receipt.', runs=runs, groups=groups, paired_round_ratios=paired, rows=rows)
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output / 'analysis.json').write_text(json.dumps(output, indent=2, allow_nan=False) + '\n')
    csv_write(args.output / 'groups.csv', groups)
    csv_write(args.output / 'paired-rounds.csv', paired)
    csv_write(args.output / 'stages.csv', rows)
    counts = dict(runs=len(runs), stages=len(rows), formal_measurements=sum(row['formal_measurement'] for row in rows), formal_all_arrivals=sum(row['formal_all_arrivals'] for row in rows), groups=len(groups), paired_rounds=len(paired))
    print(json.dumps(counts))


if __name__ == '__main__':
    main()
