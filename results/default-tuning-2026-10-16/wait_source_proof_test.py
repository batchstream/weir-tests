"""Bound the wait study to its registered source, workload and one-shot inventory."""
import copy
import importlib.util
import json
from pathlib import Path
import sys
import unittest
from unittest import mock

FOLDER = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('wait_source_analysis_under_test', FOLDER / 'analyze.py')
analysis = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = analysis
spec.loader.exec_module(analysis)


class ExtendedSourceTests(unittest.TestCase):
    def test_frozen_admission_is_only_the_six_registered_go_files(self):
        expected, _ = analysis.prior.source_snapshot(analysis.REPO, analysis.EXTENDED_SOURCE)
        self.assertEqual(analysis.extended_source_proof(), expected)

    def test_admission_rejects_other_go_module_or_regression_test_changes(self):
        prep = analysis.soak_derivation.PREPARATION_SOURCE
        before = analysis.prior.source_snapshot(analysis.REPO, prep)
        tree, after = analysis.prior.source_snapshot(analysis.REPO, analysis.EXTENDED_SOURCE)
        for path in ('internal/benchmark/single_saturation.go', 'go.mod', 'cmd/weir-lab/main_test.go'):
            corrupted = copy.deepcopy(after)
            next(item for item in corrupted if item['path'] == path)['sha256'] = '0' * 64
            with self.subTest(path=path), mock.patch.object(analysis.prior, 'source_snapshot', side_effect=[before, (tree, corrupted)]), self.assertRaises(ValueError):
                analysis.extended_source_proof()

    def test_frozen_tree_cannot_hide_changed_measurement_or_embedded_bytes(self):
        prep = analysis.soak_derivation.PREPARATION_SOURCE
        raw = []
        for path in ('cmd/weir-lab/main.go', 'internal/benchmark/client_worker.go', 'internal/workload/dataset.go'):
            raw.extend([analysis.committed(analysis.REPO, prep, path),
                        analysis.committed(analysis.REPO, analysis.EXTENDED_SOURCE, path)])
        for path in analysis.EMBEDDED_INPUTS:
            raw.extend([analysis.committed(analysis.REPO, prep, path),
                        analysis.committed(analysis.REPO, analysis.EXTENDED_SOURCE, path)])
        for index in (1, 3, 5, 7, 9, 11, 13):
            corrupted = list(raw)
            corrupted[index] += b'\n'
            with self.subTest(index=index), mock.patch.object(analysis, 'committed', side_effect=corrupted), self.assertRaises(ValueError):
                analysis.extended_source_proof()


class WaitPhaseTests(unittest.TestCase):
    def test_same_rate_wait_cases_never_feed_formal_capacity_soak_or_comparisons(self):
        entry = dict(label='wait-partial-q1', phase='wait_sensitivity', case_set='wait-partial-q1')
        rows = []
        for wait in (20, 40, 80, 160):
            row = dict(label=entry['label'], case_id='wait-partial-q1-w' + str(wait) + 'ms',
                       kind='crawler', path='optimized', rate=512, workers=512, round=1)
            rows.append(row)
        manifest = dict(entries=[entry])
        options = dict(manifest=manifest, rows=rows)
        with mock.patch.object(analysis, 'committed', side_effect=AssertionError('wait phase entered formal case processing')):
            formal = analysis.formal_results(options)
        expected = dict(cpu_capacity=[], crawler_rate_bounds=[], sustained=[], comparisons=[])
        self.assertEqual(formal, expected)

    def test_original_22_workflows_keep_all_567_formal_decisions(self):
        manifest = json.loads((FOLDER / 'manifest.json').read_bytes())
        original = json.loads((FOLDER / 'analysis-startup-corrected-confirmation/analysis.json').read_bytes())
        original_labels = {run['label'] for run in original['runs']}
        entries = [entry for entry in manifest['entries'] if entry['label'] in original_labels]
        self.assertEqual(len(entries), 22)
        self.assertEqual(len(original['rows']), 567)
        filtered = dict(entries=entries)
        options = dict(manifest=filtered, rows=original['rows'])
        self.assertEqual(analysis.formal_results(options), original['formal'])


