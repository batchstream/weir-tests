#!/usr/bin/env python3
"""Reproduce the prospectively fixed mechanism/crawler controls without pooling runners."""
import argparse
import gzip
import hashlib
import importlib.util
import itertools
import json
import math
from pathlib import Path
import re
import subprocess
import sys
import tempfile

FOLDER = Path(__file__).resolve().parent
REPO = FOLDER.parents[1]
CLIENT = 'c0f9d6159ce542e5678f81d5d5f02846a157cbad'
SERVER = '49bab6ae1622fb389926269e976dd31385b075e5'
PATHS = ('direct', 'current', 'optimized')


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    result = importlib.util.module_from_spec(spec)
    sys.modules[name] = result
    spec.loader.exec_module(result)
    return result


# Reuse immutable historical arithmetic/archive helpers; never patch their sources.
historical = FOLDER.parent / 'crawler-pressure-2026-10-15'
module('summarize', historical / 'summarize.py')
helpers = module('frozen_pressure_helpers', historical / 'analyze.py')
observations = module('mechanism_observations', REPO / 'scripts/summarize_crawler.py')
controls_receipt = module('mechanism_controls', REPO / 'scripts/run_crawler.py')
require, load = helpers.require, helpers.load


def call_proof(clients, stage, phase):
    require(len(clients) == 4, 'client process count differs')
    totals = dict(requests=0, records=0, failed=0, unknown=0, timeout=0, rejected=0)
    pids = set()
    for process, reply in enumerate(clients):
        require(reply['Phase'] == phase and reply['PID'] > 0 and reply['PID'] not in pids and reply['InputShape'] == 'single' and len(reply['Workers']) == 128, 'client identity/shape differs')
        require(reply['NativePoolLimit'] == (384 if stage['Path'] == 'direct' else 0), 'native pool differs')
        pids.add(reply['PID'])
        child = dict(requests=0, records=0)
        for index, worker in enumerate(reply['Workers']):
            require(worker['pid'] == reply['PID'] and worker['worker'] == process*128+index, 'worker identity differs')
            ledger = helpers.application_ledger(worker['application_requests'])
            pages = worker['applied_pages'] or []
            successful_calls = len(pages) if stage['Profile'] == 'partial-update' else sum(4 + int((worker['worker']+page)%100 == 0) for page in pages)
            require(successful_calls <= ledger['records'] <= worker['requested_logical_mutations'], 'actual calls differ from successful/planned operations')
            if not worker['Failed']:
                require(ledger['records'] == successful_calls == worker['requested_logical_mutations'], 'successful work has extra/missing calls')
            require(worker['Offered'] == worker['Rejected'] + worker['Completed'] + worker['Failed'] and worker['Completed'] == len(pages), 'worker completion accounting differs')
            require(worker['Indeterminate'] <= worker['Failed'] and worker['TimedOut'] <= worker['Failed'] and 0 <= worker['queue_peak_pages'] <= 1, 'failure subset or queue differs')
            for field in ('requests', 'records'):
                child[field] += ledger[field]
                totals[field] += ledger[field]
            for name, field in (('failed', 'Failed'), ('unknown', 'Indeterminate'), ('timeout', 'TimedOut'), ('rejected', 'Rejected')):
                totals[name] += worker[field]
        declared = helpers.application_ledger(reply['ApplicationRequests'])
        require(all(declared[key] == value for key, value in child.items()), 'child application aggregation differs')
    return totals


