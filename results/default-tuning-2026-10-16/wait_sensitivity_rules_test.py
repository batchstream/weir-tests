"""Boundary and failure cases for the preregistered collection-wait choice."""
import copy
import json
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import wait_sensitivity_rules as rules


def cohort_rows():
    rows = []
    for wait in (20, 40, 80, 160):
        for cohort, profile, rate in (('wait-partial-q1', 'partial-update', 512),
                                      ('wait-crawler-q1', 'two-step', 68)):
            for rd in (1, 2, 3):
                controls = dict(max_operations=128, max_bytes=4194304,
                                max_wait=str(wait) + 'ms', max_in_flight=16)
                clients = [dict(pid=pid, coverage=.99, cpu_cores=.1) for pid in (1, 2, 3, 4)]
                audit = dict(exit_code=0)
                row = dict(phase='wait_sensitivity', case_set=cohort,
                           case_id=cohort + '-w' + str(wait) + 'ms', kind='crawler',
                           profile=profile, path='optimized', database_cpus=1,
                           workers=512, round=rd, rate=rate, controls=controls,
                           wait_ns=wait * 1_000_000, usable=True, checks_passed=True,
                           verified=True, error=None, original_go_audit=audit,
                           offered=rate * 60, completed=rate * 60, completed_s=rate,
                           rejected=0, failed=0, unknown=0, timeout=0,
                           db_cpu_coverage=.99, db_cpu_ms_per_completed=1,
                           host_headroom=True, host_coverage=.99, host_busy_percent=50,
                           host_steal_percent=0, effective_host_cpus=4,
                           client_headroom=True, client_process_cpu=clients,
                           rate_stable=True, p99_ms=1000, drain_s=1,
                           max_generator_lag_ms=1)
                rows.append(row)
    return rows


