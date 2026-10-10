#!/usr/bin/env python3
"""Recompute every-round before/after throughput, latency, RSS and command evidence."""
import argparse
from datetime import datetime
import json
from pathlib import Path


def metric_delta(metrics, name):
    counters = []
    empty = {}
    for stage in ('before', 'after'):
        matching = [item['value'] for item in metrics.get(stage, empty).get('metrics', [])
                    if item['name'] == name and not item.get('labels')]
        if len(matching) != 1:
            return None
        counters.append(matching[0])
    delta = counters[1] - counters[0]
    return delta if delta >= 0 else None


def comparison_parameters(report):
    parameters = dict(report['parameters'])
    dataset = dict(parameters['dataset'])
    for fixture_field in ('weir_seed', 'namespace'):
        dataset.pop(fixture_field, None)
    parameters['dataset'] = dataset
    return parameters


def measurement_arguments(receipt):
    arguments = receipt['command'][1:]
    result = []
    index = 0
    while index < len(arguments):
        if arguments[index] in ('-weir', '-server-receipt', '-output'):
            index += 2
        else:
            result.append(arguments[index])
            index += 1
    return result


def summarize(report, concurrency, path):
    values = [pair[path] for pair in report['pairs'] if pair['concurrency'] == concurrency]
    empty = {}
    succeeded = sum(value['succeeded'] for value in values)
    elapsed = sum(value['elapsed_ns'] for value in values)
    commands = {}
    for value in values:
        for name, count in value.get('timed_physical_database_command_deltas', empty).items():
            commands[name] = commands.get(name, 0) + count
    reads = sum(value['reads'] for value in values)
    writes = sum(value['writes'] for value in values)
    samples = [sample for value in values for sample in value['resources']['samples']]
    rss = max((sample.get('weir_memory_bytes', 0) for sample in samples), default=0)
    batch_count = sum(value.get('server_metrics', empty).get('timed_counter_deltas', empty).get('weir_store_batch_operations_count', 0) for value in values)
    batch_operations = sum(value.get('server_metrics', empty).get('timed_counter_deltas', empty).get('weir_store_batch_operations_sum', 0) for value in values)
    batch_buckets = {}
    for value in values:
        for bound, count in value.get('server_metrics', empty).get('timed_adapter_batch_cumulative_buckets', empty).items():
            batch_buckets[bound] = batch_buckets.get(bound, 0) + count
    p95 = [value['latency']['p95_upper_bound_ns'] / 1e6 for value in values]
    cpu_deltas, cpu_spans, cpu_per_operation, allocated_bytes, allocated_objects = [], [], [], [], []
    round_batch_averages = []
    gc_cycles, gc_pauses = [], []
    for value in values:
        metrics = value.get('server_metrics', empty)
        timed_deltas = metrics.get('timed_counter_deltas', empty)
        count = timed_deltas.get('weir_store_batch_operations_count', 0)
        operations = timed_deltas.get('weir_store_batch_operations_sum', 0)
        round_batch_averages.append(operations / count if count else None)
        delta = metric_delta(metrics, 'process_cpu_seconds_total')
        allocations = metric_delta(metrics, 'go_memstats_alloc_bytes_total')
        objects = metric_delta(metrics, 'go_memstats_mallocs_total')
        allocated_bytes.append(allocations)
        allocated_objects.append(objects)
        gc_cycles.append(metric_delta(metrics, 'go_gc_duration_seconds_count'))
        gc_pauses.append(metric_delta(metrics, 'go_gc_duration_seconds_sum'))
        if delta is None:
            cpu_per_operation.append(None)
            continue
        start = datetime.fromisoformat(metrics['before']['at_utc'])
        end = datetime.fromisoformat(metrics['after']['at_utc'])
        span = (end - start).total_seconds()
        if span <= 0:
            cpu_per_operation.append(None)
            continue
        cpu_deltas.append(delta)
        cpu_spans.append(span)
        cpu_per_operation.append(delta / value['succeeded'] * 1e6)
    pooled_p95 = None
    for comparison in report.get('observed_business_comparisons', []):
        if comparison['concurrency'] == concurrency:
            pooled_p95 = comparison[path + '_business_request_latency']['p95_upper_bound_ns'] / 1e6
    result = dict(successful_operations=succeeded, operations_per_second=succeeded * 1e9 / elapsed,
                  round_operations_per_second=[value['successful_operations_per_second'] for value in values],
                  round_p95_upper_ms=p95, pooled_p95_upper_ms=pooled_p95,
                  p95_round_range_ms=[min(p95), max(p95)], sampled_weir_peak_rss_mib=rss / 1048576,
                  adapter_batch_average=batch_operations / batch_count if batch_count else None,
                  round_adapter_batch_average=round_batch_averages,
                  adapter_batch_cumulative_buckets=batch_buckets,
                  adapter_batch_greater_than_16_fraction=(batch_count - batch_buckets['16']) / batch_count if batch_count and '16' in batch_buckets else None,
                  adapter_batch_exact_32_fraction=(batch_buckets['32'] - batch_buckets['31']) / batch_count if batch_count and '31' in batch_buckets and '32' in batch_buckets else None,
                  server_cpu_microseconds_per_operation=sum(cpu_deltas) / succeeded * 1e6 if len(cpu_deltas) == len(values) else None,
                  server_mean_cpu_cores=sum(cpu_deltas) / sum(cpu_spans) if cpu_spans else None,
                  round_server_cpu_microseconds_per_operation=cpu_per_operation,
                  server_allocated_bytes_per_operation=sum(allocated_bytes) / succeeded if all(value is not None for value in allocated_bytes) else None,
                  server_allocated_objects_per_operation=sum(allocated_objects) / succeeded if all(value is not None for value in allocated_objects) else None,
                  round_server_allocated_bytes_per_operation=[delta / value['succeeded'] if delta is not None else None for value, delta in zip(values, allocated_bytes)],
                  round_server_gc_cycles=gc_cycles,
                  round_server_gc_pause_seconds=gc_pauses,
                  server_gc_pause_microseconds_per_operation=sum(gc_pauses) / succeeded * 1e6 if all(value is not None for value in gc_pauses) else None,
                  server_resource_scope='Prometheus counter differences between snapshots wrapping each measured Weir stage; includes small metrics collection overhead; RSS is a sampled maximum',
                  physical_commands=commands,
                  physical_commands_available=all('timed_physical_database_command_deltas' in value for value in values),
                  reads=reads, writes=writes,
                  errors=sum(value['errors'] for value in values),
                  indeterminate=sum(value['indeterminate'] for value in values),
                  business_operation_timeouts=sum(value.get('business_operation_timeouts', 0) for value in values),
                  verified=all(value['verified'] for value in values) and not report.get('incomplete'))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--before', type=Path, required=True)
    parser.add_argument('--after', type=Path, required=True)
    parser.add_argument('--backend', choices=('mongo', 'search'), required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    rows, missing_cases, case_provenance = [], [], []
    expected_client = None
    expected_servers = None
    for case in ('read', 'mixed', 'write', 'lua', 'bulk-read', 'bulk-write'):
        before_path = args.before / case / (args.backend + '.json')
        after_path = args.after / case / (args.backend + '.json')
        if not before_path.exists() or not after_path.exists():
            if case in ('read', 'mixed', 'write', 'lua') or before_path.exists() or after_path.exists():
                missing = [phase for phase, path in (('before', before_path), ('after', after_path)) if not path.exists()]
                missing_case = dict(case=case, missing=missing)
                missing_cases.append(missing_case)
            continue
        before, after = json.loads(before_path.read_text()), json.loads(after_path.read_text())
        before_receipt = json.loads((args.before / (case + '.receipt.json')).read_text())
        after_receipt = json.loads((args.after / (case + '.receipt.json')).read_text())
        receipts = (before_receipt, after_receipt)
        if any(receipt.get('exit_code') != 0 for receipt in receipts):
            raise SystemExit(case + ': benchmark process did not exit successfully')
        if before_receipt['versions'] != after_receipt['versions']:
            raise SystemExit(case + ': before/after pinned version manifests differ')
        servers = tuple(receipt['server_binary_sha256'] for receipt in receipts)
        if expected_servers is None:
            expected_servers = servers
        if servers != expected_servers:
            raise SystemExit(case + ': a server binary changed within the matrix')
        clients = [(receipt['client_source_tree_sha256'], receipt['client_binary_sha256']) for receipt in receipts]
        if expected_client is None:
            expected_client = clients[0]
        if any(client != expected_client for client in clients):
            raise SystemExit(case + ': client source or binary differs between cases/phases')
        if comparison_parameters(before) != comparison_parameters(after):
            raise SystemExit(case + ': before/after benchmark parameters differ')
        if len(before['parameters']['batch_sizes']) != 1:
            raise SystemExit(case + ': summarize one batch size per report to avoid mixing workloads')
        if measurement_arguments(before_receipt) != measurement_arguments(after_receipt):
            raise SystemExit(case + ': before/after measurement command arguments differ')
        paired_rounds = before['parameters']['paired_rounds']
        pair_keys = [(pair['concurrency'], pair['batch_size'], pair['round'], pair['order']) for pair in before['pairs']]
        after_keys = [(pair['concurrency'], pair['batch_size'], pair['round'], pair['order']) for pair in after['pairs']]
        if pair_keys != after_keys:
            raise SystemExit(case + ': before/after paired round execution order differs')
        for concurrency in before['parameters']['concurrency_levels']:
            for batch in before['parameters']['batch_sizes']:
                rounds = [pair['round'] for pair in before['pairs'] if pair['concurrency'] == concurrency and pair['batch_size'] == batch]
                if sorted(rounds) != list(range(1, paired_rounds + 1)):
                    raise SystemExit(case + ': incomplete or duplicate paired rounds')
        evidence = dict(case=case, parameters=comparison_parameters(before),
                        command_arguments=measurement_arguments(before_receipt),
                        before_server_binary_sha256=before_receipt['server_binary_sha256'],
                        after_server_binary_sha256=after_receipt['server_binary_sha256'],
                        before_receipt=str((args.before / (case + '.receipt.json')).resolve()),
                        after_receipt=str((args.after / (case + '.receipt.json')).resolve()))
        case_provenance.append(evidence)
        for concurrency in before['parameters']['concurrency_levels']:
            left, right = {}, {}
            for path in ('direct', 'weir'):
                left[path] = summarize(before, concurrency, path)
                right[path] = summarize(after, concurrency, path)
            row = dict(case=case, concurrency=concurrency, before=left, after=right,
                       weir_change_percent=100 * (right['weir']['operations_per_second'] / left['weir']['operations_per_second'] - 1),
                       direct_change_percent=100 * (right['direct']['operations_per_second'] / left['direct']['operations_per_second'] - 1),
                       scope='all ' + str(paired_rounds) + ' paired rounds; p95 values are histogram upper bounds; bulk p95 remains per-round rather than averaging quantiles')
            rows.append(row)
    provenance = dict(client_source_tree_sha256=expected_client[0] if expected_client else None,
                      client_binary_sha256=expected_client[1] if expected_client else None,
                      cases=case_provenance)
    status = 'partial' if missing_cases else 'complete'
    summary = dict(status=status, missing_cases=missing_cases, backend=args.backend, before=str(args.before.resolve()), after=str(args.after.resolve()), provenance=provenance, rows=rows)
    role_path = args.after / 'experiment-role.json'
    if role_path.exists():
        summary['after_experiment_role'] = json.loads(role_path.read_text())
    args.output.with_suffix('.json').write_text(json.dumps(summary, indent=2) + '\n')
    lines = ['# Observed before/after performance', '',
             '**Status: ' + status + '.** ' + ('Missing evidence: ' + ', '.join(item['case'] + ' ' + '/'.join(item['missing']) for item in missing_cases) if missing_cases else 'All core cases have complete matching paired rounds.'), '',
             'All paired rounds are included. Throughput is total successful operations divided by total measured time. Latencies are histogram upper bounds. Native MongoDB runs have no enforced CPU quota; these measurements do not prove maximum database capacity.', '',
             'Server CPU and Go allocation costs use the Prometheus counter differences around each timed stage, divided by successful records. These include small metrics collection overhead. RSS is the maximum observed process sample. A batch-size histogram without separate 31 and 32 boundaries cannot establish the exact 32-record full-batch fraction.', '',
             'Each included case has the same frozen client source, binary, benchmark parameters and paired round order. Source tree: `' + str(provenance['client_source_tree_sha256']) + '`. Binary: `' + str(provenance['client_binary_sha256']) + '`.', '',
             '| Case | Total workers | Before Weir ops/s | After Weir ops/s | Change | Direct drift | Before / after p95 ms | Before / after RSS MiB | Before / after CPU µs/op | Before / after allocation KiB/op | Before / after adapter batch | Verified |',
             '| --- | ---: | ---: | ---: | ---: | ---: | --- | --- | --- | --- | --- | --- |']
    if 'after_experiment_role' in summary:
        role = summary['after_experiment_role']
        lines[2:2] = ['**After experiment: ' + role['role'] + '.** ' + role['status'] + '.', '']
    for row in rows:
        before, after = row['before']['weir'], row['after']['weir']
        latency = []
        for value in (before, after):
            p95 = value['pooled_p95_upper_ms']
            if p95 is None:
                low, high = value['p95_round_range_ms']
                latency.append(f'{low:.3f}–{high:.3f}')
            else:
                latency.append(f'{p95:.3f}')
        verified = all(value['verified'] and value['errors'] == 0 and value['indeterminate'] == 0 and value['business_operation_timeouts'] == 0
                       for phase in (row['before'], row['after']) for value in phase.values())
        cpu, allocation, batch = [], [], []
        for value in (before, after):
            observed = value['server_cpu_microseconds_per_operation']
            cpu.append(f'{observed:.2f}' if observed is not None else 'unavailable')
            observed = value['server_allocated_bytes_per_operation']
            allocation.append(f'{observed / 1024:.2f}' if observed is not None else 'unavailable')
            observed = value['adapter_batch_average']
            batch.append(f'{observed:.2f}' if observed is not None else 'unavailable')
        lines.append(f"| {row['case']} | {row['concurrency']} | {before['operations_per_second']:.1f} | {after['operations_per_second']:.1f} | {row['weir_change_percent']:+.2f}% | {row['direct_change_percent']:+.2f}% | {' / '.join(latency)} | {before['sampled_weir_peak_rss_mib']:.1f} / {after['sampled_weir_peak_rss_mib']:.1f} | {' / '.join(cpu)} | {' / '.join(allocation)} | {' / '.join(batch)} | {verified} |")
    notes_path = args.after / 'comparison-notes.md'
    if notes_path.exists():
        lines.extend(['', notes_path.read_text().strip()])
    args.output.with_suffix('.md').write_text('\n'.join(lines) + '\n')
    print(args.output.with_suffix('.md'))


if __name__ == '__main__':
    main()
