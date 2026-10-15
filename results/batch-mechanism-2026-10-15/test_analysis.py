import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('batch_mechanism_analysis', Path(__file__).with_name('analyze.py'))
analysis = importlib.util.module_from_spec(spec)
spec.loader.exec_module(analysis)


class MechanismGatesTest(unittest.TestCase):
    def rows(self):
        values = []
        for round_number in (1, 2, 3):
            for path, cpu in (('direct', 10), ('current', 8), ('optimized', 7)):
                values.append(dict(label='formal-partial', role='formal', path=path, round=round_number, completed_products_s=100, sampled_db_cpu_ms_per_product=cpu, formal_all_arrivals=True, sustained_database_cpu_saturation=False))
        return values

    def test_strict_cpu_improvement_does_not_imply_actual_pressure(self):
        compared = analysis.compare(self.rows())
        self.assertTrue(all(item['supports_lower_database_cpu_at_comparable_completed_throughput'] for item in compared))
        self.assertFalse(any(item['supports_lower_database_cpu_under_demonstrated_native_pressure'] for item in compared))

    def test_one_round_cpu_qps_or_arrival_failure_blocks_claim(self):
        for field, value in (('sampled_db_cpu_ms_per_product', 9), ('completed_products_s', 94), ('formal_all_arrivals', False)):
            rows = self.rows()
            row = next(row for row in rows if row['path'] == 'optimized' and row['round'] == 3)
            row[field] = value
            compared = analysis.compare(rows)
            item = next(item for item in compared if item['baseline'] == 'current' and item['candidate'] == 'optimized')
            self.assertFalse(item['supports_lower_database_cpu_at_comparable_completed_throughput'])

    def test_missing_round_or_cpu_cannot_be_extrapolated(self):
        rows = self.rows()[:-1]
        compared = analysis.compare(rows)
        self.assertFalse(next(item for item in compared if item['candidate'] == 'optimized')['supports_lower_database_cpu_at_comparable_completed_throughput'])
        rows = self.rows()
        rows[-1]['sampled_db_cpu_ms_per_product'] = None
        self.assertFalse(any(item['supports_lower_database_cpu_at_comparable_completed_throughput'] for item in analysis.compare(rows) if item['candidate'] == 'optimized'))

    def test_actual_collection_wait_and_cap_are_attested_at_both_boundaries(self):
        for path, wait in (('direct', 0), ('current', 0), ('optimized', .02)):
            snapshot = dict(raw=f'weir_store_batch_max_wait_seconds{{store="search"}} {wait}\nweir_store_active_executions_limit{{store="search"}} 0\n')
            stage = dict(Path=path, ServerBefore=snapshot, ServerAfter=snapshot)
            analysis.configuration_metrics(stage)
            wrong = dict(raw='weir_store_batch_max_wait_seconds{store="search"} 1\nweir_store_active_executions_limit{store="search"} 0\n')
            stage['ServerAfter'] = wrong
            with self.assertRaises(ValueError):
                analysis.configuration_metrics(stage)

    def test_raw_and_parsed_histogram_metrics_must_match(self):
        raw = 'weir_search_http_request_records_bucket{operation="bulk",visibility="wait_for",le="+Inf"} 4\nweir_search_http_request_records_count{operation="bulk",visibility="wait_for"} 4\n'
        labels = dict(operation='bulk', visibility='wait_for')
        snapshot = dict(raw=raw, metrics=[dict(name='weir_search_http_request_records_bucket', labels=dict(labels, le='+Inf'), value=4), dict(name='weir_search_http_request_records_count', labels=labels, value=4)])
        analysis.phase_metric_checks(dict(ServerBefore=snapshot, ServerAfter=snapshot))
        bad = dict(snapshot, raw=raw.replace(' 4', ' 5'))
        with self.assertRaises(ValueError):
            analysis.phase_metric_checks(dict(ServerBefore=snapshot, ServerAfter=bad))


if __name__ == '__main__':
    unittest.main()
