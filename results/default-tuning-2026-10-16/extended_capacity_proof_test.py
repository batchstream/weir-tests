"""Counterexamples for the one registered extended-concurrency confirmation."""
import ast
import copy
import importlib.util
import json
from pathlib import Path
import sys
import unittest
from unittest import mock

FOLDER = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('extended_capacity_analysis_test', FOLDER / 'analyze.py')
analysis = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = analysis
spec.loader.exec_module(analysis)
generator = analysis.soak_derivation


def extended_fixture():
    raw = (analysis.REPO / generator.EXTENDED_PLAN).read_bytes()
    plan = json.loads(raw)
    parent = dict(label=generator.EXTENDED_PARENT, phase='confirmation', case_set=generator.EXTENDED_PARENT,
                  client_source=analysis.EXTENDED_SOURCE, server_source=generator.STARTUP_SERVER,
                  plan_path=generator.EXTENDED_PLAN, plan_sha256=analysis.checksum(raw))
    rows = []
    for path in ('direct', 'weir'):
        for workers in (1024, 2048):
            for rd in (1, 2, 3):
                clients = [dict(pid=pid, coverage=1, cpu_cores=.1) for pid in (1, 2, 3, 4)]
                row = dict(label=parent['label'], phase='confirmation', case_set=parent['case_set'],
                           case_id=generator.EXTENDED_CASE_ID, kind='write', profile=None,
                           database_cpus=.125, path=path, workers=workers, round=rd,
                           usable=True, checks_passed=True, verified=True, completed=180000,
                           completed_s=1000, failed=0, unknown=0, timeout=0, db_cpu_coverage=1,
                           sustained_database_cpu_saturation=True, db_cpu_quota_percent=99,
                           fraction_measurement_at_90_percent_cpu=1, cpu_intervals=[{} for _ in range(5)],
                           host_headroom=True, host_coverage=1, host_busy_percent=40, host_steal_percent=0,
                           effective_host_cpus=4, client_headroom=True, client_process_cpu=clients,
                           rate_stable=True, p99_ms=1, db_cpu_ms_per_completed=.12375)
                rows.append(row)
    manifest = dict(entries=[parent])
    options = dict(manifest=manifest, rows=rows)
    with mock.patch.object(analysis, 'committed', return_value=raw):
        formal = analysis.formal_results(options)
    selection = dict(selected=copy.deepcopy(generator.STARTUP_CONTROLS))
    byte_selection = dict(selected=generator.STARTUP_CONTROLS['max_bytes'])
    run = dict(label=parent['label'], available=True)
    actual = dict(complete_inventory=True, runs=[run], selection=selection, byte_selection=byte_selection, formal=formal)
    plans = {parent['label']: plan}
    options = dict(analysis=actual, manifest=manifest, plans=plans,
                   controls=generator.STARTUP_CONTROLS, server_source=generator.STARTUP_SERVER)
    record = generator.build_plan(options)
    result = dict(parent=parent, parent_raw=raw, parent_plan=plan, rows=rows, record=record,
                  run=run, selection=selection, byte_selection=byte_selection, derivation=options)
    return result


