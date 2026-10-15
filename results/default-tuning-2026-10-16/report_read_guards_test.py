import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).with_name('report_read_guards.py')
spec = importlib.util.spec_from_file_location('read_guard_report', SCRIPT)
reporter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reporter)


def fixture():
    rows = []
    for backend in ('search', 'mongo'):
        for workers in (4, 128):
            for rd in (1, 2, 3):
                for variant in ('default', 'legacy'):
                    for path in ('weir', 'direct'):
                        row = dict(label='confirm-' + backend + '-read', phase='confirmation', kind='write',
                                   case_set='confirm-' + backend + '-read', case_id=variant + '-' + backend + '-read-q1',
                                   path=path, workers=workers, round=rd, usable=True, qualification_errors=[],
                                   error=None, completed_s=100, p99_ms=10)
                        rows.append(row)
    result = dict(schema=1, complete_inventory=True, rows=rows)
    return result


class ReadGuardTests(unittest.TestCase):
    def test_reports_every_backend_worker_and_round_without_averaging(self):
        report = reporter.build_report(fixture())
        self.assertTrue(report['passed'])
        self.assertEqual(len(report['guards']), 12)
        self.assertEqual({(guard['backend'], guard['workers'], guard['round']) for guard in report['guards']},
                         {(backend, workers, rd) for backend in ('search', 'mongo') for workers in (4, 128) for rd in (1, 2, 3)})
        self.assertTrue(all(guard['values']['normalized_qps_ratio'] == 1 for guard in report['guards']))

    def test_native_normalization_can_fail_even_when_default_raw_qps_is_higher(self):
        analysis = fixture()
        analysis['rows'][0]['completed_s'] = 150
        analysis['rows'][1]['completed_s'] = 200
        report = reporter.build_report(analysis)
        self.assertFalse(report['passed'])
        guard = report['guards'][0]
        self.assertEqual(guard['values']['normalized_qps_ratio'], .75)
        self.assertFalse(guard['throughput_passed'])
        self.assertTrue(guard['latency_passed'])
        self.assertEqual(sum(not item['passed'] for item in report['guards']), 1)
        analysis = fixture()
        analysis['rows'][0]['completed_s'] = 80
        analysis['rows'][3]['completed_s'] = 200
        report = reporter.build_report(analysis)
        self.assertTrue(report['passed'])
        self.assertEqual(report['guards'][0]['values']['normalized_qps_ratio'], 1.6)

    def test_both_latency_branches_and_inclusive_thresholds(self):
        for legacy, limit in ((10, 12.5), (1, 3)):
            analysis = fixture()
            analysis['rows'][0]['completed_s'] = 90
            analysis['rows'][0]['p99_ms'] = limit
            analysis['rows'][2]['p99_ms'] = legacy
            report = reporter.build_report(analysis)
            with self.subTest(legacy=legacy):
                self.assertTrue(report['passed'])
                self.assertEqual(report['guards'][0]['values']['p99_limit_ms'], limit)
                analysis['rows'][0]['p99_ms'] = limit + .001
                failed = reporter.build_report(analysis)
                self.assertFalse(failed['passed'])
                self.assertTrue(failed['guards'][0]['throughput_passed'])
                self.assertFalse(failed['guards'][0]['latency_passed'])

    def test_missing_duplicate_and_each_unusable_role_fail_without_dropping_guards(self):
        for mutation in ('missing', 'duplicate', 'unusable-default', 'unusable-default-native', 'unusable-legacy', 'unusable-legacy-native'):
            analysis = fixture()
            if mutation == 'missing':
                analysis['rows'].pop(0)
            elif mutation == 'duplicate':
                analysis['rows'].append(copy.deepcopy(analysis['rows'][0]))
            else:
                index = ('unusable-default', 'unusable-default-native', 'unusable-legacy', 'unusable-legacy-native').index(mutation)
                analysis['rows'][index].update(usable=False, qualification_errors=['original observation failed'])
            report = reporter.build_report(analysis)
            with self.subTest(mutation=mutation):
                self.assertFalse(report['passed'])
                self.assertEqual(len(report['guards']), 12)
                self.assertFalse(report['guards'][0]['passed'])
                self.assertTrue(report['guards'][0]['errors'])
                self.assertIsNone(report['guards'][0]['throughput_passed'])

    def test_invalid_numbers_and_incomplete_inventory_do_not_pass(self):
        for value in (0, -1, None, True, float('nan'), float('inf')):
            analysis = fixture()
            analysis['rows'][0]['completed_s'] = value
            report = reporter.build_report(analysis)
            with self.subTest(value=value):
                self.assertFalse(report['passed'])
                json.dumps(report, allow_nan=False)
        analysis = fixture()
        analysis['complete_inventory'] = False
        report = reporter.build_report(analysis)
        self.assertFalse(report['passed'])
        self.assertTrue(report['input_errors'])

    def test_cli_retains_negative_report_and_exits_unsuccessfully(self):
        analysis = fixture()
        analysis['rows'][0]['completed_s'] = 89
        with tempfile.TemporaryDirectory(prefix='weir-read-guard-test-') as tmp:
            source, target = Path(tmp) / 'analysis.json', Path(tmp) / 'guards.json'
            source.write_text(json.dumps(analysis))
            command = [sys.executable, str(SCRIPT), '--analysis', str(source), '--output', str(target)]
            result = subprocess.run(command, capture_output=True, text=True, check=False)
            self.assertEqual(result.returncode, 1)
            report = json.loads(target.read_text())
            self.assertFalse(report['passed'])
            self.assertEqual(len(report['guards']), 12)
            self.assertEqual(len(report['analysis_sha256']), 64)
            self.assertEqual(report['guards'][0]['measurements']['default_weir']['observations'][0]['completed_s'], 89)


if __name__ == '__main__':
    unittest.main()