def identities(bundle, manifest):
    report, receipt, fixture, cleanup, entry, workflow = (bundle[key] for key in ('report', 'receipt', 'fixture', 'cleanup', 'entry', 'workflow'))
    opts = report['Parameters']
    require(receipt['source'] == workflow['headSha'] == manifest['client_source'] == CLIENT and receipt['source_dirty'] is False, 'client source differs/dirty')
    require(workflow['headBranch'] == manifest['client_ref'] and workflow['url'] == receipt['workflow_run'] == entry['workflow_run'], 'workflow identity differs')
    require(receipt['client_binary_sha256'] == receipt['client_binary_after_sha256'] and re.fullmatch('[a-f0-9]{64}', receipt['client_binary_sha256']), 'client binary changed')
    require(len(receipt['servers']) == len(receipt['server_binary_after_sha256']) == 2, 'server receipts missing')
    require(receipt['servers'][0]['sha256'] == receipt['servers'][1]['sha256'] and receipt['servers'][0]['identity'] == receipt['servers'][1]['identity'], 'both paths must have identical server source identity and binary')
    for index, path in enumerate(('current', 'optimized')):
        server = receipt['servers'][index]
        require(server['source'] == server['identity']['revision'] == manifest['server_sources'][path] == SERVER and not server['source_dirty'] and server['identity']['dirty'] == 'false' and server['identity']['state'] == 'clean-commit', 'server source differs/dirty')
        require(server['sha256'] == receipt['server_binary_after_sha256'][index] == report['Servers'][path]['sha256'] and server == report['Servers'][path]['receipt'], 'server binary/report receipt differs')
    require(opts['InputShape'] == 'single' and opts['Profiles'] == [entry['profile']] and opts['Modes'] == ['fixed-rate'] and opts['Rates'] == [entry['rate']] and opts['Concurrency'] == [512], 'workload matrix differs')
    exact = dict(Processes=4, PageSize=1, QueuePages=1, Timeout=10*10**9, CPUs=1, AfterMaxInFlight=0, AfterBatchMaxWait=20*10**6, Warmup=entry['warmup_ns'], Duration=entry['duration_ns'], Rounds=entry['rounds'], HTTPObserver=entry['http_observer'])
    require(all(opts[key] == value for key, value in exact.items()), 'workload controls differ')
    command = receipt['command']
    require(len(command)%2 == 1 and len(set(command[1::2])) == len(command[1::2]), 'duplicate/invalid command flags')
    flags = dict(zip(command[1::2], command[2::2]))
    for flag, field in (('-client-processes', 'Processes'), ('-page-size', 'PageSize'), ('-queue-pages', 'QueuePages'), ('-after-max-in-flight', 'AfterMaxInFlight'), ('-rounds', 'Rounds')):
        require(int(flags[flag]) == opts[field], 'command integer differs: ' + flag)
    for flag, field in (('-warmup', 'Warmup'), ('-duration', 'Duration'), ('-timeout', 'Timeout'), ('-after-batch-max-wait', 'AfterBatchMaxWait')):
        require(helpers.duration(flags[flag]) == opts[field], 'command duration differs: ' + flag)
    require(flags['-rates'] == str(entry['rate']) and flags['-concurrency-levels'] == '512' and flags['-profiles'] == entry['profile'] and flags['-modes'] == 'fixed-rate' and flags['-input-shape'] == 'single' and flags['-http-observer'] == entry['http_observer'] == receipt['http_observer'] and receipt['input_shape'] == 'single' and float(flags['-database-cpus']) == 1, 'command shape/mode/rate differs')
    expected_receipt = controls_receipt.workload_controls(dict(CRAWLER_PROFILES=entry['profile'], CRAWLER_LEVELS='512', CRAWLER_PAGE_SIZE='1', CRAWLER_BATCH_MAX_WAIT='20ms'))
    require(receipt['workload_controls'] == expected_receipt, 'receipt logical units/retries/phase scope differ')
    require(fixture['host_os'] == receipt['host']['system'].lower() == 'linux' and len(fixture['nodes']) == 1, 'fixture host/nodes differ')
    require(all(fixture['options'][key] == value for key, value in dict(BatchSize=32, MaxInFlight=0, SearchRetryAttempts=4, PendingRecords=4096, DatabaseCPUs=1).items()), 'fixture limits differ')
    require(cleanup['owner'] == fixture['owner'] and fixture['owner'] and not cleanup['errors'] and len(fixture['containers']) == len(cleanup['containers']) == 1, 'owned cleanup failed')
    container = fixture['containers'][0]
    require(container['backend'] == 'search' and all(container[key] == cleanup['containers'][0][key] for key in ('id', 'name', 'image')) and cleanup['containers'][0]['removed'] and cleanup['processes'] and all(item['stopped'] for item in cleanup['processes']), 'owned resource identity/removal differs')
    require(fixture['binary_sha256'] == receipt['servers'][0]['sha256'], 'fixture binary differs')
    expected = [(PATHS[(round_index+offset)%3], round_index+1) for round_index in range(entry['rounds']) for offset in range(3)]
    require(len(report['Stages']) <= len(expected) and (len(report['Stages']) == len(expected) or report.get('incomplete')), 'unexpected stage matrix')
    return expected


