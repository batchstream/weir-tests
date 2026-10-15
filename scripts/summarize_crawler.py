#!/usr/bin/env python3
"""Recalculate every crawler stage without hiding overloads or unavailable counters."""
import argparse
import csv
import json
from pathlib import Path


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


def rows(report):
    result = []
    parameters = report['Parameters']
    observer_mode = parameters.get('HTTPObserver') or 'enabled'
    modes = parameters.get('Modes') or ['fixed-rate', 'saturation']
    case_count = (len(parameters['Rates'] or []) if 'fixed-rate' in modes else 0) + (len(parameters['Concurrency']) if 'saturation' in modes else 0)
    expected_stages = len(parameters['Profiles']) * case_count * parameters['Rounds'] * 3
    report_clean = not report.get('incomplete') and not report.get('cleanup_error') and len(report['Stages']) == expected_stages
    for stage in report['Stages']:
        success = stage['Completed']
        cpu_ns, coverage, cpu_percent = cpu_cost(stage)
        before = index_totals(stage['DatabaseBefore'])
        after = index_totals(stage['DatabaseAfter'])
        qualified = report_clean and stage['Verified'] and stage['ClientsExited'] and not stage['Failed'] and not stage['Indeterminate'] and not stage.get('verification_error') and not stage.get('client_exit_errors')
        stable = stage['Mode'] == 'fixed-rate' and qualified and not stage['Rejected'] and not stage['TimedOut'] and stage['GeneratorLagNS'] < 50_000_000
        row = dict(http_observer=observer_mode,profile=stage['Profile'], mode=stage['Mode'], offered_products_s=stage['ProductsPerSecond'],
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
        for method in ('POST _bulk', 'POST _mget', 'POST _refresh', 'GET _refresh', 'GET monitor', 'GET index-metadata'):
            counter = (stage['HTTPDelta'] or {}).get(method) if observer_mode != 'disabled' else None
            prefix = method.replace(' ', '_')
            row[prefix + '_requests'] = counter['Requests'] if counter else (None if observer_mode == 'disabled' else 0)
            row[prefix + '_request_bytes'] = counter['RequestBytes'] if counter else (None if observer_mode == 'disabled' else 0)
            row[prefix + '_response_bytes'] = counter['ResponseBytes'] if counter else (None if observer_mode == 'disabled' else 0)
            row[prefix + '_failed_requests'] = counter['Failed'] if counter else (None if observer_mode == 'disabled' else 0)
            row[prefix + '_aborted_requests'] = counter.get('Aborted') if counter else (None if observer_mode == 'disabled' else 0)
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
