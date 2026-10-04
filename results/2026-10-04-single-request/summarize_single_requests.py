#!/usr/bin/env python3
"""Present audited business throughput without relabeling it database capacity."""
import argparse
import json
from pathlib import Path


def read(path):
    return json.loads(path.read_text())


def report_rows(root):
    rows = []
    for path in sorted(root.rglob('*.json')):
        if path.name not in ('mongo.json', 'search.json'):
            continue
        report = read(path)
        if report.get('schema') != 3:
            continue
        backend = report['parameters']['dataset']['backend']
        write = report['parameters']['requested_write_percent']
        for comparison in report['observed_business_comparisons']:
            concurrency = comparison['concurrency']
            stages = [p['weir'] for p in report['pairs'] if p['concurrency'] == concurrency]
            records = sum(s['server_metrics']['timed_counter_deltas']['weir_store_batch_operations_sum'] for s in stages)
            invocations = sum(s['server_metrics']['timed_counter_deltas']['weir_store_batch_operations_count'] for s in stages)
            cpus = {point['path']: point['mean_database_cpu_budget_percent'] for point in report['points'] if point['concurrency'] == concurrency}
            row = {
                'backend': backend, 'write_percent': write, 'concurrency': concurrency,
                'direct_qps': comparison['direct_successful_operations_per_second'],
                'weir_qps': comparison['weir_successful_operations_per_second'],
                'change_percent': comparison['observed_weir_throughput_delta_percent'],
                'direct_p99_ms_upper_bound': comparison['direct_business_request_latency']['p99_upper_bound_ns'] / 1e6,
                'weir_p99_ms_upper_bound': comparison['weir_business_request_latency']['p99_upper_bound_ns'] / 1e6,
                'actual_adapter_batch_average': records / invocations,
                'mean_database_cpu_budget_percent': cpus,
                'database_saturated_comparisons': report['database_saturated_comparisons'],
                'observed_peaks': report['observed_business_peak_comparison'],
                'source_file': str(path.relative_to(root)),
            }
            rows.append(row)
    if len(rows) != 18 or len({(r['backend'], r['write_percent'], r['concurrency']) for r in rows}) != 18:
        raise ValueError('Expected six complete reports and exactly three predeclared concurrency levels each')
    return sorted(rows, key=lambda r: (r['backend'], r['write_percent'], r['concurrency']))