class WaitChoiceTests(unittest.TestCase):
    def test_no_repeatable_improvement_keeps_twenty_and_unrelated_rows_do_not_vote(self):
        rows = cohort_rows()
        unrelated = dict(case_set='confirm-partial-q1', wait_ns=999, round=99)
        rows.append(unrelated)
        result = rules.choose_wait(rows)
        self.assertEqual(result['outcome'], 'selected')
        self.assertEqual(result['selected_wait_ms'], 20)
        self.assertEqual(result['cpu_plateau_waits_ms'], [20])
        self.assertTrue(result['ranking'][0]['eligible'])
        self.assertTrue(all(item['qualified'] for item in result['ranking']))
        self.assertTrue(all(not item['eligible'] for item in result['ranking'][1:]))

    def test_shortest_wait_within_five_percent_of_best_cpu_wins(self):
        rows = cohort_rows()
        cpu_by_wait = {20: 1, 40: .9, 80: .87, 160: .86}
        for row in rows:
            row['db_cpu_ms_per_completed'] = cpu_by_wait[row['wait_ns'] // 1_000_000]
        result = rules.choose_wait(rows)
        self.assertEqual(result['selected_wait_ms'], 40)
        self.assertEqual(result['cpu_plateau_waits_ms'], [40, 80, 160])
        self.assertAlmostEqual(result['best_cpu_score'], .86)
        self.assertEqual(len(result['ranking'][1]['comparisons']), 6)

    def test_five_percent_cpu_reduction_is_strict_in_every_round(self):
        for cpu, selected in ((.95, 20), (.949, 40)):
            rows = cohort_rows()
            for row in rows:
                if row['wait_ns'] == 40_000_000:
                    row['db_cpu_ms_per_completed'] = cpu
            with self.subTest(cpu=cpu):
                self.assertEqual(rules.choose_wait(rows)['selected_wait_ms'], selected)

        rows = cohort_rows()
        for row in rows:
            if row['wait_ns'] == 40_000_000:
                row['db_cpu_ms_per_completed'] = .5 if row['round'] < 3 else .96
        result = rules.choose_wait(rows)
        self.assertEqual(result['selected_wait_ms'], 20)
        self.assertLess(result['ranking'][1]['cpu_score'], .95)
        self.assertTrue(any('round 3: CPU reduction' in reason for reason in result['ranking'][1]['reasons']))

    def test_cpu_plateau_boundary_is_inclusive(self):
        for forty_score, selected in ((.84, 40), (.840001, 80)):
            rows = cohort_rows()
            for row in rows:
                if row['wait_ns'] == 40_000_000:
                    row['db_cpu_ms_per_completed'] = forty_score
                elif row['wait_ns'] == 80_000_000:
                    row['db_cpu_ms_per_completed'] = .8
            with self.subTest(forty_score=forty_score):
                self.assertEqual(rules.choose_wait(rows)['selected_wait_ms'], selected)

    def test_individual_profile_regression_cannot_hide_in_geometric_mean(self):
        for profile_cpu, selected in ((1.05, 40), (1.050001, 20)):
            rows = cohort_rows()
            for row in rows:
                if row['wait_ns'] == 40_000_000:
                    row['db_cpu_ms_per_completed'] = profile_cpu if row['profile'] == 'partial-update' else .5
            with self.subTest(profile_cpu=profile_cpu):
                self.assertEqual(rules.choose_wait(rows)['selected_wait_ms'], selected)

    def test_completed_throughput_band_is_inclusive_and_applies_to_each_profile(self):
        for ratio, selected in ((.95, 40), (1.05, 40), (.949999, 20), (1.050001, 20)):
            rows = cohort_rows()
            for row in rows:
                if row['wait_ns'] == 40_000_000:
                    row['db_cpu_ms_per_completed'] = .9
                    row['completed_s'] *= ratio
            with self.subTest(ratio=ratio):
                self.assertEqual(rules.choose_wait(rows)['selected_wait_ms'], selected)

    def test_original_quality_arrival_headroom_and_slo_gates_are_independent(self):
        failures = (('checks_passed', False), ('verified', False), ('usable', False),
                    ('rate_stable', False), ('error', 'startup failed'),
                    ('original_go_audit', None), ('db_cpu_coverage', .949),
                    ('host_headroom', False), ('host_coverage', .899),
                    ('host_busy_percent', 85.1), ('host_steal_percent', 5.1),
                    ('client_headroom', False), ('client_process_cpu', []),
                    ('failed', 1), ('unknown', 1), ('timeout', 1),
                    ('offered', 30719), ('completed', 30719), ('rejected', 1),
                    ('p99_ms', 5000.1), ('drain_s', 3.1),
                    ('max_generator_lag_ms', 50.1), ('completed_s', 0),
                    ('db_cpu_ms_per_completed', float('nan')))
        for field, value in failures:
            rows = cohort_rows()
            for row in rows:
                if row['wait_ns'] == 40_000_000:
                    row['db_cpu_ms_per_completed'] = .9
            changed = next(row for row in rows if row['wait_ns'] == 40_000_000)
            changed[field] = value
            result = rules.choose_wait(rows)
            with self.subTest(field=field):
                self.assertEqual(result['selected_wait_ms'], 20)
                self.assertFalse(result['ranking'][1]['qualified'])
                self.assertTrue(result['ranking'][1]['reasons'])
                json.dumps(result, allow_nan=False)

    def test_coverage_headroom_and_slo_boundaries_remain_inclusive(self):
        rows = cohort_rows()
        for row in rows:
            row.update(db_cpu_coverage=.95, host_coverage=.9, host_busy_percent=85,
                       host_steal_percent=5, p99_ms=5000, drain_s=3,
                       max_generator_lag_ms=50)
            for client in row['client_process_cpu']:
                client.update(coverage=.9, cpu_cores=3.1999)
            if row['wait_ns'] == 40_000_000:
                row['db_cpu_ms_per_completed'] = .9
        self.assertEqual(rules.choose_wait(rows)['selected_wait_ms'], 40)

    def test_missing_or_failed_anchor_is_inconclusive_not_a_longer_wait_vote(self):
        for change in ('missing', 'failed', 'audit-failed'):
            rows = cohort_rows()
            for row in rows:
                if row['wait_ns'] > 20_000_000:
                    row['db_cpu_ms_per_completed'] = .5
            if change == 'missing':
                rows.pop(0)
            elif change == 'failed':
                rows[0]['verified'] = False
            else:
                rows[0]['original_go_audit']['exit_code'] = 1
            result = rules.choose_wait(rows)
            with self.subTest(change=change):
                self.assertEqual(result['outcome'], 'inconclusive')
                self.assertIsNone(result['selected_wait_ms'])
                self.assertFalse(result['anchor_qualified'])

    def test_missing_candidate_is_preserved_and_does_not_erase_anchor(self):
        rows = [row for row in cohort_rows() if row['wait_ns'] != 80_000_000]
        result = rules.choose_wait(rows)
        self.assertEqual(result['selected_wait_ms'], 20)
        missing = result['ranking'][2]
        self.assertFalse(missing['qualified'])
        self.assertEqual(len(missing['reasons']), 6)
        self.assertTrue(all(reason.endswith('missing') for reason in missing['reasons']))

    def test_duplicate_unregistered_identity_and_wrong_controls_are_rejected(self):
        changes = (('phase', 'confirmation'), ('case_id', 'other'), ('path', 'direct'),
                   ('profile', 'two-step'), ('rate', 513), ('workers', 1024),
                   ('database_cpus', .25), ('wait_ns', 10_000_000), ('round', True))
        for field, value in changes:
            rows = cohort_rows()
            rows[0][field] = value
            with self.subTest(field=field), self.assertRaises(ValueError):
                rules.choose_wait(rows)
        rows = cohort_rows()
        rows[0]['controls']['max_bytes'] = 8388608
        with self.assertRaises(ValueError):
            rules.choose_wait(rows)
        rows = cohort_rows()
        rows.append(copy.deepcopy(rows[0]))
        with self.assertRaises(ValueError):
            rules.choose_wait(rows)

    def test_overflowing_comparison_is_a_reportable_failure_not_non_json_output(self):
        rows = cohort_rows()
        for row in rows:
            row['db_cpu_ms_per_completed'] = 1e-308 if row['wait_ns'] == 20_000_000 else 1e308
        result = rules.choose_wait(rows)
        self.assertEqual(result['selected_wait_ms'], 20)
        self.assertFalse(result['ranking'][1]['eligible'])
        self.assertIsNone(result['ranking'][1]['comparisons'][0]['cpu_per_completed_unit_ratio'])
        json.dumps(result, allow_nan=False)


if __name__ == '__main__':
    unittest.main()
