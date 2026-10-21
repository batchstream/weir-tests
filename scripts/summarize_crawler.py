#!/usr/bin/env python3
"""Recalculate every crawler stage without hiding overloads or unavailable counters."""
import argparse
import csv
import json
import math
from pathlib import Path


REPORT_SCHEMA = 4


def cpu_cost(stage):
    elapsed = stage['ElapsedNS']
    quota = stage['Resources']['database_allocated_cpus']
    if elapsed <= 0 or quota <= 0:
        return None, 0, None
    cpu_ns = 0.0
    coverage_ns = 0
    samples = stage['Resources']['samples'] or []
    for before, after in zip(samples, samples[1:]):
        span = min(after['elapsed_ns'], elapsed) - max(before['elapsed_ns'], 0)
        clock_span = after.get('database_cpu_counter_clock_unix_ns', 0) - before.get('database_cpu_counter_clock_unix_ns', 0)
        if clock_span <= 0:
            clock_span = after['elapsed_ns'] - before['elapsed_ns']
        first = before.get('database_cpu_time_ns')
        last = after.get('database_cpu_time_ns')
        if span <= 0 or clock_span <= 0 or first is None or last is None or last < first or before.get('error') or after.get('error'):
            continue
        cpu_ns += (last - first) / clock_span * span
        coverage_ns += span
    return (cpu_ns, coverage_ns / elapsed, cpu_ns / coverage_ns / quota * 100) if coverage_ns else (None, 0, None)


def index_totals(raw):
    return (raw or {}).get('_all', {}).get('primaries', {})


def field_delta(before, after, group, field):
    first = before.get(group, {}).get(field)
    last = after.get(group, {}).get(field)
    if first is None or last is None or last < first:
        return None
    return last - first


SEARCH_METRICS = (
    'weir_search_http_requests_total',
    'weir_search_http_request_duration_seconds_',
    'weir_search_http_request_records_',
    'weir_search_http_request_body_bytes_',
    'weir_search_lua_evaluation_duration_seconds_',
)


def search_metric_deltas(stage):
    """Keep labeled counters/buckets; records count framed attempts, not acknowledgements."""
    snapshots = [stage['ServerBefore'], stage['ServerAfter']]
    if any(snapshot.get('error') for snapshot in snapshots):
        return None
    indexed = []
    for snapshot in snapshots:
        metrics = {}
        for metric in snapshot.get('metrics') or []:
            name = metric['name']
            if not any(name.startswith(prefix) for prefix in SEARCH_METRICS):
                continue
            labels = tuple(sorted((metric.get('labels') or {}).items()))
            key = (name, labels)
            if key in metrics:
                raise ValueError('duplicate Search metric series')
            metrics[key] = metric['value']
        indexed.append(metrics)
    if not any(indexed):
        return None
    result = []
    for name, labels in sorted(indexed[0].keys() | indexed[1].keys()):
        first = indexed[0].get((name, labels))
        last = indexed[1].get((name, labels))
        delta = last - first if first is not None and last is not None and math.isfinite(first) and math.isfinite(last) and last >= first else None
        result.append(dict(name=name, labels=dict(labels), delta=delta))
    return result


def search_metric_sum(deltas, name, labels):
    if deltas is None:
        return None
    matches = [metric['delta'] for metric in deltas if metric['name'] == name and all(metric['labels'].get(key) == value for key, value in labels.items())]
    return sum(matches) if matches and all(value is not None for value in matches) else None