def write_stress_summary(root):
    audit = read(root / 'reviewer-stress-audit.json')
    if audit['status'] != 'INCOMPLETE_STRESS' or len(audit['complete_backend_reports']) != 5 or len(audit['incomplete_backend_reports']) != 1:
        raise ValueError('Expected actual five-complete, one-incomplete stress evidence')
    rows = []
    lines = ['These stress runs use separate runners from the primary matrix. Compare each row within its paired run; absolute QPS across tables is not controlled.', '',
             '| Database | Write % | Direct req/s | Weir req/s | Change | Actual batch |',
             '|---|---:|---:|---:|---:|---:|']
    for item in sorted(audit['complete_backend_reports'], key=lambda r: (r['backend'], r['write_percent'])):
        report = read(root / 'final-stress' / item['file'])
        comparison = next(c for c in report['observed_business_comparisons'] if c['concurrency'] == 512)
        stages = [p['weir'] for p in report['pairs'] if p['concurrency'] == 512]
        records = sum(s['server_metrics']['timed_counter_deltas']['weir_store_batch_operations_sum'] for s in stages)
        invocations = sum(s['server_metrics']['timed_counter_deltas']['weir_store_batch_operations_count'] for s in stages)
        row = {'backend': item['backend'], 'write_percent': item['write_percent'], 'concurrency': 512,
               'direct_qps': comparison['direct_successful_operations_per_second'], 'weir_qps': comparison['weir_successful_operations_per_second'],
               'change_percent': comparison['observed_weir_throughput_delta_percent'], 'actual_adapter_batch_average': records / invocations,
               'workflow': item['workflow'], 'source_file': item['file'], 'capacity': item['database_saturated_comparison']}
        rows.append(row)
        lines.append(f"| {row['backend']} | {row['write_percent']} | {row['direct_qps']:,.0f} | {row['weir_qps']:,.0f} | {row['change_percent']:+.1f}% | {row['actual_adapter_batch_average']:.2f} |")
    lines += ['| search | 10 | unavailable | unavailable | unavailable | unavailable |', '',
              'Search mixed-load round 1 at concurrency 512 failed during native warmup: 52 operation timeouts (47 errors and 5 UNKNOWN writes), maximum 10.003122920 seconds. Unknown write effects remain unconfirmed. The earlier Weir timed stage completed 209,844 verified operations, but does not create a paired 512 ratio. All 19 completed Search timed paths remain individually verified; five planned timed paths are absent.', '',
              'Overall stress evidence contains five complete backend reports plus one incomplete report, 139 individually successful timed paths and 17,575,059 successful timed operations. It is not a complete six-report PASS. No stress dataset establishes a paired database-saturated capacity ratio.']
    summary = {'source': audit['source'], 'weir_source': audit['weir_source'], 'status': audit['status'], 'rows': rows,
               'incomplete_backend_reports': audit['incomplete_backend_reports'], 'scope': audit['scope']}
    (root / 'stress-summary.json').write_text(json.dumps(summary, indent=2) + '\n')
    (root / 'stress-summary.md').write_text('\n'.join(lines) + '\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('archive', type=Path)
    args = parser.parse_args()
    root = args.archive.resolve()
    identities = read(root / 'run-index.json')
    for filename, expected_count in [('final-primary-audit.json', 6), ('final-control-audit.json', 12)]:
        audit = read(root / filename)
        if not audit['equal_timeout_policy_verified'] or len(audit['reports']) != expected_count:
            raise ValueError('Final strict equal-timeout audit missing: ' + filename)
        if audit['source'] != identities['test_source'] or audit['weir_source'] != identities['weir_source']:
            raise ValueError('Audit identity differs from the measured recipe: ' + filename)
    independent = read(root / 'reviewer-audit.json')
    controls = read(root / 'reviewer-control-audit.json')
    if 'primary' not in independent or len(controls['controls']) != 6:
        raise ValueError('Independent primary/control audit missing')
    for audit in [independent['primary'], independent['control_raw'], controls]:
        if audit['source'] != identities['test_source'] or audit['weir_source'] != identities['weir_source']:
            raise ValueError('Independent audit identity differs from the measured recipe')
    rows = report_rows(root / 'final-primary')
    summary = {'scope': 'Observed verified business throughput in the 8/32/128 ladder; database capacity requires independent sustained CPU and next-level plateau qualifications.',
               'matched_concurrency_rows': rows, 'same_runner_controls': controls['controls']}
    (root / 'business-summary.json').write_text(json.dumps(summary, indent=2) + '\n')
    lines = ['Primary matrix at total concurrency **128**, across four independent client processes:', '',
             '| Database | Write % | Direct req/s | Weir req/s | Change | Direct p99 ms | Weir p99 ms | Actual batch | DB CPU direct / Weir |',
             '|---|---:|---:|---:|---:|---:|---:|---:|---:|']
    for row in rows:
        if row['concurrency'] != 128:
            continue
        cpu = row['mean_database_cpu_budget_percent']
        lines.append(f"| {row['backend']} | {row['write_percent']} | {row['direct_qps']:,.0f} | {row['weir_qps']:,.0f} | {row['change_percent']:+.1f}% | {row['direct_p99_ms_upper_bound']:.2f} | {row['weir_p99_ms_upper_bound']:.2f} | {row['actual_adapter_batch_average']:.2f} | {cpu['direct']:.1f}% / {cpu['weir']:.1f}% |")
    lines += ['', 'Same-job control at concurrency 128 (limit 1 then 32; native baseline variation is reported):', '',
              '| Database | Write % | Weir limit 1 req/s | Weir limit 32 req/s | Change | Native baseline change | Batch 1 / 32 |',
              '|---|---:|---:|---:|---:|---:|---:|']
    for control in controls['controls']:
        if not control['same_single_job_runner_demonstrated']:
            raise ValueError('Same-runner control not demonstrated')
        row = next(r for r in control['comparisons'] if r['concurrency'] == 128)
        weir, direct = row['weir'], row['direct']
        lines.append(f"| {control['backend']} | {control['write_percent']} | {weir['limit_1_qps']:,.0f} | {weir['limit_32_qps']:,.0f} | {weir['change_percent']:+.1f}% | {direct['change_percent']:+.1f}% | {weir['limit_1_actual_batch_average']:.2f} / {weir['limit_32_actual_batch_average']:.2f} |")
    (root / 'business-summary.md').write_text('\n'.join(lines) + '\n')
    write_stress_summary(root)
    print('\n'.join(lines))


if __name__ == '__main__':
    main()