def stage_proof(stage, report, entry, fixture):
    server_path = 'optimized' if stage['Path'] == 'optimized' else 'current'
    require(stage['ServerSHA256'] == report['Servers'][server_path]['sha256'], 'stage executable differs')
    require(stage['Profile'] == entry['profile'] and stage['Mode'] == 'fixed-rate' and stage['ProductsPerSecond'] == entry['rate'] and stage['Concurrency'] == 512, 'stage matrix differs')
    require(stage['BatchMaxWaitNS'] == (20*10**6 if stage['Path'] == 'optimized' else 0), 'stage collection wait differs')
    warmup = call_proof(stage['WarmupClients'], stage, 'warmup') if stage['WarmupClients'] else None
    phase_metric_checks(stage)
    if not stage['ElapsedNS']:
        require(report.get('incomplete') and not stage['Clients'] and stage['StartedNS'] == 0, 'unmeasured stage lacks failed report')
        return dict(passed=False, reason='did not reach timed measurement', warmup=warmup)
    require(warmup is not None and warmup['failed'] == warmup['unknown'] == warmup['timeout'] == 0, 'timed stage lacks successful warmup')
    calls = call_proof(stage['Clients'], stage, 'timed')
    require(all(before['PID'] == after['PID'] for before, after in zip(stage['WarmupClients'], stage['Clients'])), 'warmup process/pool changed')
    ledger = helpers.application_ledger(stage['ApplicationRequests'])
    require(all(ledger[key] == calls[key] for key in ('requests', 'records')), 'stage application sums differ')
    require(stage['ElapsedNS'] >= entry['duration_ns'] and stage['DrainNS'] == stage['ElapsedNS']-entry['duration_ns'] and math.isclose(stage['CompletedProductsPerSecond'], stage['Completed']/(stage['ElapsedNS']/1e9), rel_tol=1e-12), 'completion/drain denominator differs')
    for key in ('Offered', 'Rejected', 'Completed', 'Failed', 'Indeterminate', 'TimedOut', 'Late', 'WithinWindow'):
        require(stage[key] == sum(worker[key] for reply in stage['Clients'] for worker in reply['Workers']), 'stage business count differs')
    require(stage['Offered'] == stage['Rejected']+stage['Completed']+stage['Failed'] and stage['Completed'] == stage['Late']+stage['WithinWindow'], 'stage completion accounting differs')
    quota = stage['CPUQuota']
    require(quota['container_id'] == fixture['containers'][0]['id'] and quota['observed_cpus'] == quota['requested_cpus'] == 1 and quota['host_config_nano_cpus'] == 10**9 and stage['Resources']['database_allocated_cpus'] == 1, 'actual stage quota/container differs')
    before_indices = helpers.settling(stage['BeforeWarmupSettling'])
    require(before_indices == helpers.settling(stage['BeforeMeasurementSettling']) == tuple(sorted(stage['DatabaseBefore']['indices'])) == tuple(sorted(stage['DatabaseAfter']['indices'])), 'index identity/settling differs')
    require(stage['VerifiedDocuments'] == (3073 if stage['Verified'] else 0), 'full seeded oracle count differs')
    if stage['Path'] != 'direct':
        require(all(helpers.prom_value(stage[key], 'weir_store_active_executions_limit') == 0 for key in ('ServerBefore', 'ServerAfter')), 'adapter cap attestation differs')
    pressure = helpers.cpu_pressure(stage)
    return dict(passed=True, reason=None, warmup=warmup, timed=calls, pressure=pressure)