def rows(report):
    if report.get('schema') != REPORT_SCHEMA:
        raise ValueError('crawler report schema must be ' + str(REPORT_SCHEMA))
    result = []
    parameters = report['Parameters']
    input_shape = parameters['InputShape']
    observer_mode = parameters['HTTPObserver']
    modes = parameters['Modes']
    case_count = (len(parameters['Rates'] or []) if 'fixed-rate' in modes else 0) + (len(parameters['Concurrency']) if 'saturation' in modes else 0)
    paths = parameters['Paths']
    expected_stages = len(parameters['Profiles']) * case_count * parameters['Rounds'] * len(paths)
    report_clean = not report.get('incomplete') and not report.get('cleanup_error') and len(report['Stages']) == expected_stages
    for stage in report['Stages']:
        success = stage['Completed']
        cpu_ns, coverage, cpu_percent = cpu_cost(stage)
        before = index_totals(stage['DatabaseBefore'])
        after = index_totals(stage['DatabaseAfter'])
        qualified = report_clean and stage['Verified'] and stage['ClientsExited'] and not stage['Failed'] and not stage['Indeterminate'] and not stage.get('verification_error') and not stage.get('client_exit_errors')
        stable = stage['Mode'] == 'fixed-rate' and qualified and not stage['Rejected'] and not stage['TimedOut'] and stage['GeneratorLagNS'] < 50_000_000
        configuration = stage['ServerConfiguration'] or {}
        stores = configuration.get('stores') or []
        batching = stores[0].get('batching', {}) if len(stores) == 1 else {}
        row = dict(visibility=parameters['Visibility'], seed_padding_bytes=parameters['SeedPaddingBytes'], batch_max_operations=batching.get('max_operations'), batch_max_bytes=batching.get('max_bytes'), batch_max_in_flight=batching['max_in_flight'] if batching else None, logical_unit='partial_update' if stage['Profile'] == 'partial-update' else 'crawler_product',input_shape=input_shape,http_observer=observer_mode,batch_max_wait_ms=stage['BatchMaxWaitNS'] / 1e6,profile=stage['Profile'], mode=stage['Mode'], offered_products_s=stage['ProductsPerSecond'],
                   workers=stage['Concurrency'], path=stage['Path'], round=stage['Round'],
                   qualified=qualified, stable_arrivals=stable, offered=stage['Offered'],
                   rejected=stage['Rejected'], completed=success, failed=stage['Failed'],
                   unknown=stage['Indeterminate'], timeout=stage['TimedOut'],
                   completed_within_arrival_window=stage['WithinWindow'], completed_in_drain=stage['Late'],
                   drain_s=stage['DrainNS'] / 1e9, elapsed_s=stage['ElapsedNS'] / 1e9,
                   completed_products_s=success / (stage['ElapsedNS'] / 1e9) if stage['ElapsedNS'] > 0 else None,
                   p95_planned_to_visible_ms=stage['ScheduledToVisible']['p95_upper_bound_ns'] / 1e6,
                   p99_planned_to_visible_ms=stage['ScheduledToVisible']['p99_upper_bound_ns'] / 1e6,
                   max_generator_lag_ms=stage['GeneratorLagNS'] / 1e6,
                   max_dispatch_lag_ms=stage['DispatchLagNS'] / 1e6,
                   db_cpu_coverage=coverage, db_cpu_quota_percent=cpu_percent,
                   sampled_db_cpu_ms_per_product=cpu_ns / 1e6 / success if success and cpu_ns is not None else None,
                   requested_logical_mutations=stage['RequestedMutations'],
                   indexed_documents=field_delta(before, after, 'indexing', 'index_total'),
                   index_time_ms=field_delta(before, after, 'indexing', 'index_time_in_millis'),
                   noop_updates=field_delta(before, after, 'indexing', 'noop_update_total'),
                   version_conflicts=field_delta(before, after, 'indexing', 'index_failed_due_to_version_conflict'),
                   get_operations=field_delta(before, after, 'get', 'total'),
                   refreshes=field_delta(before, after, 'refresh', 'total'),
                   refresh_time_ms=field_delta(before, after, 'refresh', 'total_time_in_millis'),
                   merge_time_ms=field_delta(before, after, 'merges', 'total_time_in_millis'),
                   verified_documents=stage['VerifiedDocuments'],
                   expected_documents_sha256=stage['ExpectedDocumentsSHA256'])
        application = stage['ApplicationRequests']
        row['application_request_attempts'] = application['requests']
        row['application_record_attempts'] = application['records']
        row['application_records_minimum'] = application['minimum_records']
        row['application_records_maximum'] = application['maximum_records']
        row['application_records_histogram'] = json.dumps(application['records_per_request'], sort_keys=True)
        for method in ('POST _update', 'POST _bulk', 'POST _mget', 'POST _refresh', 'GET _refresh', 'GET monitor', 'GET index-metadata'):
            counter = (stage['HTTPDelta'] or {}).get(method) if observer_mode != 'disabled' else None
            prefix = method.replace(' ', '_')
            row[prefix + '_requests'] = counter['Requests'] if counter else (None if observer_mode == 'disabled' else 0)
            row[prefix + '_request_bytes'] = counter['RequestBytes'] if counter else (None if observer_mode == 'disabled' else 0)
            row[prefix + '_response_bytes'] = counter['ResponseBytes'] if counter else (None if observer_mode == 'disabled' else 0)
            row[prefix + '_failed_requests'] = counter['Failed'] if counter else (None if observer_mode == 'disabled' else 0)
            row[prefix + '_aborted_requests'] = counter['Aborted'] if counter else (None if observer_mode == 'disabled' else 0)
        deltas = search_metric_deltas(stage)
        row['search_metric_deltas'] = json.dumps(deltas, sort_keys=True) if deltas is not None else None
        for operation in ('metadata', 'mget', 'bulk', 'refresh', 'search', 'other'):
            labels = dict(operation=operation)
            prefix = 'search_' + operation
            row[prefix + '_http_requests'] = search_metric_sum(deltas, 'weir_search_http_requests_total', labels)
            count = search_metric_sum(deltas, 'weir_search_http_request_duration_seconds_count', labels)
            seconds = search_metric_sum(deltas, 'weir_search_http_request_duration_seconds_sum', labels)
            row[prefix + '_http_duration_count'] = count
            row[prefix + '_http_duration_seconds'] = seconds
            row[prefix + '_http_duration_mean_ms'] = seconds * 1000 / count if seconds is not None and count else None
            body_count = search_metric_sum(deltas, 'weir_search_http_request_body_bytes_count', labels)
            body_sum = search_metric_sum(deltas, 'weir_search_http_request_body_bytes_sum', labels)
            row[prefix + '_request_body_bytes_count'] = body_count
            row[prefix + '_request_body_bytes_sum'] = body_sum
            row[prefix + '_request_body_bytes_mean'] = body_sum / body_count if body_sum is not None and body_count else None
            if operation in ('mget', 'bulk'):
                records_count = search_metric_sum(deltas, 'weir_search_http_request_records_count', labels)
                records_sum = search_metric_sum(deltas, 'weir_search_http_request_records_sum', labels)
                row[prefix + '_wire_records_count'] = records_count
                row[prefix + '_wire_records_sum'] = records_sum
                row[prefix + '_wire_records_mean'] = records_sum / records_count if records_sum is not None and records_count else None
        lua_count = search_metric_sum(deltas, 'weir_search_lua_evaluation_duration_seconds_count', {})
        lua_seconds = search_metric_sum(deltas, 'weir_search_lua_evaluation_duration_seconds_sum', {})
        row['search_lua_duration_count'] = lua_count
        row['search_lua_duration_seconds'] = lua_seconds
        row['search_lua_duration_mean_ms'] = lua_seconds * 1000 / lua_count if lua_seconds is not None and lua_count else None
        result.append(row)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--report', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    arguments = parser.parse_args()
    report = json.loads(arguments.report.read_text())
    observations = rows(report)
    arguments.output.parent.mkdir(parents=True, exist_ok=True)
    arguments.output.with_suffix('.json').write_text(json.dumps(observations, indent=2) + '\n')
    with arguments.output.with_suffix('.csv').open('w', newline='') as stream:
        writer = csv.DictWriter(stream, fieldnames=list(observations[0]) if observations else [], lineterminator='\n')
        writer.writeheader()
        writer.writerows(observations)
    summary = dict(stages=len(observations), qualified=sum(row['qualified'] for row in observations),
                   stable_arrivals=sum(row['stable_arrivals'] for row in observations))
    print(json.dumps(summary))


if __name__ == '__main__':
    main()
