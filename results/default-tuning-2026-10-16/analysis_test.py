"""Independent counterexamples for the prospectively frozen selection rules."""
import copy
import gzip
import importlib.util
import json
from pathlib import Path
import sys
import unittest

FOLDER = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('tuning_analysis_under_test', FOLDER / 'analyze.py')
analysis = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = analysis
spec.loader.exec_module(analysis)
rules = analysis.rules


def fixture():
    controls = dict(max_operations=128, max_bytes=8 << 20, max_wait='5ms', max_in_flight=16)
    cases = [dict(id='a', batching=controls), dict(id='b', batching=dict(controls, max_operations=64)),
             dict(id=rules.LEGACY, batching=dict(controls, max_operations=32, max_wait='0s', max_in_flight=0))]
    rows = []
    for case in cases:
        for case_set in rules.WRITE_SETS + rules.READ_SETS:
            levels = (4, 128) if case_set in rules.READ_SETS else (512,)
            for workers in levels:
                for path in ('direct', 'weir'):
                    rows.append(dict(case_set=case_set, case_id=case['id'], path=path, workers=workers,
                                     round=1, usable=True, completed_s=100, db_cpu_ms_per_completed=2,
                                     p99_ms=1, wait_ns=analysis.runner.duration_ns(case['batching']['max_wait']),
                                     max_bytes=case['batching']['max_bytes'], controls=case['batching']))
    result = dict(rows=rows, plan=dict(case_sets={'select-pure': cases}))
    return result


class SelectionTests(unittest.TestCase):
    def test_tie_prefers_smaller_finite_count(self):
        outcome = rules.choose(fixture())
        self.assertEqual(outcome['selected_id'], 'b')
        self.assertFalse(next(r for r in outcome['ranking'] if r['id'] == rules.LEGACY)['eligible'])

    def test_one_fast_profile_cannot_hide_another_slow_one(self):
        inputs = fixture()
        for row in inputs['rows']:
            if row['path'] == 'weir' and row['case_id'] == 'a':
                row['completed_s'] = 200 if row['case_set'] == 'select-pure' else 50 if row['case_set'] == 'select-partial' else 100
        self.assertEqual(rules.choose(inputs)['selected_id'], 'b')

    def test_cpu_breaks_throughput_plateau(self):
        inputs = fixture()
        for row in inputs['rows']:
            if row['path'] == 'weir' and row['case_id'] == 'a' and row['case_set'] in rules.WRITE_SETS:
                row.update(completed_s=98, db_cpu_ms_per_completed=1)
        self.assertEqual(rules.choose(inputs)['selected_id'], 'a')

    def test_read_regression_and_missing_profile_disqualify(self):
        inputs = fixture()
        for row in inputs['rows']:
            if row['path'] == 'weir' and row['case_id'] == 'a' and row['case_set'] == 'select-search-read':
                row['completed_s'] = 89
            if row['path'] == 'weir' and row['case_id'] == 'b' and row['case_set'] == 'select-crawler':
                row['usable'] = False
        self.assertIsNone(rules.choose(inputs)['selected'])

    def test_duplicate_point_rejected(self):
        inputs = fixture()
        inputs['rows'].append(copy.deepcopy(inputs['rows'][1]))
        with self.assertRaises(ValueError):
            rules.choose(inputs)

    def test_bytes_plateau_and_binding_witness(self):
        rows = [dict(case_set='select-bytes', path='weir', usable=True, max_bytes=cap,
                     completed_s=qps, db_cpu_ms_per_completed=1, search_bulk_attempts=10,
                     http_body_buckets=[dict(labels={'le': str(256 << 10)}, delta=small)])
                for cap, qps, small in ((256 << 10, 98, 10), (1 << 20, 100, 7))]
        outcome = rules.choose_bytes(rows)
        self.assertEqual(outcome['selected'], 256 << 10)
        self.assertTrue(outcome['binding'][0]['binding_demonstrated'])
        self.assertFalse(outcome['binding'][1]['binding_demonstrated'])


class RawPreflightTests(unittest.TestCase):
    def test_real_pure_child_fields_and_completion_bins(self):
        raw = gzip.decompress((FOLDER / 'preflight/local/write-locked/search.json.gz').read_bytes())
        report = json.loads(raw)
        case = dict(client_processes=4, records=32)
        for pair in report['pairs']:
            for path in pair['order']:
                stage = pair[path]
                analysis.children_proof(dict(stage=stage, case=case, workers=pair['concurrency']))
                totals = dict(offered=stage['attempted'], rejected=0, completed=stage['succeeded'],
                              failed=stage['errors'] + stage['indeterminate'], unknown=stage['indeterminate'],
                              timed_out=stage['business_operation_timeouts'])
                merged = analysis.capacity_rules.merge_buckets(dict(pools=[child['time_buckets'] for child in stage['client_processes']],
                             duration=5_000_000_000, histogram=analysis.helpers.histogram, totals=totals, latency=stage['latency']))
                self.assertEqual(sum(bucket['completed'] for bucket in merged), stage['succeeded'])
                self.assertEqual(sum(bucket['offered'] for bucket in merged), stage['attempted'])
                corrupted = copy.deepcopy(stage)
                corrupted['client_processes'][0]['expected_record_ranges'][0]['revisions'][0] = 2
                with self.assertRaises(ValueError):
                    analysis.children_proof(dict(stage=corrupted, case=case, workers=pair['concurrency']))

    def test_real_crawler_completion_bins(self):
        raw = gzip.decompress((FOLDER / 'preflight/local/crawler.json.gz').read_bytes())
        for stage in json.loads(raw)['Stages']:
            totals = {key: stage[field] for key, field in dict(offered='Offered', rejected='Rejected', completed='Completed', failed='Failed', unknown='Indeterminate', timed_out='TimedOut').items()}
            merged = analysis.capacity_rules.merge_buckets(dict(pools=[worker['time_buckets'] for reply in stage['Clients'] for worker in reply['Workers']],
                         duration=5_000_000_000, histogram=analysis.helpers.histogram, totals=totals, latency=stage['ScheduledToVisible']))
            self.assertEqual(sum(bucket['completed'] for bucket in merged), stage['Completed'])

    def test_command_parser_retains_boolean_flags(self):
        self.assertEqual(analysis.command_flags(['tool', '-x', '1', '-flag', '-output', 'result']), {'-x': '1', '-flag': True, '-output': 'result'})
        with self.assertRaises(ValueError):
            analysis.command_flags(['tool', '-flag', '-flag'])


if __name__ == '__main__':
    unittest.main()
