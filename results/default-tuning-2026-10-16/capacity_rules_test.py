"""Counterexamples for the registered capacity and sustained-check criteria."""
import copy
import importlib.util
from pathlib import Path
import sys
import unittest

FOLDER = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('capacity_analysis_test', FOLDER / 'analyze.py')
analysis = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = analysis
spec.loader.exec_module(analysis)
rules, histogram = analysis.capacity_rules, analysis.helpers.histogram


def windows(counts, latency_ns=1_000_000):
    micros = (latency_ns + 999) // 1000
    exponent = micros.bit_length() - 1
    base = 1 << exponent
    latency_bin = min(255, 1 + exponent * 8 + (micros - base) * 8 // base)
    duration = len(counts) * rules.WIDTH_NS
    result = []
    for index, count in enumerate(counts + [0]):
        bins = [0] * 256
        bins[latency_bin] = count
        maximum = latency_ns if count else 0
        bucket = dict(offset_ns=min(index * rules.WIDTH_NS, duration), width_ns=rules.WIDTH_NS if index < len(counts) else 0, drain=index == len(counts), invalid_offsets=0, latency_histogram_buckets=bins, max_ns=maximum, latency=histogram(bins, maximum))
        bucket.update(dict.fromkeys(rules.COUNTERS, 0))
        bucket.update(offered=count, completed=count)
        result.append(bucket)
    return result


def row(path='weir', workers=128, rd=1):
    clients = [dict(pid=pid, coverage=1, cpu_cores=.1) for pid in (1, 2, 3, 4)]
    result = dict(label='formal', case_set='capacity', case_id='pure', profile=None, kind='write', phase='confirmation', database_cpus=.25, path=path, workers=workers, round=rd, usable=True, checks_passed=True, verified=True, completed=6000, completed_s=100 if path == 'direct' else 200, failed=0, unknown=0, timeout=0, db_cpu_coverage=.99, sustained_database_cpu_saturation=True, db_cpu_quota_percent=95, fraction_measurement_at_90_percent_cpu=.9, cpu_intervals=[{}] * 6, rate_stable=True, host_headroom=True, host_coverage=.99, host_busy_percent=60, host_steal_percent=0, effective_host_cpus=4, client_headroom=True, client_process_cpu=clients, p99_ms=1, elapsed_s=60)
    if workers == 512:
        result['completed_s'] *= 1.05
    return result


def capacity_rows():
    return [row(path, workers, rd) for path in ('direct', 'weir') for workers in (128, 512) for rd in (1, 2, 3)]


def crawler_rows():
    result = []
    for path in ('direct', 'optimized'):
        for rate in (128, 256, 512):
            for rd in (1, 2, 3):
                item = row(path, 512, rd)
                item.update(kind='crawler', case_id=str(rate), profile='partial-update', case_set='crawler', rate=rate, offered=rate*60, completed=rate*60, completed_s=rate, rejected=0, drain_s=1, max_generator_lag_ms=1)
                if rate == 512:
                    item.update(usable=False, completed=rate*60-6, rejected=6, p99_ms=6000, drain_s=4, rate_stable=False)
                result.append(item)
    return result


class BucketTests(unittest.TestCase):
    def test_closed_loop_offered_is_started_calls_not_zero(self):
        buckets = windows([100, 100])
        pooled = [sum(b['latency_histogram_buckets'][index] for b in buckets) for index in range(256)]
        totals = dict(offered=200, completed=200, rejected=0, failed=0, unknown=0, timed_out=0)
        options = dict(duration=20_000_000_000, pools=[buckets], histogram=histogram, totals=totals, latency=histogram(pooled, 1_000_000))
        self.assertEqual(sum(b['offered'] for b in rules.merge_buckets(options)), 200)
        for corruption in ('offered-zero', 'missing-drain', 'bool-count', 'bad-max', 'unknown', 'overflow'):
            changed = copy.deepcopy(options)
            if corruption == 'offered-zero':
                changed['totals']['offered'] = 0
            elif corruption == 'missing-drain':
                changed['pools'][0].pop()
            elif corruption == 'bool-count':
                changed['pools'][0][0]['completed'] = True
            elif corruption == 'bad-max':
                changed['pools'][0][-1]['latency']['max_observed_ns'] = 1
            elif corruption == 'unknown':
                changed['pools'][0][0]['unknown'] = 1
            else:
                changed['pools'][0][0]['latency_histogram_buckets'][0] = 1 << 64
            with self.subTest(corruption=corruption), self.assertRaises(ValueError):
                rules.merge_buckets(changed)

    def test_cross_window_completions_are_normal_but_backlog_growth_is_not(self):
        buckets = windows([90, 100, 100, 100, 100, 100])
        for bucket in buckets[:-1]:
            bucket['offered'] = 100
        options = dict(buckets=buckets, duration=60_000_000_000, histogram=histogram, rate=10)
        self.assertTrue(rules.stability(options)['rate_stable'])
        buckets[4]['offered'] += 11
        self.assertFalse(rules.stability(options)['rate_stable'])

    def test_soak_rejects_compounding_decline_and_minute_tail_latency(self):
        counts = [count for count in (100, 100, 100, 95, 89) for _ in range(6)]
        options = dict(buckets=windows(counts), duration=300_000_000_000, histogram=histogram)
        result = rules.stability(options)
        self.assertGreater(result['second_half_completed_s']/result['first_half_completed_s'], .9)
        self.assertEqual(result['final_first_minute_ratio'], .89)
        self.assertFalse(result['rate_stable'])
        options['buckets'] = windows([100]*30, 6_000_000_000)
        self.assertFalse(rules.stability(options)['rate_stable'])


class CapacityTests(unittest.TestCase):
    def test_three_round_saturation_plateau_and_comparable_actual_cpu(self):
        options = dict(rows=capacity_rows(), levels=[128, 512])
        result = rules.capacity(options)
        self.assertTrue(result['both_qualified'])
        self.assertTrue(result['comparable_actual_load_higher_capacity'])
        self.assertEqual([p['selected_workers'] for p in result['paths']], [128, 128])

    def test_cpu_host_client_and_next_point_are_independent_requirements(self):
        for field, value in (('db_cpu_coverage', .949), ('db_cpu_quota_percent', 89.9), ('fraction_measurement_at_90_percent_cpu', .799), ('host_busy_percent', 85.1), ('host_coverage', .899), ('rate_stable', False), ('client_process_cpu', []), ('completed_s', 300)):
            options = dict(rows=capacity_rows(), levels=[128, 512])
            options['rows'][-1][field] = value
            with self.subTest(field=field):
                self.assertFalse(rules.capacity(options)['both_qualified'])

    def test_duplicate_selection_and_mixed_quota_rejected(self):
        for change in ('duplicate', 'selection', 'quota'):
            rows = capacity_rows()
            if change == 'duplicate':
                rows.append(copy.deepcopy(rows[0]))
            elif change == 'selection':
                for item in rows:
                    item['phase'] = 'selection'
            else:
                rows[0]['database_cpus'] = 1
            options = dict(rows=rows, levels=[128, 512])
            with self.subTest(change=change), self.assertRaises(ValueError):
                rules.capacity(options)

    def test_rate_bound_needs_all_three_valid_service_failures(self):
        options = dict(rows=crawler_rows(), rates=[128, 256, 512])
        result = rules.crawler_rate_bounds(options)
        self.assertTrue(all(p['lower_rate'] == 256 and p['upper_rate'] == 512 for p in result['paths']))
        for change in ('max_generator_lag_ms', 'host_busy_percent', 'unknown'):
            modified = copy.deepcopy(options)
            modified['rows'][-1][change] = 51 if change == 'max_generator_lag_ms' else 86 if change == 'host_busy_percent' else 1
            with self.subTest(change=change):
                candidate = rules.crawler_rate_bounds(modified)['paths'][1]
                self.assertEqual(candidate['lower_rate'], 256)
                self.assertIsNone(candidate['upper_rate'])
                self.assertFalse(candidate['soak_eligible'])

    def test_ladder_top_is_only_a_lower_bound(self):
        options = dict(rows=[r for r in crawler_rows() if r['rate'] < 512], rates=[128, 256])
        self.assertTrue(all(p['lower_rate'] == 256 and p['upper_rate'] is None and not p['bounded'] for p in rules.crawler_rate_bounds(options)['paths']))

    def test_soak_requires_five_minutes_and_preserves_cpu_capacity_gate(self):
        stage = row()
        stage.update(phase='soak', elapsed_s=300, time_buckets=windows([100]*30))
        options = dict(row=stage, kind='write', histogram=histogram)
        self.assertTrue(rules.soak(options)['passed'])
        stage['db_cpu_quota_percent'] = 89
        self.assertFalse(rules.soak(options)['passed'])
        stage['time_buckets'] = windows([100]*6)
        with self.assertRaises(ValueError):
            rules.soak(options)


if __name__ == '__main__':
    unittest.main()
