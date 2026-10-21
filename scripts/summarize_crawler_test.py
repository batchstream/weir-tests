import unittest

from summarize_crawler import REPORT_SCHEMA, rows, search_metric_deltas, search_metric_sum


class SearchPhaseMetricsTest(unittest.TestCase):
    def test_unavailable_and_reset_stay_null(self):
        before = dict(metrics=[dict(name='weir_search_http_requests_total', labels=dict(operation='bulk'), value=4)])
        after = dict(metrics=[dict(name='weir_search_http_requests_total', labels=dict(operation='bulk'), value=2)])
        deltas = search_metric_deltas(dict(ServerBefore=before, ServerAfter=after))
        self.assertIsNone(deltas[0]['delta'])
        self.assertIsNone(search_metric_sum(deltas, 'weir_search_http_requests_total', dict(operation='bulk')))
        self.assertIsNone(search_metric_deltas(dict(ServerBefore=dict(error='unavailable'), ServerAfter=after)))
        deltas = search_metric_deltas(dict(ServerBefore={}, ServerAfter=after))
        self.assertIsNone(deltas[0]['delta'])
        self.assertIsNone(search_metric_sum(deltas, 'weir_search_http_requests_total', dict(operation='mget')))

    def test_labeled_actual_record_buckets_are_not_adapter_batches(self):
        labels = dict(operation='bulk', visibility='wait_for', le='4')
        before = dict(metrics=[dict(name='weir_search_http_request_records_bucket', labels=labels, value=3), dict(name='weir_store_batch_operations_sum', value=100)])
        after = dict(metrics=[dict(name='weir_search_http_request_records_bucket', labels=labels, value=8), dict(name='weir_store_batch_operations_sum', value=1000)])
        deltas = search_metric_deltas(dict(ServerBefore=before, ServerAfter=after))
        self.assertEqual(deltas, [dict(name='weir_search_http_request_records_bucket', labels=labels, delta=5)])

    def test_attempted_http_body_bytes_are_retained_without_inventing_missing_values(self):
        name = 'weir_search_http_request_body_bytes_sum'
        labels = dict(operation='bulk', visibility='none')
        before = dict(metrics=[dict(name=name, labels=labels, value=100)])
        after = dict(metrics=[dict(name=name, labels=labels, value=612)])
        deltas = search_metric_deltas(dict(ServerBefore=before, ServerAfter=after))
        self.assertEqual(search_metric_sum(deltas, name, dict(operation='bulk')), 512)
        self.assertIsNone(search_metric_sum(deltas, name, dict(operation='mget')))

    def test_counters_sum_observed_outcomes_and_preserve_missing_series(self):
        name = 'weir_search_http_requests_total'
        metrics = [dict(name=name, labels=dict(operation='bulk', outcome='success', visibility='wait_for'), value=2), dict(name=name, labels=dict(operation='bulk', outcome='http_error', visibility='wait_for'), value=1)]
        after = [dict(metric, value=metric['value'] + 3) for metric in metrics]
        deltas = search_metric_deltas(dict(ServerBefore=dict(metrics=metrics), ServerAfter=dict(metrics=after)))
        self.assertEqual(search_metric_sum(deltas, name, dict(operation='bulk')), 6)
        deltas[0]['delta'] = None
        self.assertIsNone(search_metric_sum(deltas, name, dict(operation='bulk')))


class ReportSchemaTest(unittest.TestCase):
    def test_only_the_current_schema_is_summarized(self):
        for schema in (None, REPORT_SCHEMA - 1, REPORT_SCHEMA + 1):
            with self.subTest(schema=schema):
                report = dict(schema=schema, Parameters={}, Stages=[])
                with self.assertRaisesRegex(ValueError, 'schema'):
                    rows(report)

    def test_current_schema_requires_recorded_parameters(self):
        parameters = dict(InputShape='single', HTTPObserver='disabled', Modes=['saturation'], Rates=None,
                          Concurrency=[4], Paths=['direct', 'before', 'after'], Profiles=['two-step'], Rounds=1)
        report = dict(schema=REPORT_SCHEMA, Parameters=parameters, Stages=[])
        self.assertFalse(rows(report))
        for field in ('InputShape', 'HTTPObserver', 'Modes', 'Paths'):
            with self.subTest(missing=field):
                missing = dict(parameters)
                del missing[field]
                incomplete = dict(report, Parameters=missing)
                with self.assertRaises(KeyError):
                    rows(incomplete)


if __name__ == '__main__':
    unittest.main()
