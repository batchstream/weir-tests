import importlib.util
from pathlib import Path
import unittest


spec = importlib.util.spec_from_file_location('confirmation_generator', Path(__file__).with_name('generate_confirmation_plan.py'))
generator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generator)


class ConfirmationInventoryTest(unittest.TestCase):
    def setUp(self):
        self.repository = Path(__file__).resolve().parents[2]
        self.controls = dict(max_operations=128, max_bytes=1048576, max_wait='5ms', max_in_flight=16)
        self.plan = generator.build_plan(self.repository, 'a' * 40, self.controls)

    def test_exact_inventory_and_workflow_budgets(self):
        expected = {'confirm-pure-q025': 18, 'confirm-pure-q1': 18,
                    'confirm-partial-q025': 36, 'confirm-crawler-q025': 30,
                    'confirm-partial-q1': 6, 'confirm-crawler-q1': 6,
                    'confirm-search-read': 24, 'confirm-mongo-read': 24,
                    'confirm-mongo-regression': 36, 'confirm-search-large-body': 12}
        inventory = self.plan['metadata']['inventory']
        self.assertEqual({key: value['timed_stages'] for key, value in inventory.items()}, expected)
        self.assertEqual(sum(entry['timed_stages'] for entry in inventory.values()), 210)
        self.assertTrue(all(entry['warmup_and_timed_minutes'] <= 100 for entry in inventory.values()))
        self.assertEqual(max(entry['warmup_and_timed_minutes'] for entry in inventory.values()), 72)

    def test_soak_derivation_is_registered_without_changing_confirmation_inventory(self):
        expected = dict(version=1, max_checks=3,
                        pure_case_sets=['confirm-pure-q025', 'confirm-pure-q1'],
                        pure_order=['workers_ascending', 'database_cpus_ascending', 'label_ascending', 'case_id_ascending'],
                        crawler_order=['lower_rate_descending', 'database_cpus_ascending', 'label_ascending'],
                        crawler_profiles=['partial-update', 'two-step'], warmup='60s', duration='300s',
                        rounds=1, client_processes=4, rerun_failed_soak=False)
        self.assertEqual(self.plan['metadata']['soak_derivation'], expected)
        self.assertEqual(len(self.plan['case_sets']), 10)

    def test_defaults_omitted_and_legacy_zeros_explicit(self):
        for cases in self.plan['case_sets'].values():
            for case in cases:
                self.assertEqual(case['client_processes'], 4)
                self.assertEqual(case['rounds'], 3)
                if case['id'].startswith('default-'):
                    self.assertIsNone(case['batching'])
                    self.assertEqual(case['expected_batching'], self.controls)
                else:
                    self.assertEqual(case['batching'], dict(max_operations=32, max_bytes=8388608, max_wait='0s', max_in_flight=0))
                    self.assertNotIn('expected_batching', case)

    def test_rates_visibility_sizes_and_mongo_mix_match_method(self):
        sets = self.plan['case_sets']
        self.assertEqual([case['rate'] for case in sets['confirm-partial-q025']], [128, 256, 512, 1024, 2048, 4096])
        self.assertEqual([case['rate'] for case in sets['confirm-crawler-q025']], [34, 68, 136, 272, 544])
        for cases in sets.values():
            for case in cases:
                if case['kind'] == 'crawler':
                    self.assertEqual(case['paths'], ['direct', 'optimized'])
                    self.assertEqual(case['concurrency'], 512)
                    self.assertEqual(case['warmup'], '60s')
                    self.assertEqual(case['duration'], '60s')
                    self.assertEqual(case['seed_padding_bytes'], 16384 if case['profile'] == 'partial-update' else 0)
                    self.assertEqual(case['visibility'], 'application_acknowledged' if case['profile'] == 'partial-update' else 'query_visible')
                else:
                    large = 'large-body' in case['id']
                    self.assertEqual(case['records'], 2048 if large else 8192)
                    self.assertEqual(case['payload_bytes'], 65536 if large else 1024)
        mongo = sets['confirm-mongo-regression']
        self.assertEqual([(case['write_percent'], case['lua_mutations']) for case in mongo],
                         [(100, False), (100, False), (10, False), (10, False), (100, True), (100, True)])
        large = sets['confirm-search-large-body']
        self.assertEqual([case['levels'] for case in large], [[512], [512]])
        self.assertTrue(all(case['backend'] == 'search' and case['database_cpus'] == 1
                            and case['warmup'] == '60s' and case['duration'] == '60s' for case in large))
        self.assertEqual(self.plan['metadata']['confirmation_addendum_source'],
                         '778a3b1ef3d48a9b1bb1e6e10bdae4d8edbce61e')
        self.assertEqual(len(self.plan['metadata']['confirmation_addendum_sha256']), 64)

    def test_invalid_selection_cannot_generate_a_formal_plan(self):
        self.controls['max_in_flight'] = 0
        with self.assertRaises(ValueError):
            generator.build_plan(self.repository, 'a' * 40, self.controls)
        self.controls['max_in_flight'] = 16
        self.controls['max_operations'] = 17
        with self.assertRaises(ValueError):
            generator.build_plan(self.repository, 'a' * 40, self.controls)


if __name__ == '__main__':
    unittest.main()