def phase_metric_checks(stage):
    for snapshot in (stage['ServerBefore'], stage['ServerAfter']):
        if snapshot.get('error'):
            continue
        parsed = {}
        for line in (snapshot.get('raw') or '').splitlines():
            match = re.fullmatch(r'(weir_search_[a-zA-Z0-9_]+)(?:\{(.*)\})?\s+(\S+)(?:\s+\d+)?', line.strip())
            if not match:
                continue
            labels = {key: json.loads(value) for key, value in re.findall(r'([a-zA-Z_][a-zA-Z0-9_]*)=("(?:\\.|[^"\\])*")', match[2] or '')}
            key = (match[1], tuple(sorted(labels.items())))
            require(key not in parsed and math.isfinite(float(match[3])), 'duplicate/nonfinite raw Search series')
            parsed[key] = float(match[3])
        declared = {(item['name'], tuple(sorted((item.get('labels') or {}).items()))): item['value'] for item in snapshot.get('metrics') or [] if item['name'].startswith('weir_search_')}
        require(parsed == declared, 'raw and parsed Search metrics differ')
    deltas = observations.search_metric_deltas(stage)
    if deltas is None:
        return
    histograms = {}
    for metric in deltas:
        if not metric['name'].endswith('_bucket') or metric['delta'] is None:
            continue
        labels = dict(metric['labels'])
        bound = labels.pop('le')
        key = (metric['name'][:-7], tuple(sorted(labels.items())))
        histograms.setdefault(key, []).append((float(bound), metric['delta']))
    for (name, labels), buckets in histograms.items():
        ordered = sorted(buckets)
        require(all(a[1] <= b[1] for a, b in zip(ordered, ordered[1:])), 'histogram delta buckets decreased')
        count = observations.search_metric_sum(deltas, name+'_count', dict(labels))
        require(math.isinf(ordered[-1][0]) and ordered[-1][1] == count, 'histogram infinite bucket/count differ')


def diagnostic_counts(stage, deltas):
    if stage['ElapsedNS'] <= 0:
        return dict(usable=False, checks=[], reason='unmeasured stage')
    checks = []
    if stage['Path'] == 'direct':
        actual = (stage['HTTPDelta'] or {}).get('POST _update', {}).get('Requests', 0)
        checks.append(dict(metric='native update attempts', internal=stage['ApplicationRequests']['requests'], observer=actual, equal=actual == stage['ApplicationRequests']['requests']))
    else:
        for operation, route in (('metadata', 'GET index-metadata'), ('mget', 'POST _mget'), ('bulk', 'POST _bulk'), ('refresh', 'POST _refresh')):
            internal = observations.search_metric_sum(deltas, 'weir_search_http_requests_total', dict(operation=operation))
            observed = (stage['HTTPDelta'] or {}).get(route, {}).get('Requests', 0)
            # An unused phase can have no initialized metric series; it stays unavailable.
            checks.append(dict(metric=operation+' attempts', internal=internal, observer=observed, equal=(internal == observed) if internal is not None else None))
        if stage['Profile'] == 'partial-update':
            records = observations.search_metric_sum(deltas, 'weir_search_http_request_records_sum', dict(operation='bulk'))
            checks.append(dict(metric='partial framed bulk records versus submitted calls', internal=records, observer=stage['ApplicationRequests']['records'], equal=records == stage['ApplicationRequests']['records']))
    usable = all(item['equal'] is not False for item in checks) and (stage['Path'] == 'direct' or any(item['metric'] == 'bulk attempts' and item['equal'] is True and item['internal'] > 0 for item in checks))
    return dict(usable=usable, checks=checks, reason=None if usable else 'attempt boundary/cancellation/drift or missing active-phase metrics; retain both observations')


