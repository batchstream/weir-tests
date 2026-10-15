"""Offline checks for the prospectively specified pressure and selection gates."""
import copy
import unittest

import analyze


class PressureAnalysisTests(unittest.TestCase):
    def stage(self, percent=95, intervals=5):
        second = 10**9
        elapsed = intervals * second
        samples = [dict(elapsed_ns=n*second, database_cpu_counter_clock_unix_ns=10**18+n*second, database_cpu_time_ns=n*percent*second//100) for n in range(intervals+1)]
        resources = dict(database_allocated_cpus=1, samples=samples, valid_cpu_intervals=intervals, valid_cpu_sampling_elapsed_ns=elapsed, measurement_elapsed_ns=elapsed, mean_database_cpu_budget_percent=percent, fraction_intervals_at_cpu_threshold=1 if percent >= 90 else 0, cpu_sampling_time_coverage=1, sustained_cpu_saturation=percent >= 90 and intervals >= 5)
        result = dict(ElapsedNS=elapsed, Resources=resources)
        return result

    def test_pressure_requires_five_intervals_and_actual_sustained_cpu(self):
        self.assertTrue(analyze.cpu_pressure(self.stage())['sustained'])
        self.assertFalse(analyze.cpu_pressure(self.stage(intervals=4))['sustained'])
        self.assertFalse(analyze.cpu_pressure(self.stage(percent=89))['sustained'])
        stage = self.stage()
        stage['Resources']['sustained_cpu_saturation'] = False
        with self.assertRaisesRegex(ValueError, 'recorded sustained CPU'):
            analyze.cpu_pressure(stage)

    def test_selection_prefers_lowest_eligible_workers_then_native_completed_qps(self):
        rows = []
        for workers, offered, qps, eligible in ((512, 96, 70, False), (512, 384, 101.9, True), (512, 768, 99, True), (2048, 384, 200, True)):
            row = dict(role='calibration', workers=workers, native_selection_eligible=eligible, completed_products_s=qps, offered_products_s=offered, workflow_run=str(workers), label=str(workers), whole_run_go_audit_pass=True)
            rows.append(row)
        selected = analyze.select_formal(rows)
        self.assertEqual((selected['workers'], selected['native_offered_products_s'], selected['q'], selected['rates']), (512, 384, 101, [80, 101, 121]))
        negative = copy.deepcopy(rows)
        for row in negative:
            row['native_selection_eligible'] = False
        selected = analyze.select_formal(negative)
        self.assertEqual(selected['status'], 'no_eligible_sustained_cpu_native_stage')
        self.assertFalse(selected['formal_dispatch_permitted_by_plan'])


if __name__ == '__main__':
    unittest.main()
