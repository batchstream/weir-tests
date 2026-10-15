import unittest

from run_crawler import workload_controls


class CrawlerControlsReceiptTest(unittest.TestCase):
    def test_mechanism_units_oracle_and_phase_scopes_are_explicit(self):
        environ = dict(CRAWLER_PROFILES='partial-update', CRAWLER_LEVELS='512', CRAWLER_PAGE_SIZE='1', CRAWLER_BATCH_MAX_WAIT='20ms')
        controls = workload_controls(environ)
        self.assertEqual(controls['logical_unit'], 'single_partial_update')
        self.assertEqual(controls['seeded_documents'], 3073)
        self.assertEqual(controls['native_conflict_retries'], 0)
        self.assertEqual(controls['weir_backend_expression_conflict_retries'], 0)
        self.assertIsNone(controls['weir_lua_total_attempts'])
        self.assertEqual(controls['current_batch_max_wait'], '0s')
        self.assertEqual(controls['optimized_batch_max_wait'], '20ms')
        self.assertIn('refresh=wait_for', controls['search_phase_metrics']['http_duration'])
        self.assertIn('not acknowledged writes', controls['search_phase_metrics']['request_records'])
        self.assertIn('Evaluate only', controls['search_phase_metrics']['lua_duration'])

    def test_crawler_controls_preserve_existing_retry_and_completion_policy(self):
        environ = dict(CRAWLER_PROFILES='two-step', CRAWLER_LEVELS='128,512', CRAWLER_PAGE_SIZE='1')
        controls = workload_controls(environ)
        self.assertEqual(controls['logical_unit'], 'completed_business_product')
        self.assertEqual(controls['seeded_documents'], 3073)
        self.assertEqual(controls['native_conflict_retries'], 3)
        self.assertEqual(controls['weir_lua_total_attempts'], 4)
        self.assertIsNone(controls['weir_backend_expression_conflict_retries'])
        self.assertEqual(controls['optimized_batch_max_wait'], '0s')
        self.assertEqual(controls['visibility'], 'query_visible')
        self.assertEqual(controls['seed_padding_bytes'], 0)
        self.assertEqual(controls['current_batch_max_operations'], 32)
        self.assertEqual(controls['current_batch_max_bytes'], 8 << 20)

    def test_partial_candidate_controls_and_completion_are_recorded_before_launch(self):
        environ = dict(CRAWLER_PROFILES='partial-update', CRAWLER_LEVELS='512', CRAWLER_PAGE_SIZE='1', CRAWLER_VISIBILITY='application_acknowledged', CRAWLER_SEED_PADDING_BYTES='16384', CRAWLER_BATCH_MAX_OPERATIONS='128', CRAWLER_BATCH_MAX_BYTES='1048576', CRAWLER_MAX_IN_FLIGHT='4', CRAWLER_BATCH_MAX_WAIT='20ms')
        controls = workload_controls(environ)
        self.assertEqual(controls['visibility'], 'application_acknowledged')
        self.assertEqual(controls['seed_padding_bytes'], 16384)
        self.assertEqual(controls['optimized_batch_max_operations'], 128)
        self.assertEqual(controls['optimized_batch_max_bytes'], 1 << 20)
        self.assertEqual(controls['optimized_batch_max_in_flight'], 4)
        self.assertEqual(controls['optimized_batch_max_wait'], '20ms')
        self.assertEqual(controls['current_batch_max_in_flight'], 0)
        self.assertIn('attempts', controls['search_phase_metrics']['request_body_bytes'])


if __name__ == '__main__':
    unittest.main()
