import copy
import gzip
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('batch_coverage_analysis', Path(__file__).with_name('analyze.py'))
analysis = importlib.util.module_from_spec(spec)
spec.loader.exec_module(analysis)


class CoverageAnalysisTest(unittest.TestCase):
    def rows(self, kind='crawler'):
        rows = []
        for workers in ((128, 512) if kind == 'write' else (512,)):
            for round_number in (1, 2, 3):
                for path, cpu in (('direct', 10), ('weir' if kind == 'write' else 'optimized', 8)):
                    rows.append(dict(label='formal-test', role='formal', kind=kind, workers=workers, path=path, round=round_number, formal_measurement=True, formal_all_arrivals=kind == 'crawler', completed_s=100, db_cpu_ms_per_completed=cpu, db_cpu_quota_percent=80 if path == 'direct' else 64, sustained_database_cpu_saturation=False))
        return rows

    def compare(self, rows, kind='crawler'):
        entries = [dict(label='formal-test', role='formal', kind=kind)]
        return next(item for item in analysis.comparisons(rows, entries) if item['candidate'] == ('weir' if kind == 'write' else 'optimized') and item['baseline'] == 'direct')

    def test_every_round_needs_qualification_qps_and_lower_cpu(self):
        self.assertTrue(self.compare(self.rows())['supports_lower_cpu_at_comparable_completed_qps'])
        for field, value in (('formal_all_arrivals', False), ('completed_s', 94), ('db_cpu_ms_per_completed', 10), ('db_cpu_ms_per_completed', None)):
            rows = self.rows()
            rows[-1][field] = value
            self.assertFalse(self.compare(rows)['supports_lower_cpu_at_comparable_completed_qps'], field)
        self.assertFalse(self.compare(self.rows()[:-1])['supports_lower_cpu_at_comparable_completed_qps'])

    def test_empty_formal_inventory_is_preserved_without_positive_claims(self):
        comparison = self.compare([])
        self.assertEqual(len(comparison['paired_rounds']), 3)
        self.assertTrue(all(pair['qps_ratio'] is None and not pair['both_qualified'] for pair in comparison['paired_rounds']))
        self.assertFalse(comparison['supports_lower_cpu_at_comparable_completed_qps'])

    def test_write_qps_increase_with_more_actual_cpu_is_not_equal_load(self):
        rows = self.rows('write')
        for row in rows:
            if row['path'] == 'weir':
                row['completed_s'] = 150
                row['db_cpu_quota_percent'] = 90
        compared = self.compare(rows, 'write')
        self.assertTrue(all(pair['qps_ratio'] == 1.5 for pair in compared['paired_rounds']))
        self.assertFalse(compared['higher_write_qps_at_no_more_measured_db_cpu'])
        self.assertFalse(compared['supports_equal_load_capacity_increase'])

    def test_capacity_requires_pressure_and_plateau_in_each_round(self):
        rows = self.rows('write')
        for row in rows:
            row['sustained_database_cpu_saturation'] = True
        self.assertTrue(analysis.capacity_gate(rows, 'formal-test', 'direct'))
        selected = next(row for row in rows if row['path'] == 'direct' and row['workers'] == 512 and row['round'] == 3)
        selected['completed_s'] = 111
        self.assertFalse(analysis.capacity_gate(rows, 'formal-test', 'direct'))
        selected['completed_s'] = 100
        rows[0]['sustained_database_cpu_saturation'] = False
        self.assertFalse(analysis.capacity_gate(rows, 'formal-test', 'direct'))

    def test_formal_gates_keep_rejections_lag_and_missing_cpu_negative(self):
        row = dict(role='formal', kind='crawler', run_checks_passed=True, stage_checks_passed=True, verified=True, failed=0, unknown=0, timeout=0, db_cpu_coverage=.8, rejected=0, offered=100, completed=100, p99_ms=5000, drain_s=3, max_generator_lag_ms=50)
        analysis.formal_gate(row)
        self.assertTrue(row['formal_all_arrivals'])
        for field, value in (('rejected', 1), ('max_generator_lag_ms', 50.001), ('db_cpu_coverage', .799), ('run_checks_passed', False), ('completed', 99)):
            changed = dict(row)
            changed[field] = value
            analysis.formal_gate(changed)
            self.assertFalse(changed['formal_all_arrivals'], field)

    def test_cpu_counter_intervals_are_clipped_and_missing_is_unavailable(self):
        samples = [dict(elapsed_ns=-10, database_cpu_time_ns=0, database_cpu_counter_clock_unix_ns=100), dict(elapsed_ns=50, database_cpu_time_ns=30, database_cpu_counter_clock_unix_ns=160), dict(elapsed_ns=110, database_cpu_time_ns=60, database_cpu_counter_clock_unix_ns=220)]
        resources = dict(database_allocated_cpus=1, samples=samples, valid_cpu_intervals=2, valid_cpu_sampling_elapsed_ns=100, measurement_elapsed_ns=100, mean_database_cpu_budget_percent=50, fraction_intervals_at_cpu_threshold=0, cpu_sampling_time_coverage=1, sustained_cpu_saturation=False)
        observed = analysis.cpu_observation(resources, 100, 10)
        self.assertEqual(observed['db_cpu_ns'], 50)
        self.assertEqual(observed['db_cpu_coverage'], 1)
        self.assertEqual(observed['first_half_cpu_percent'], 50)
        self.assertEqual(observed['second_half_cpu_percent'], 50)
        resources.update(samples=[], valid_cpu_intervals=0, valid_cpu_sampling_elapsed_ns=0, mean_database_cpu_budget_percent=0, cpu_sampling_time_coverage=0)
        observed = analysis.cpu_observation(resources, 100, 10)
        self.assertIsNone(observed['db_cpu_ns'])
        self.assertIsNone(observed['db_cpu_ms_per_completed'])

    def test_nested_archive_hashes_and_path_traversal(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            nested = folder/'artifacts/tmp/fixture/report.json.gz'
            nested.parent.mkdir(parents=True)
            raw = b'{"verified":true}\n'
            compressed = gzip.compress(raw, mtime=0)
            nested.write_bytes(compressed)
            item = dict(path=nested.relative_to(folder).as_posix(), gzip_sha256=analysis.digest(compressed), sha256=analysis.digest(raw), bytes=len(raw))
            (folder/'files.json').write_text(json.dumps([item]))
            self.assertEqual(analysis.verify_archive(folder), 1)
            item['sha256'] = '0'*64
            (folder/'files.json').write_text(json.dumps([item]))
            with self.assertRaises(ValueError):
                analysis.verify_archive(folder)
            item['path'] = '../report.json.gz'
            (folder/'files.json').write_text(json.dumps([item]))
            with self.assertRaises(ValueError):
                analysis.verify_archive(folder)

    def test_diagnostic_requires_both_actual_attempts_and_body_bytes(self):
        stage = dict(Path='optimized', HTTPDelta={'POST _bulk': dict(Requests=4, RequestBytes=1000)})
        metrics = dict(search_bulk_attempts=4, search_bulk_body_bytes=1000)
        self.assertTrue(analysis.diagnostic_counts(stage, metrics)['usable'])
        metrics['search_bulk_body_bytes'] = 999
        self.assertFalse(analysis.diagnostic_counts(stage, metrics)['usable'])
        metrics['search_bulk_body_bytes'] = None
        self.assertFalse(analysis.diagnostic_counts(stage, metrics)['usable'])

    def write_result(self):
        children = []
        counts = dict(attempted=1, succeeded=1, errors=0, indeterminate=0, not_attempted=0, applied_with_error=0, business_operation_timeouts=0, logical_request_document_bytes=1024, logical_response_document_bytes=0)
        buckets = [0, 1]+[0]*254
        latency = analysis.helpers.histogram(buckets, 1000)
        for index in range(4):
            child = dict(phase='timed', pid=index+1, concurrent_workers=1, worker_offset=index, native_connection_pool_limit=1, owned_process_exited=True, result=dict(counts, latency=latency), business_requests=1, writes=1, reads=0, latency_histogram_buckets=buckets, expected_record_ranges=[dict(start=index*2048, revisions=[0]*2048)])
            children.append(child)
        result = {key: value*4 for key, value in counts.items()}
        result.update(path='direct', planned=4, client_requests=4, writes=4, reads=0, client_processes=children, latency=analysis.helpers.histogram([0, 4]+[0]*254, 1000))
        return result

    def test_write_postflight_requires_every_seeded_record_and_real_process_exit(self):
        result = self.write_result()
        analysis.write_clients(result, 4)
        omitted = copy.deepcopy(result)
        omitted['client_processes'][-1]['expected_record_ranges'][0]['revisions'].pop()
        with self.assertRaises(ValueError):
            analysis.write_clients(omitted, 4)
        overlap = copy.deepcopy(result)
        overlap['client_processes'][1]['expected_record_ranges'][0]['start'] = 0
        with self.assertRaises(ValueError):
            analysis.write_clients(overlap, 4)
        alive = copy.deepcopy(result)
        alive['client_processes'][0]['owned_process_exited'] = False
        with self.assertRaises(ValueError):
            analysis.write_clients(alive, 4)

    def test_live_candidate_cap_is_sixteen_and_both_boundaries_must_match(self):
        configuration = dict(stores=[dict(name='search', batching=dict(max_operations=128, max_bytes='8388608B', max_in_flight=16, max_wait='5ms'))])
        snapshot = dict(raw='weir_store_batch_max_wait_seconds{store="search"} 0.005\nweir_store_active_executions_limit{store="search"} 16\n')
        analysis.batching_proof(configuration, (snapshot, snapshot), analysis.CASES['bounded128'])
        wrong = dict(raw=snapshot['raw'].replace(' 16', ' 0'))
        with self.assertRaises(ValueError):
            analysis.batching_proof(configuration, (snapshot, wrong), analysis.CASES['bounded128'])

    def test_manifest_is_six_workflows_and_sixty_second_formal_controls(self):
        manifest = analysis.load(Path(__file__).with_name('manifest.json'))
        analysis.validate_manifest(manifest)
        changed = copy.deepcopy(manifest)
        next(entry for entry in changed['entries'] if entry['role'] == 'formal')['duration_ns'] = 20*10**9
        with self.assertRaises(ValueError):
            analysis.validate_manifest(changed)


if __name__ == '__main__':
    unittest.main()