def compare(rows):
    result = []
    for label in sorted({row['label'] for row in rows if row['role'] == 'formal'}):
        selected = [row for row in rows if row['label'] == label]
        for baseline, candidate in (('direct', 'current'), ('direct', 'optimized'), ('current', 'optimized')):
            pairs = []
            for round_number in (1, 2, 3):
                first = next((row for row in selected if row['path'] == baseline and row['round'] == round_number), None)
                last = next((row for row in selected if row['path'] == candidate and row['round'] == round_number), None)
                if first is None or last is None:
                    continue
                qps = last['completed_products_s']/first['completed_products_s'] if first['completed_products_s'] and last['completed_products_s'] is not None else None
                cpu = last['sampled_db_cpu_ms_per_product']/first['sampled_db_cpu_ms_per_product'] if first['sampled_db_cpu_ms_per_product'] and last['sampled_db_cpu_ms_per_product'] is not None else None
                pairs.append(dict(round=round_number, both_all_arrivals=first['formal_all_arrivals'] and last['formal_all_arrivals'], qps_ratio=qps, cpu_ms_per_completed_ratio=cpu))
            complete = len(pairs) == 3
            strict = complete and all(pair['both_all_arrivals'] and pair['qps_ratio'] is not None and .95 <= pair['qps_ratio'] <= 1.05 and pair['cpu_ms_per_completed_ratio'] is not None and pair['cpu_ms_per_completed_ratio'] < 1 for pair in pairs)
            native = [row for row in selected if row['path'] == 'direct']
            pressure = len(native) == 3 and all(row['formal_all_arrivals'] and row['sustained_database_cpu_saturation'] for row in native)
            result.append(dict(label=label, baseline=baseline, candidate=candidate, paired_rounds=pairs, supports_lower_database_cpu_at_comparable_completed_throughput=strict, native_sustained_cpu_in_every_formal_round=pressure, supports_lower_database_cpu_under_demonstrated_native_pressure=strict and pressure))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, default=FOLDER/'manifest.json')
    parser.add_argument('--auditor', type=Path, required=True)
    parser.add_argument('--output', type=Path, default=FOLDER/'analysis')
    args = parser.parse_args()
    manifest = load(args.manifest)
    require(manifest['client_source'] == CLIENT and manifest['server_sources'] == dict(current=SERVER, optimized=SERVER) and manifest['client_ref'] == 'randy/batch-mechanism-client', 'frozen identities differ')
    expected_entries = [('diagnostic-partial', 'partial-update', 512, 'enabled', 2, 5, 1), ('diagnostic-crawler', 'two-step', 68, 'enabled', 2, 5, 1), ('formal-partial', 'partial-update', 512, 'disabled', 30, 20, 3), ('formal-crawler', 'two-step', 68, 'disabled', 30, 20, 3)]
    require(len(manifest['entries']) == 4, 'fixed workflow matrix differs')
    for entry, expected in zip(manifest['entries'], expected_entries):
        actual = (entry['label'], entry['profile'], entry['rate'], entry['http_observer'], entry['warmup_ns']//10**9, entry['duration_ns']//10**9, entry['rounds'])
        require(actual == expected, 'predeclared workflow settings differ')
    for name in ('scripts/summarize_crawler.py', 'scripts/run_crawler.py'):
        frozen = subprocess.check_output(['git', 'show', CLIENT+':'+name], cwd=REPO)
        require((REPO/name).read_bytes() == frozen, 'source helper differs from frozen client: '+name)
    auditor_receipt = load(FOLDER/'auditor-build.json')
    require(auditor_receipt['base_source'] == CLIENT and auditor_receipt['unmodified_source_archive'], 'auditor source differs')
    rows, runs, diagnostics = [], [], []
    for entry in manifest['entries']:
        if not entry['workflow_run']:
            continue
        folder = FOLDER/'raw'/entry['label']
        archive_count = helpers.verify_archive(folder)
        dispatch = [item for item in manifest['dispatch_attempts'] if item['workflow_run'] == entry['workflow_run']]
        require(len(dispatch) == 1 and dispatch[0]['head_sha'] == CLIENT and dispatch[0]['head_branch'] == manifest['client_ref'] and dispatch[0]['label'] == entry['label'] and dispatch[0]['rate'] == entry['rate'], 'dispatch ledger differs')
        bundle = dict(entry=entry, **{name: load(folder/file) for name, file in dict(report='crawler.json.gz', receipt='crawler.receipt.json.gz', fixture='manifest.json.gz', cleanup='cleanup.json.gz', workflow='workflow-run.json.gz').items()})
        report = bundle['report']
        provenance_error = None
        try:
            expected = identities(bundle, manifest)
            require([(stage['Path'], stage['Round']) for stage in report['Stages']] == expected[:len(report['Stages'])], 'stage rotation differs')
        except (ValueError, KeyError, TypeError) as error:
            provenance_error = str(error)
        with tempfile.NamedTemporaryFile(mode='w', suffix='.json') as temporary:
            json.dump(report, temporary)
            temporary.flush()
            audit = subprocess.run([str(args.auditor.resolve()), 'audit', temporary.name], capture_output=True, text=True, timeout=180)
        complete = not report.get('incomplete') and not report.get('cleanup_error') and len(report['Stages']) == entry['planned_timed_stages']
        runs.append(dict(label=entry['label'], workflow_run=entry['workflow_run'], workflow_conclusion=bundle['workflow']['conclusion'], archive_files=archive_count, provenance_error=provenance_error, complete=complete, incomplete=report.get('incomplete'), cleanup_error=report.get('cleanup_error'), runner_exit=bundle['receipt']['exit_code'], go_audit_exit=audit.returncode, go_audit_output=(audit.stdout+audit.stderr).strip(), host=bundle['receipt']['host'], cpu_model=bundle['receipt'].get('host_cpu_model')))
        for row, stage in zip(observations.rows(report), report['Stages']):
            proof = dict(passed=False, reason=None, pressure=None)
            try:
                proof.update(stage_proof(stage, report, entry, bundle['fixture']))
            except (ValueError, KeyError, TypeError, ZeroDivisionError) as error:
                proof['reason'] = str(error)
            pressure = proof.get('pressure') or {}
            row.update(label=entry['label'], role=entry['role'], workflow_run=entry['workflow_run'], stage_checks_passed=proof['passed'], stage_check_error=proof['reason'], warmup_evidence=proof.get('warmup'), whole_run_go_audit_pass=audit.returncode == 0, timed_observed=stage['ElapsedNS'] > 0, sustained_database_cpu_saturation=pressure.get('sustained', False), fraction_measurement_at_90_percent_cpu=pressure.get('fraction_measurement_at_90_percent'))
            row.update(helpers.adapter_metrics(stage))
            row['formal_measurement'] = entry['role'] == 'formal' and complete and provenance_error is None and audit.returncode == 0 and bundle['receipt']['exit_code'] == 0 and proof['passed'] and row['qualified'] and row['db_cpu_coverage'] >= .8 and stage['Failed'] == stage['Indeterminate'] == stage['TimedOut'] == 0
            row['formal_all_arrivals'] = row['formal_measurement'] and stage['Rejected'] == 0 and stage['Offered'] == stage['Completed'] and row['p99_planned_to_visible_ms'] <= 5000 and row['drain_s'] <= 3 and row['max_generator_lag_ms'] <= 50
            if entry['role'] == 'diagnostic':
                delta = observations.search_metric_deltas(stage)
                diagnostic = diagnostic_counts(stage, delta)
                diagnostics.append(dict(label=entry['label'], path=stage['Path'], round=stage['Round'], **diagnostic))
            rows.append(row)
    comparisons = compare(rows)
    output = dict(schema=1, client_source=CLIENT, server_source=SERVER, auditor_declared_source=auditor_receipt['base_source'], auditor_actual_sha256=hashlib.sha256(args.auditor.read_bytes()).hexdigest(), auditor_historical_sha256=auditor_receipt['binary_sha256'], runs=runs, diagnostic_count_checks=diagnostics, comparisons=comparisons, rows=rows, analysis_source_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), helper_hashes={str(path.relative_to(REPO)): hashlib.sha256(path.read_bytes()).hexdigest() for path in (historical/'analyze.py', historical/'summarize.py', REPO/'scripts/summarize_crawler.py', REPO/'scripts/run_crawler.py')})
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output/'analysis.json').write_text(json.dumps(output, indent=2, allow_nan=False)+'\n')
    for name, values in (('stages.csv', rows), ('runs.csv', runs), ('comparisons.csv', comparisons)):
        flattened = [{key: json.dumps(value, sort_keys=True) if isinstance(value, (dict, list)) else value for key, value in item.items()} for item in values]
        helpers.csv_write(args.output/name, flattened)
    print(json.dumps(dict(runs=len(runs), stages=len(rows), audit_failed_runs=sum(run['go_audit_exit'] != 0 for run in runs), formal_all_arrivals=sum(row['formal_all_arrivals'] for row in rows), strict_positive_comparisons=sum(item['supports_lower_database_cpu_at_comparable_completed_throughput'] for item in comparisons))))


if __name__ == '__main__':
    main()