class ExtendedCapacityTests(unittest.TestCase):
    def test_actual_plan_has_only_the_fixed_20ms_12_stage_confirmation(self):
        inputs = extended_fixture()
        self.assertEqual(analysis.checksum(inputs['parent_raw']), '89bd5d403fead5814315428eb184f85797f40c10e0c7d8373a8129f06f271fb8')
        entries = {inputs['parent']['label']: inputs['parent']}
        options = dict(entry=inputs['parent'], plan=inputs['parent_plan'], entries=entries,
                       plans=inputs['derivation']['plans'])
        generator.validate_extended_plan(options)
        self.assertEqual(analysis.runner.validate_case(inputs['parent_plan']['case_sets'][generator.EXTENDED_PARENT][0]), 12)
        soak = inputs['record']['case_sets']['soak-pure'][0]
        self.assertEqual((soak['levels'], soak['warmup'], soak['duration'], soak['rounds']), ([1024], '60s', '300s', 1))
        self.assertEqual(inputs['record']['metadata']['checks'], 1)

    def test_confirmation_rejects_changed_registration_shape_source_and_controls(self):
        for mutation in ('document-source', 'document-hash', 'reference', 'server', 'controls',
                         'explicit', 'warmup', 'duration', 'levels', 'rounds', 'workers',
                         'payload', 'records', 'quota', 'numeric-bool', 'inventory', 'extra-cohort', 'path'):
            inputs = extended_fixture()
            entry, plan = inputs['parent'], inputs['parent_plan']
            case = plan['case_sets'][generator.EXTENDED_PARENT][0]
            if mutation == 'document-source':
                plan['metadata']['extended_concurrency_supplement_source'] = generator.PREPARATION_SOURCE
            elif mutation == 'document-hash':
                plan['metadata']['extended_concurrency_supplement_sha256'] = '0' * 64
            elif mutation == 'reference':
                plan['metadata']['client_go_reference'] = generator.PREPARATION_SOURCE
            elif mutation == 'server':
                plan['server_source'] = entry['server_source'] = '0' * 40
            elif mutation == 'controls':
                plan['metadata']['selected_controls']['max_wait'] = '40ms'
            elif mutation == 'explicit':
                case['batching'] = dict(generator.STARTUP_CONTROLS)
            elif mutation in ('warmup', 'duration', 'levels', 'rounds', 'workers', 'payload', 'records', 'quota', 'numeric-bool'):
                field, value = {'warmup': ('warmup', '60s'), 'duration': ('duration', '60s'),
                                'levels': ('levels', [1024, 2048, 4096]), 'rounds': ('rounds', 1),
                                'workers': ('client_processes', 8), 'payload': ('payload_bytes', 65536),
                                'records': ('records', 2048), 'quota': ('database_cpus', .25),
                                'numeric-bool': ('client_processes', True)}[mutation]
                case[field] = value
            elif mutation == 'inventory':
                plan['metadata']['inventory'][generator.EXTENDED_PARENT]['timed_stages'] = 13
            elif mutation == 'extra-cohort':
                plan['case_sets']['another-attempt'] = []
            else:
                entry['plan_path'] = 'plans/another-extended-attempt.json'
            entries, plans = {entry['label']: entry}, {entry['label']: plan}
            options = dict(entry=entry, plan=plan, entries=entries, plans=plans)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                generator.validate_extended_plan(options)

    def test_document_bytes_and_original_numerical_rules_remain_fixed(self):
        inputs = extended_fixture()
        entries = {inputs['parent']['label']: inputs['parent']}
        options = dict(entry=inputs['parent'], plan=inputs['parent_plan'], entries=entries,
                       plans=inputs['derivation']['plans'])
        with mock.patch.object(generator.subprocess, 'check_output', return_value=b'changed method'), self.assertRaises(ValueError):
            generator.validate_extended_plan(options)
        source = 'd619907dbad944c59995939dd0f3ccd5d731906f'
        for name in ('capacity_rules.py', 'selection_rules.py'):
            self.assertEqual((FOLDER / name).read_bytes(), analysis.committed(analysis.REPO, source, 'results/default-tuning-2026-10-16/' + name))
        original = analysis.committed(analysis.REPO, source, 'results/default-tuning-2026-10-16/derive_soak_plan.py').decode()
        segments = []
        for text in (original, (FOLDER / 'derive_soak_plan.py').read_text()):
            tree = ast.parse(text)
            node = next(node for node in tree.body if isinstance(node, ast.Assign) and any(isinstance(target, ast.Name) and target.id == 'POLICY' for target in node.targets))
            segments.append(ast.get_source_segment(text, node))
        self.assertEqual(*segments)

    def test_cext_go_tree_is_available_only_to_the_registered_parent_and_once(self):
        inputs = extended_fixture()
        prep_tree, _ = analysis.prior.source_snapshot(analysis.REPO, generator.PREPARATION_SOURCE)
        extended_tree, _ = analysis.prior.source_snapshot(analysis.REPO, analysis.EXTENDED_SOURCE)
        for mutation in ('valid', 'c0', 'cprep', 'old-cohort', 'wrong-phase', 'duplicate', 'different-tree'):
            entry = dict(inputs['parent'])
            observed_tree = extended_tree
            entries = [entry]
            if mutation == 'c0':
                entry['client_source'] = generator.ORIGINAL_SOURCE
            elif mutation == 'cprep':
                entry['client_source'] = generator.PREPARATION_SOURCE
            elif mutation == 'old-cohort':
                entry['case_set'] = 'confirm-pure-q025'
            elif mutation == 'wrong-phase':
                entry['phase'] = 'wait_sensitivity'
            elif mutation == 'duplicate':
                duplicate = dict(entry, label='another-extended-attempt')
                entries.append(duplicate)
            elif mutation == 'different-tree':
                observed_tree = '0' * 64
            manifest = dict(entries=entries)
            with self.subTest(mutation=mutation), mock.patch.object(analysis, 'preparation_source_proof', return_value=prep_tree), mock.patch.object(analysis, 'extended_source_proof', return_value=observed_tree), mock.patch.object(analysis, 'committed', return_value=inputs['parent_raw']):
                if mutation == 'valid':
                    analysis.measurement_source_proof(manifest)
                else:
                    with self.assertRaises(ValueError):
                        analysis.measurement_source_proof(manifest)

    def test_only_the_actual_selected_1024_parent_can_propagate_to_one_pure_soak(self):
        for mutation in ('valid', 'parent-label', 'parent-case', 'parent-source', 'parent-plan',
                         'extra-proof', 'unselected', 'workers', 'duration', 'warmup', 'rounds', 'explicit', 'control', 'registration'):
            inputs = extended_fixture()
            plan = inputs['record']
            proof = plan['metadata']['extended_concurrency_parent']
            case = plan['case_sets']['soak-pure'][0]
            if mutation in ('parent-label', 'parent-case', 'parent-source', 'parent-plan'):
                key, value = {'parent-label': ('label', 'confirm-pure-q025'),
                              'parent-case': ('case_id', 'default-pure-q025'),
                              'parent-source': ('client_source', generator.ORIGINAL_SOURCE),
                              'parent-plan': ('plan_sha256', '0' * 64)}[mutation]
                proof[key] = value
            elif mutation == 'extra-proof':
                proof['decisions'] = [True, True, True]
            elif mutation == 'unselected':
                plan['metadata']['qualified_and_skipped'][0]['selected'] = False
            elif mutation in ('workers', 'duration', 'warmup', 'rounds'):
                key, value = {'workers': ('levels', [2048]), 'duration': ('duration', '180s'),
                              'warmup': ('warmup', '300s'), 'rounds': ('rounds', 3)}[mutation]
                case[key] = value
            elif mutation == 'explicit':
                case['batching'] = dict(generator.STARTUP_CONTROLS)
            elif mutation == 'control':
                case['expected_batching']['max_wait'] = '40ms'
            elif mutation == 'registration':
                plan['metadata']['client_go_reference'] = generator.PREPARATION_SOURCE
            entry = dict(label='soak-pure', phase='soak', case_set='soak-pure', server_source=generator.STARTUP_SERVER)
            entries = {inputs['parent']['label']: inputs['parent']}
            options = dict(entry=entry, plan=plan, entries=entries, plans=inputs['derivation']['plans'])
            with self.subTest(mutation=mutation):
                if mutation == 'valid':
                    generator.validate_extended_plan(options)
                else:
                    with self.assertRaises(ValueError):
                        generator.validate_extended_plan(options)

    def test_last_point_and_failed_round_cannot_create_a_soak(self):
        for mutation in ('last-point', 'failed-round', 'duplicate'):
            inputs = extended_fixture()
            options = inputs['derivation']
            if mutation == 'duplicate':
                entry = dict(inputs['parent'], label='another-extended-attempt')
                options['manifest']['entries'].append(entry)
                run = dict(label=entry['label'], available=True)
                options['analysis']['runs'].append(run)
                options['plans'][entry['label']] = copy.deepcopy(inputs['parent_plan'])
            else:
                point = options['analysis']['formal']['cpu_capacity'][0]['decision']['paths'][1]['points'][0]
                if mutation == 'last-point':
                    point['workers'] = 2048
                else:
                    point['decisions'][0]['passed'] = False
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                generator.build_plan(options)

    def test_conditional_soak_source_is_bound_to_its_parent_and_global_one_pure_budget(self):
        inputs = extended_fixture()
        raw = json.dumps(inputs['record']).encode()
        prep_tree, _ = analysis.prior.source_snapshot(analysis.REPO, generator.PREPARATION_SOURCE)
        extended_tree, _ = analysis.prior.source_snapshot(analysis.REPO, analysis.EXTENDED_SOURCE)
        for mutation in ('valid', 'cprep', 'wrong-cohort', 'duplicate-parent', 'duplicate-soak', 'second-c0-soak'):
            parent = dict(inputs['parent'])
            soak = dict(label='soak-pure', phase='soak', case_set='soak-pure',
                        client_source=analysis.EXTENDED_SOURCE, server_source=generator.STARTUP_SERVER,
                        plan_path='plans/derived-extended-soak.json', plan_sha256=analysis.checksum(raw))
            entries, contents = [parent, soak], [inputs['parent_raw'], raw]
            if mutation == 'cprep':
                soak['client_source'] = generator.PREPARATION_SOURCE
            elif mutation == 'wrong-cohort':
                soak['case_set'] = 'soak-crawler'
            elif mutation == 'duplicate-parent':
                duplicate = dict(parent, label='another-parent-attempt')
                entries.append(duplicate)
                contents.append(inputs['parent_raw'])
            elif mutation in ('duplicate-soak', 'second-c0-soak'):
                duplicate = dict(soak, label='another-pure-soak')
                if mutation == 'second-c0-soak':
                    duplicate['client_source'] = generator.ORIGINAL_SOURCE
                entries.append(duplicate)
                contents.append(raw)
            manifest = dict(entries=entries)
            with self.subTest(mutation=mutation), mock.patch.object(analysis, 'preparation_source_proof', return_value=prep_tree), mock.patch.object(analysis, 'extended_source_proof', return_value=extended_tree), mock.patch.object(analysis, 'committed', side_effect=contents):
                if mutation == 'valid':
                    analysis.measurement_source_proof(manifest)
                else:
                    with self.assertRaises(ValueError):
                        analysis.measurement_source_proof(manifest)

    def test_soak_eligibility_is_recomputed_from_real_parent_rows_with_metadata_unchanged(self):
        for mutation in ('valid', 'point-unstable', 'following-unstable', 'cpu', 'next-gain'):
            inputs = extended_fixture()
            workers = 2048 if mutation in ('following-unstable', 'next-gain') else 1024
            row = next(row for row in inputs['rows'] if row['path'] == 'weir' and row['workers'] == workers and row['round'] == 1)
            if mutation in ('point-unstable', 'following-unstable'):
                row['rate_stable'] = False
            elif mutation == 'cpu':
                row['sustained_database_cpu_saturation'] = False
            elif mutation == 'next-gain':
                row['completed_s'] = 1101
            entries = {inputs['parent']['label']: inputs['parent']}
            soak = dict(label='soak-pure', phase='soak', case_set='soak-pure',
                        client_source=analysis.EXTENDED_SOURCE, server_source=generator.STARTUP_SERVER,
                        plan_path='plans/derived-extended-soak.json')
            preflight = dict(entry=soak, plan=inputs['record'], entries=entries, plans=inputs['derivation']['plans'])
            generator.validate_extended_plan(preflight)
            raw = json.dumps(inputs['record']).encode()
            soak['plan_sha256'] = analysis.checksum(raw)
            manifest = dict(entries=[inputs['parent'], soak])
            soak_run = dict(label=soak['label'], available=True)
            options = dict(manifest=manifest, rows=inputs['rows'], runs=[inputs['run'], soak_run],
                           selection=inputs['selection'], byte_selection=inputs['byte_selection'])
            with self.subTest(mutation=mutation), mock.patch.object(analysis, 'committed', side_effect=[raw, inputs['parent_raw'], inputs['parent_raw']]):
                if mutation == 'valid':
                    analysis.validate_registered_soak_derivation(options)
                else:
                    with self.assertRaises(ValueError):
                        analysis.validate_registered_soak_derivation(options)

    def test_original_smaller_worker_tie_wins_even_when_extended_metadata_claims_selection(self):
        inputs = extended_fixture()
        source = '682e725df1a81e29f4d4983d8711df1989c4adad'
        path = 'plans/default-confirmation-2026-10-16.json'
        old_raw = analysis.committed(analysis.REPO, source, path)
        old_plan = json.loads(old_raw)
        old_case = old_plan['case_sets']['confirm-pure-q025'][0]
        old = dict(label='confirm-pure-q025', phase='confirmation', case_set='confirm-pure-q025',
                   client_source=source, server_source=generator.STARTUP_SERVER,
                   plan_path=path, plan_sha256=analysis.checksum(old_raw))
        rows = list(inputs['rows'])
        for path_name in ('direct', 'weir'):
            for workers in old_case['levels']:
                for rd in (1, 2, 3):
                    rate = 500 if workers == 128 else 1000
                    row = dict(inputs['rows'][0], label=old['label'], case_set=old['case_set'],
                               case_id=old_case['id'], database_cpus=.25, path=path_name,
                               workers=workers, round=rd, completed_s=rate, completed=rate*60,
                               db_cpu_ms_per_completed=247.5/rate)
                    rows.append(row)
        raw = json.dumps(inputs['record']).encode()
        soak = dict(label='soak-pure', phase='soak', case_set='soak-pure',
                    client_source=analysis.EXTENDED_SOURCE, server_source=generator.STARTUP_SERVER,
                    plan_path='plans/derived-extended-soak.json', plan_sha256=analysis.checksum(raw))
        manifest = dict(entries=[inputs['parent'], old, soak])
        runs = [inputs['run'], dict(label=old['label'], available=True), dict(label=soak['label'], available=True)]
        options = dict(manifest=manifest, rows=rows, runs=runs,
                       selection=inputs['selection'], byte_selection=inputs['byte_selection'])
        contents = [raw, inputs['parent_raw'], old_raw, inputs['parent_raw'], old_raw]
        with mock.patch.object(analysis, 'committed', side_effect=contents), self.assertRaisesRegex(ValueError, 'another pure point wins'):
            analysis.validate_registered_soak_derivation(options)

    def test_ordinary_parent_does_not_acquire_extended_warmup_admission(self):
        source = '682e725df1a81e29f4d4983d8711df1989c4adad'
        raw = analysis.committed(analysis.REPO, source, 'plans/default-confirmation-2026-10-16.json')
        plan = json.loads(raw)
        entry = dict(label='confirm-pure-q025', phase='confirmation', case_set='confirm-pure-q025')
        case = plan['case_sets'][entry['case_set']][0]
        entries, plans = {entry['label']: entry}, {entry['label']: plan}
        options = dict(entries=entries, plans=plans, controls=generator.STARTUP_CONTROLS)
        self.assertIs(generator.parent_case(options, entry['label'], case['id']), case)
        case['warmup'], case['duration'] = '300s', '180s'
        with self.assertRaises(ValueError):
            generator.parent_case(options, entry['label'], case['id'])

    def test_old_24_source_and_591_rows_keep_formal_choice_and_zero_soak_unchanged(self):
        manifest = json.loads((FOLDER / 'manifest-before-extended-capacity.json').read_bytes())
        original = json.loads((FOLDER / 'analysis-wait-sensitivity/analysis.json').read_bytes())
        self.assertEqual(len(manifest['entries']), 24)
        self.assertEqual(len(original['rows']), 591)
        analysis.measurement_source_proof(manifest)
        options = dict(manifest=manifest, rows=original['rows'])
        self.assertEqual(analysis.formal_results(options), original['formal'])
        selection_plan = json.loads(analysis.committed(analysis.REPO, manifest['selection_client'], manifest['selection_plan']))
        selection_rows = [row for row in original['rows'] if row['phase'] == 'selection']
        selection_options = dict(rows=selection_rows, plan=selection_plan)
        self.assertEqual(analysis.rules.choose(selection_options), original['selection'])
        self.assertEqual(analysis.rules.choose_bytes(selection_rows), original['byte_selection'])
        selector = analysis.module('extended_test_wait_selector', FOLDER / 'wait_sensitivity_rules.py')
        choice = json.loads((FOLDER / 'wait-sensitivity-choice.json').read_bytes())
        choice.pop('analysis_sha256')
        choice.pop('selector_sha256')
        self.assertEqual(selector.choose_wait(original['rows']), choice)
        plans = {}
        for entry in manifest['entries']:
            if entry['phase'] == 'confirmation':
                plans[entry['label']] = json.loads(analysis.committed(analysis.REPO, entry['client_source'], entry['plan_path']))
        options = dict(analysis=original, manifest=manifest, plans=plans,
                       controls=generator.STARTUP_CONTROLS, server_source=generator.STARTUP_SERVER)
        derived = generator.build_plan(options)
        empty = {}
        self.assertEqual(derived['case_sets'], empty)
        self.assertEqual(derived['metadata']['checks'], 0)
        self.assertNotIn('extended_concurrency_parent', derived['metadata'])
        frozen = json.loads((FOLDER / 'soak-derivation-after-startup-correction.json').read_bytes())
        frozen['metadata'].pop('input_sha256')
        derived['metadata'].pop('input_sha256')
        self.assertEqual(derived, frozen)


if __name__ == '__main__':
    unittest.main()