class WaitPlanSourceTests(unittest.TestCase):
    def setUp(self):
        self.raw = (analysis.REPO / analysis.WAIT_PLAN).read_bytes()
        self.plan = json.loads(self.raw)
        self.entry = dict(label='wait-partial-q1', phase='wait_sensitivity', case_set='wait-partial-q1',
                          plan_path=analysis.WAIT_PLAN, plan_sha256=analysis.checksum(self.raw),
                          client_source=analysis.EXTENDED_SOURCE, server_source=analysis.WAIT_SERVER)

    def test_actual_generated_plan_matches_both_exact_registered_cohorts(self):
        self.assertEqual(analysis.checksum(self.raw), '3ddd9f8bda45a694da942bd6db81017d108cd2e2eb5cfe92c44b9c593c04a8f3')
        for name in analysis.WAIT_CASE_SETS:
            entry = dict(self.entry, label=name, case_set=name)
            analysis.validate_wait_plan(entry, self.plan)

    def test_wait_plan_rejects_changed_registration_workload_controls_order_and_types(self):
        mutations = ('document-source', 'document-hash', 'selector-source', 'selector-hash',
                     'parent-source', 'parent-hash', 'client-reference', 'metadata-extra',
                     'inventory', 'count', 'bytes', 'slots', 'wait', 'omitted', 'paths',
                     'rate', 'visibility', 'workers', 'processes', 'duration', 'warmup',
                     'rounds', 'quota', 'numeric-bool', 'order', 'extra-cohort')
        for mutation in mutations:
            plan = copy.deepcopy(self.plan)
            metadata = plan['metadata']
            case = plan['case_sets']['wait-partial-q1'][0]
            if mutation in ('document-source', 'document-hash', 'selector-source', 'selector-hash', 'parent-source', 'parent-hash'):
                prefix, suffix = mutation.split('-')
                prefix = {'document': 'wait_sensitivity_supplement', 'selector': 'wait_selector', 'parent': 'parent_confirmation'}[prefix]
                suffix = 'source' if suffix == 'source' else 'sha256'
                metadata[prefix + '_' + suffix] = '0' * (40 if suffix == 'source' else 64)
            elif mutation == 'client-reference':
                metadata['client_go_reference'] = analysis.soak_derivation.ORIGINAL_SOURCE
            elif mutation == 'metadata-extra':
                metadata['retry_failed'] = True
            elif mutation == 'inventory':
                metadata['inventory']['wait-partial-q1']['timed_stages'] = 13
            elif mutation in ('count', 'bytes', 'slots', 'wait'):
                field, value = {'count': ('max_operations', 64), 'bytes': ('max_bytes', 8388608),
                                'slots': ('max_in_flight', 0), 'wait': ('max_wait', '40ms')}[mutation]
                case['batching'][field] = case['expected_batching'][field] = value
            elif mutation == 'omitted':
                case['batching'] = None
            elif mutation == 'paths':
                case['paths'] = ['direct', 'optimized']
            elif mutation in ('rate', 'visibility', 'workers', 'processes', 'duration', 'warmup', 'rounds', 'quota', 'numeric-bool'):
                field, value = {'rate': ('rate', 513), 'visibility': ('visibility', 'query_visible'),
                                'workers': ('concurrency', 1024), 'processes': ('client_processes', 8),
                                'duration': ('duration', '61s'), 'warmup': ('warmup', '61s'),
                                'rounds': ('rounds', 1), 'quota': ('database_cpus', .5),
                                'numeric-bool': ('database_cpus', True)}[mutation]
                case[field] = value
            elif mutation == 'order':
                plan['case_sets']['wait-partial-q1'].reverse()
            else:
                plan['case_sets']['unregistered-wait'] = []
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                analysis.validate_wait_plan(self.entry, plan)

    def test_cext_tree_is_admitted_only_once_for_each_exact_wait_cohort(self):
        prep_tree, _ = analysis.prior.source_snapshot(analysis.REPO, analysis.soak_derivation.PREPARATION_SOURCE)
        extended_tree, _ = analysis.prior.source_snapshot(analysis.REPO, analysis.EXTENDED_SOURCE)
        references = []
        registration = analysis.WAIT_REGISTRATION
        references.append(analysis.committed(analysis.REPO, registration['parent_confirmation_source'], registration['parent_confirmation_plan']))
        for name in ('wait_sensitivity_supplement', 'wait_selector'):
            references.append(analysis.committed(analysis.REPO, registration[name + '_source'], registration[name]))
        for mutation in ('valid', 'old-cohort', 'unregistered-wait', 'old-source', 'prep-source', 'wrong-phase', 'duplicate', 'different-tree'):
            entry = dict(self.entry)
            entries = [entry]
            observed_tree = extended_tree
            if mutation == 'valid':
                crawler = dict(entry, label='wait-crawler-q1', case_set='wait-crawler-q1')
                entries.append(crawler)
            elif mutation == 'old-cohort':
                entry.update(phase='confirmation', case_set='confirm-partial-q1')
            elif mutation == 'unregistered-wait':
                entry['case_set'] = 'wait-partial-retry'
            elif mutation == 'old-source':
                entry['client_source'] = analysis.soak_derivation.ORIGINAL_SOURCE
            elif mutation == 'prep-source':
                entry['client_source'] = analysis.soak_derivation.PREPARATION_SOURCE
            elif mutation == 'wrong-phase':
                entry['phase'] = 'confirmation'
            elif mutation == 'duplicate':
                duplicate = dict(entry, label='another-wait-attempt')
                entries.append(duplicate)
            elif mutation == 'different-tree':
                observed_tree = '0' * 64
            inputs = [self.raw] * len(entries) + references * len(entries)
            manifest = dict(entries=entries)
            with self.subTest(mutation=mutation), mock.patch.object(analysis, 'preparation_source_proof', return_value=prep_tree), mock.patch.object(analysis, 'extended_source_proof', return_value=observed_tree), mock.patch.object(analysis, 'committed', side_effect=inputs):
                if mutation == 'valid':
                    analysis.measurement_source_proof(manifest)
                else:
                    with self.assertRaises(ValueError):
                        analysis.measurement_source_proof(manifest)

    def test_fixed_parent_document_and_selector_bytes_are_independently_checked(self):
        registration = analysis.WAIT_REGISTRATION
        raw = [analysis.committed(analysis.REPO, registration['parent_confirmation_source'], registration['parent_confirmation_plan'])]
        for name in ('wait_sensitivity_supplement', 'wait_selector'):
            raw.append(analysis.committed(analysis.REPO, registration[name + '_source'], registration[name]))
        for index in range(3):
            corrupted = list(raw)
            corrupted[index] += b'\n'
            with self.subTest(index=index), mock.patch.object(analysis, 'committed', side_effect=corrupted), self.assertRaises(ValueError):
                analysis.validate_wait_plan(self.entry, self.plan)

    def test_wait_entry_cannot_hide_embedded_changes_behind_the_same_go_tree(self):
        prep_tree, _ = analysis.prior.source_snapshot(analysis.REPO, analysis.soak_derivation.PREPARATION_SOURCE)
        extended_tree, _ = analysis.prior.source_snapshot(analysis.REPO, analysis.EXTENDED_SOURCE)
        analysis.prior.source_snapshot(analysis.REPO, analysis.soak_derivation.ORIGINAL_SOURCE)
        embedded = []
        for path in analysis.EMBEDDED_INPUTS:
            embedded.extend([analysis.committed(analysis.REPO, analysis.soak_derivation.ORIGINAL_SOURCE, path),
                             analysis.committed(analysis.REPO, analysis.EXTENDED_SOURCE, path)])
        embedded[3] += b'changed crawler business program'
        manifest = dict(entries=[self.entry])
        with mock.patch.object(analysis, 'preparation_source_proof', return_value=prep_tree), mock.patch.object(analysis, 'extended_source_proof', return_value=extended_tree), mock.patch.object(analysis, 'committed', return_value=self.raw), mock.patch.object(analysis.subprocess, 'check_output', side_effect=embedded), self.assertRaises(ValueError):
            analysis.measurement_source_proof(manifest)


if __name__ == '__main__':
    unittest.main()
