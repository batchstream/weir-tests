import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import unittest

spec = importlib.util.spec_from_file_location('soak_generator_test', Path(__file__).with_name('derive_soak_plan.py'))
generator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generator)


def fixture():
    controls = dict(max_operations=128, max_bytes=1 << 20, max_wait='5ms', max_in_flight=16)
    source = 'b' * 40
    entries, plans, pure, crawler = [], {}, [], []
    rounds = [dict(round=rd, passed=True) for rd in (1, 2, 3)]
    common = dict(batching=None, expected_batching=controls, client_processes=4, rounds=3, warmup='60s', duration='60s')
    for tag, quota, workers in (('q025', .25, 512), ('q1', 1, 128)):
        label = 'confirm-pure-' + tag
        entry = dict(label=label, phase='confirmation', case_set=label, server_source=source)
        case = dict(common, id='pure-' + tag, kind='write', backend='search', payload_bytes=1024, records=8192, write_percent=100, lua_mutations=False, levels=[128, 512, 1024], database_cpus=quota)
        entries.append(entry)
        plans[label] = dict(server_source=source, metadata=dict(soak_derivation=copy.deepcopy(generator.POLICY), selected_controls=controls), case_sets={label: [case]})
        point = dict(workers=workers, capacity_qualified=True, decisions=copy.deepcopy(rounds))
        decision = dict(database_cpus=quota, paths=[dict(path='weir', capacity_qualified=True, selected_workers=workers, points=[point])])
        pure.append(dict(label=label, case_id=case['id'], decision=decision))
    for profile, tag, rate in (('partial-update', 'partial', 256), ('two-step', 'crawler', 68)):
        label = 'confirm-' + tag + '-q025'
        entry = dict(label=label, phase='confirmation', case_set=label, server_source=source)
        cases = []
        for offered in (rate, rate*2):
            case = dict(common, id=tag + '-r' + str(offered), kind='crawler', profile=profile, rate=offered, concurrency=512, paths=['direct', 'optimized'], visibility='application_acknowledged' if profile == 'partial-update' else 'query_visible', seed_padding_bytes=16384 if profile == 'partial-update' else 0, http_observer='disabled', page_size=1, database_cpus=.25)
            cases.append(case)
        entries.append(entry)
        plans[label] = dict(server_source=source, metadata=dict(soak_derivation=copy.deepcopy(generator.POLICY), selected_controls=controls), case_sets={label: cases})
        points = [dict(rate=rate, qualified=True, service_overload=False, rounds=copy.deepcopy(rounds)), dict(rate=rate*2, qualified=False, service_overload=True, rounds=[dict(round=rd, passed=False, service_overload=True, evidence_errors=[]) for rd in (1, 2, 3)])]
        bound = dict(path='optimized', soak_eligible=True, bounded=True, lower_rate=rate, upper_rate=rate*2, points=points)
        crawler.append(dict(label=label, decision=dict(database_cpus=.25, profile=profile, paths=[bound])))
    manifest = dict(entries=entries)
    analysis = dict(complete_inventory=True, runs=[dict(label=entry['label'], available=True) for entry in entries], selection=dict(selected=dict(controls, max_bytes=8 << 20)), byte_selection=dict(selected=controls['max_bytes']), formal=dict(cpu_capacity=pure, crawler_rate_bounds=crawler))
    result = dict(analysis=analysis, manifest=manifest, plans=plans, controls=controls, server_source=source)
    return result


def add_independent_parents(inputs):
    for tag in ('partial', 'crawler'):
        original_label = 'confirm-' + tag + '-q025'
        label = 'confirm-' + tag + '-independent-q025'
        plan = copy.deepcopy(inputs['plans'][original_label])
        cases = plan['case_sets'].pop(original_label)
        for case in cases:
            case['id'] = 'independent-' + case['id']
            case['rate'] *= 2
            case['paths'] = ['optimized']
        plan['case_sets'][label] = cases
        plan['metadata'].update(generator.CANDIDATE_CAPACITY_REGISTRATION)
        inputs['plans'][label] = plan
        entry = dict(label=label, phase='confirmation', case_set=label, server_source=inputs['server_source'])
        inputs['manifest']['entries'].append(entry)
        inputs['analysis']['runs'].append(dict(label=label, available=True))
        original = next(result for result in inputs['analysis']['formal']['crawler_rate_bounds'] if result['label'] == original_label)
        result = copy.deepcopy(original)
        result['label'] = label
        bound = result['decision']['paths'][0]
        bound['lower_rate'] *= 2
        bound['upper_rate'] *= 2
        for point in bound['points']:
            point['rate'] *= 2
        inputs['analysis']['formal']['crawler_rate_bounds'].append(result)


def add_pure_quota_parent(inputs):
    label = 'confirm-pure-q0125'
    plan = copy.deepcopy(inputs['plans']['confirm-pure-q025'])
    cases = plan['case_sets'].pop('confirm-pure-q025')
    cases[0].update(id='default-pure-q0125', database_cpus=.125, levels=[256, 512, 1024])
    plan['case_sets'][label] = cases
    plan['metadata'].update(generator.PURE_QUOTA_REGISTRATION)
    inputs['plans'][label] = plan
    entry = dict(label=label, phase='confirmation', case_set=label, server_source=inputs['server_source'])
    inputs['manifest']['entries'].append(entry)
    inputs['analysis']['runs'].append(dict(label=label, available=True))
    result = copy.deepcopy(inputs['analysis']['formal']['cpu_capacity'][0])
    result.update(label=label, case_id='default-pure-q0125')
    result['decision']['database_cpus'] = .125
    inputs['analysis']['formal']['cpu_capacity'].append(result)


def add_startup_parent(inputs):
    inputs['controls'].update(generator.STARTUP_CONTROLS)
    inputs['analysis']['selection']['selected'].update(generator.STARTUP_CONTROLS)
    inputs['analysis']['byte_selection']['selected'] = inputs['controls']['max_bytes']
    inputs['server_source'] = generator.STARTUP_SERVER
    for entry in inputs['manifest']['entries']:
        entry['server_source'] = generator.STARTUP_SERVER
    for plan in inputs['plans'].values():
        plan['server_source'] = generator.STARTUP_SERVER
    path = 'plans/default-pure-capacity-q0125-startup-fix-2026-10-16.json'
    raw = (Path(__file__).resolve().parents[2] / path).read_bytes()
    plan = json.loads(raw)
    label = generator.STARTUP_PARENT
    entry = dict(label=label, phase='confirmation', case_set=label, server_source=generator.STARTUP_SERVER,
                 client_source=generator.PREPARATION_SOURCE, plan_path=path, plan_sha256=generator.sha(raw))
    inputs['manifest']['entries'].append(entry)
    inputs['plans'][label] = plan
    inputs['analysis']['runs'].append(dict(label=label, available=True))
    result = copy.deepcopy(inputs['analysis']['formal']['cpu_capacity'][0])
    result.update(label=label, case_id=generator.PURE_PARENTS[label]['case_id'])
    result['decision']['database_cpus'] = .125
    result['decision']['paths'][0]['points'][0]['decisions'] = [dict(round=rd, passed=True, reasons=[]) for rd in (1, 2, 3)]
    inputs['analysis']['formal']['cpu_capacity'].append(result)


class SoakGeneratorTests(unittest.TestCase):
    def test_startup_correction_cannot_be_repeated_under_another_label(self):
        inputs = fixture()
        add_startup_parent(inputs)
        entry = copy.deepcopy(inputs['manifest']['entries'][-1])
        entry['label'] = 'another-startup-attempt'
        inputs['manifest']['entries'].append(entry)
        inputs['analysis']['runs'].append(dict(label=entry['label'], available=True))
        inputs['plans'][entry['label']] = copy.deepcopy(inputs['plans'][generator.STARTUP_PARENT])
        with self.assertRaises(ValueError):
            generator.build_plan(inputs)

    def test_startup_registration_propagates_only_from_the_selected_parent(self):
        inputs = fixture()
        add_startup_parent(inputs)
        plan = generator.build_plan(inputs)
        self.assertNotIn('startup_preparation_parent', plan['metadata'])
        self.assertFalse(any(key in plan['metadata'] for key in generator.STARTUP_REGISTRATION))
        inputs['analysis']['formal']['cpu_capacity'][1]['decision']['paths'][0]['points'][0]['workers'] = 512
        plan = generator.build_plan(inputs)
        proof = plan['metadata']['startup_preparation_parent']
        self.assertEqual(proof['case_set'], generator.STARTUP_PARENT)
        self.assertEqual(proof['workers'], 512)
        self.assertTrue(all(plan['metadata'][key] == value for key, value in generator.STARTUP_REGISTRATION.items()))
        entry = dict(label='soak-pure', phase='soak', case_set='soak-pure', server_source=generator.STARTUP_SERVER)
        entries = {entry['label']: entry for entry in inputs['manifest']['entries']}
        options = dict(entry=entry, plan=plan, entries=entries, plans=inputs['plans'])
        generator.validate_startup_plan(options)

    def test_startup_confirmation_requires_every_fixed_registration_field(self):
        for key in generator.STARTUP_REGISTRATION:
            for replacement in (None, 'forged'):
                inputs = fixture()
                add_startup_parent(inputs)
                metadata = inputs['plans'][generator.STARTUP_PARENT]['metadata']
                if replacement is None:
                    metadata.pop(key)
                else:
                    metadata[key] = replacement
                with self.subTest(key=key, replacement=replacement), self.assertRaises(ValueError):
                    generator.build_plan(inputs)

    def test_startup_confirmation_cannot_change_shape_or_use_last_point_for_soak(self):
        for mutation in ('quota', 'levels', 'explicit', 'last-point', 'failed-round'):
            inputs = fixture()
            add_startup_parent(inputs)
            case = inputs['plans'][generator.STARTUP_PARENT]['case_sets'][generator.STARTUP_PARENT][0]
            point = inputs['analysis']['formal']['cpu_capacity'][-1]['decision']['paths'][0]['points'][0]
            if mutation == 'quota':
                case['database_cpus'] = .2
            elif mutation == 'levels':
                case['levels'] = [128, 512, 1024]
            elif mutation == 'explicit':
                case['batching'] = dict(inputs['controls'])
            elif mutation == 'last-point':
                point['workers'] = 1024
            else:
                point['decisions'][0]['passed'] = False
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                generator.build_plan(inputs)

    def test_startup_soak_rejects_wrong_parent_registration_and_shape(self):
        for mutation in ('parent-label', 'parent-case', 'parent-source', 'parent-plan', 'extra-proof', 'unselected', 'registration', 'quota', 'workers', 'duration', 'rounds', 'cohort'):
            inputs = fixture()
            add_startup_parent(inputs)
            inputs['analysis']['formal']['cpu_capacity'][1]['decision']['paths'][0]['points'][0]['workers'] = 512
            plan = generator.build_plan(inputs)
            proof, case = plan['metadata']['startup_preparation_parent'], plan['case_sets']['soak-pure'][0]
            entry = dict(label='soak-pure', phase='soak', case_set='soak-pure', server_source=generator.STARTUP_SERVER)
            if mutation == 'parent-label':
                proof['label'] = 'confirm-pure-q025'
            elif mutation == 'parent-case':
                proof['case_id'] = 'default-pure-q0125'
            elif mutation == 'parent-source':
                proof['client_source'] = generator.ORIGINAL_SOURCE
            elif mutation == 'parent-plan':
                proof['plan_sha256'] = '0' * 64
            elif mutation == 'extra-proof':
                proof['decisions'] = [dict(round=rd, passed=True, reasons=[]) for rd in (1, 2, 3)]
            elif mutation == 'unselected':
                for point in plan['metadata']['qualified_and_skipped']:
                    point['selected'] = False
            elif mutation == 'registration':
                plan['metadata']['preparation_source'] = generator.ORIGINAL_SOURCE
            elif mutation == 'quota':
                case['database_cpus'] = .2
            elif mutation == 'workers':
                case['levels'] = [1024]
            elif mutation == 'duration':
                case['duration'] = '60s'
            elif mutation == 'rounds':
                case['rounds'] = 3
            else:
                entry['case_set'] = 'soak-crawler'
            entries = {entry['label']: entry for entry in inputs['manifest']['entries']}
            options = dict(entry=entry, plan=plan, entries=entries, plans=inputs['plans'])
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                generator.validate_startup_plan(options)

    def test_original_derivation_policy_is_unchanged(self):
        expected = dict(version=1, max_checks=3,
                        pure_case_sets=['confirm-pure-q025', 'confirm-pure-q1'],
                        pure_order=['workers_ascending', 'database_cpus_ascending', 'label_ascending', 'case_id_ascending'],
                        crawler_order=['lower_rate_descending', 'database_cpus_ascending', 'label_ascending'],
                        crawler_profiles=['partial-update', 'two-step'], warmup='60s', duration='300s',
                        rounds=1, client_processes=4, rerun_failed_soak=False)
        self.assertEqual(generator.POLICY, expected)

    def test_three_checks_use_actual_omitted_defaults_and_original_topology(self):
        inputs = fixture()
        plan = generator.build_plan(inputs)
        self.assertEqual(plan['metadata']['checks'], 3)
        for name, cases in plan['case_sets'].items():
            self.assertEqual(len(cases), 1)
            case = cases[0]
            self.assertIsNone(case['batching'])
            self.assertEqual(case['expected_batching'], inputs['controls'])
            self.assertEqual((case['warmup'], case['duration'], case['rounds'], case['client_processes']), ('60s', '300s', 1, 4))
            if name == 'soak-pure':
                self.assertEqual((case['levels'], case['database_cpus']), ([128], 1))
            else:
                self.assertEqual(case['paths'], ['optimized'])

    def test_equal_smallest_workers_tie_prefers_smaller_quota_and_retains_other_point(self):
        inputs = fixture()
        inputs['analysis']['formal']['cpu_capacity'][0]['decision']['paths'][0]['points'][0]['workers'] = 128
        plan = generator.build_plan(inputs)
        self.assertEqual(plan['case_sets']['soak-pure'][0]['database_cpus'], .25)
        retained = plan['metadata']['qualified_and_skipped']
        self.assertTrue(any(point.get('database_cpus') == 1 and point.get('workers') == 128 and point['selected'] is False for point in retained))

    def test_registered_pure_quota_uses_original_workers_then_quota_order(self):
        inputs = fixture()
        add_pure_quota_parent(inputs)
        plan = generator.build_plan(inputs)
        self.assertEqual(plan['case_sets']['soak-pure'][0]['database_cpus'], 1)
        self.assertEqual(plan['case_sets']['soak-pure'][0]['levels'], [128])
        inputs['analysis']['formal']['cpu_capacity'][1]['decision']['paths'][0]['points'][0]['workers'] = 512
        plan = generator.build_plan(inputs)
        self.assertEqual(plan['metadata']['checks'], 3)
        self.assertEqual(plan['case_sets']['soak-pure'][0]['database_cpus'], .125)
        self.assertEqual(plan['case_sets']['soak-pure'][0]['levels'], [512])
        retained = plan['metadata']['qualified_and_skipped']
        self.assertTrue(any(point.get('label') == 'confirm-pure-q025' and point.get('selected') is False for point in retained))

    def test_registered_pure_quota_requires_frozen_supplement_metadata(self):
        expected = dict(
            pure_capacity_quota_supplement='results/default-tuning-2026-10-16/PURE-CAPACITY-QUOTA-SUPPLEMENT.md',
            pure_capacity_quota_supplement_source='f50bddd0537866354b564e3607202fe3c460faaa',
            pure_capacity_quota_supplement_sha256='e52d8433f9939174987451470fa075d8419e01acad9a58ed7db65ab5eaff1e89')
        self.assertEqual(generator.PURE_QUOTA_REGISTRATION, expected)
        for key in expected:
            for replacement in (None, 'unregistered'):
                inputs = fixture()
                add_pure_quota_parent(inputs)
                metadata = inputs['plans']['confirm-pure-q0125']['metadata']
                if replacement is None:
                    metadata.pop(key)
                else:
                    metadata[key] = replacement
                with self.subTest(key=key, replacement=replacement), self.assertRaises(ValueError):
                    generator.build_plan(inputs)

    def test_prospective_pure_plan_matches_the_committed_supplement(self):
        repository = Path(__file__).resolve().parents[2]
        plan = json.loads((repository / 'plans/default-pure-capacity-q0125-2026-10-16.json').read_bytes())
        registration = generator.PURE_QUOTA_REGISTRATION
        raw = subprocess.check_output(['git', 'show', registration['pure_capacity_quota_supplement_source'] + ':' + registration['pure_capacity_quota_supplement']], cwd=repository)
        self.assertEqual(generator.sha(raw), registration['pure_capacity_quota_supplement_sha256'])
        self.assertTrue(all(plan['metadata'][key] == value for key, value in registration.items()))
        self.assertEqual(plan['metadata']['soak_derivation'], generator.POLICY)
        self.assertEqual(plan['metadata']['inventory']['confirm-pure-q0125']['timed_stages'], 18)
        self.assertEqual(plan['metadata']['inventory']['confirm-pure-q0125']['warmup_and_timed_minutes'], 36)
        controls = plan['metadata']['selected_controls']
        entry = dict(label='confirm-pure-q0125', phase='confirmation', case_set='confirm-pure-q0125', server_source=plan['server_source'])
        source = dict(complete_inventory=True, runs=[dict(label=entry['label'], available=True)],
                      selection=dict(selected=controls), byte_selection=dict(selected=controls['max_bytes']))
        options = dict(analysis=source, manifest=dict(entries=[entry]), plans={entry['label']: plan},
                       controls=controls, server_source=plan['server_source'])
        self.assertEqual(generator.validate_context(options), {entry['label']: entry})

    def test_registered_pure_quota_requires_the_entire_registered_case_shape(self):
        mutations = dict(database_cpus=.2, levels=[128, 512, 1024], rounds=1, warmup='30s',
                         duration='30s', client_processes=8, payload_bytes=65536, records=2048,
                         write_percent=10, lua_mutations=True, backend='mongo', id='unregistered',
                         batching={}, hidden_control=True)
        for key, value in mutations.items():
            inputs = fixture()
            add_pure_quota_parent(inputs)
            inputs['plans']['confirm-pure-q0125']['case_sets']['confirm-pure-q0125'][0][key] = value
            with self.subTest(field=key), self.assertRaises(ValueError):
                generator.build_plan(inputs)

    def test_other_pure_quota_names_stay_excluded_and_failed_decisions_stay_invalid(self):
        inputs = fixture()
        add_pure_quota_parent(inputs)
        entry = inputs['manifest']['entries'][-1]
        entry['case_set'] = 'confirm-pure-unregistered'
        cases = inputs['plans'][entry['label']]['case_sets'].pop('confirm-pure-q0125')
        inputs['plans'][entry['label']]['case_sets'][entry['case_set']] = cases
        plan = generator.build_plan(inputs)
        self.assertEqual(plan['case_sets']['soak-pure'][0]['database_cpus'], 1)
        retained = plan['metadata']['qualified_and_skipped']
        self.assertTrue(any(point.get('label') == entry['label'] and 'excluded' in point.get('reason', '') for point in retained))
        inputs = fixture()
        add_pure_quota_parent(inputs)
        inputs['analysis']['formal']['cpu_capacity'][-1]['decision']['paths'][0]['points'][0]['decisions'][0]['passed'] = False
        with self.assertRaises(ValueError):
            generator.build_plan(inputs)

    def test_no_qualified_gate_means_no_check(self):
        inputs = fixture()
        for item in inputs['analysis']['formal']['cpu_capacity']:
            item['decision']['paths'][0]['points'][0]['capacity_qualified'] = False
        for item in inputs['analysis']['formal']['crawler_rate_bounds']:
            item['decision']['paths'][0]['soak_eligible'] = False
        plan = generator.build_plan(inputs)
        self.assertEqual(plan['case_sets'], {})
        self.assertEqual(plan['metadata']['checks'], 0)

    def test_registered_candidate_only_parents_compete_under_the_original_tie_order(self):
        inputs = fixture()
        add_independent_parents(inputs)
        plan = generator.build_plan(inputs)
        self.assertEqual(plan['metadata']['checks'], 3)
        self.assertEqual(plan['case_sets']['soak-partial'][0]['rate'], 512)
        self.assertEqual(plan['case_sets']['soak-crawler'][0]['rate'], 136)
        self.assertTrue(all(cases[0]['paths'] == ['optimized'] for name, cases in plan['case_sets'].items() if name != 'soak-pure'))
        retained = plan['metadata']['qualified_and_skipped']
        for tag in ('partial', 'crawler'):
            self.assertTrue(any(point.get('label') == 'confirm-' + tag + '-q025' and point.get('selected') is False for point in retained))

    def test_candidate_only_parent_requires_exact_document_source_and_hash(self):
        expected = dict(
            candidate_capacity_supplement='results/default-tuning-2026-10-16/CANDIDATE-CAPACITY-SUPPLEMENT.md',
            candidate_capacity_supplement_source='f7b01989520378d8794915361c1790b4f0eaec5a',
            candidate_capacity_supplement_sha256='769672a99683520d6ec6a522277d2d3befab2c060951fc66d92bf5ee0a8a2ada')
        self.assertEqual(generator.CANDIDATE_CAPACITY_REGISTRATION, expected)
        for key in expected:
            for replacement in (None, 'unregistered'):
                inputs = fixture()
                add_independent_parents(inputs)
                metadata = inputs['plans']['confirm-partial-independent-q025']['metadata']
                if replacement is None:
                    metadata.pop(key)
                else:
                    metadata[key] = replacement
                with self.subTest(key=key, replacement=replacement), self.assertRaises(ValueError):
                    generator.build_plan(inputs)

    def test_unregistered_single_paths_and_wrong_registered_profiles_are_rejected(self):
        paths = {'unregistered-optimized': ['optimized'], 'unregistered-direct': ['direct'],
                 'registered-direct': ['direct'], 'registered-paired': ['direct', 'optimized']}
        for mutation in ('unregistered-optimized', 'unregistered-direct', 'registered-direct', 'registered-paired', 'wrong-profile'):
            inputs = fixture()
            add_independent_parents(inputs)
            label = 'confirm-partial-independent-q025' if mutation.startswith('registered') or mutation == 'wrong-profile' else 'confirm-partial-q025'
            plan = inputs['plans'][label]
            plan['metadata'].update(generator.CANDIDATE_CAPACITY_REGISTRATION)
            for case in plan['case_sets'][label]:
                if mutation == 'wrong-profile':
                    case['profile'] = 'two-step'
                else:
                    case['paths'] = paths[mutation]
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                generator.build_plan(inputs)

    def test_single_path_registration_is_checked_even_without_a_capacity_bound(self):
        inputs = fixture()
        label = 'confirm-partial-q025'
        for case in inputs['plans'][label]['case_sets'][label]:
            case['paths'] = ['optimized']
        result = inputs['analysis']['formal']['crawler_rate_bounds'][0]
        result['decision']['paths'][0]['soak_eligible'] = False
        with self.assertRaises(ValueError):
            generator.build_plan(inputs)

    def test_candidate_only_parents_keep_round_and_genuine_overload_gates(self):
        for mutation in ('missing-lower-round', 'unknown-upper', 'not-overload'):
            inputs = fixture()
            add_independent_parents(inputs)
            result = next(result for result in inputs['analysis']['formal']['crawler_rate_bounds'] if result['label'] == 'confirm-partial-independent-q025')
            bound = result['decision']['paths'][0]
            if mutation == 'missing-lower-round':
                bound['points'][0]['rounds'].pop()
            elif mutation == 'unknown-upper':
                bound['points'][1]['rounds'][0]['evidence_errors'] = ['unknown completion outcome']
            else:
                bound['points'][1]['rounds'][0]['service_overload'] = False
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                generator.build_plan(inputs)

    def test_large_body_regression_is_excluded_even_if_qualified(self):
        inputs = fixture()
        extra = copy.deepcopy(inputs['analysis']['formal']['cpu_capacity'][0])
        extra['label'] = 'confirm-search-large-body'
        inputs['analysis']['formal']['cpu_capacity'].append(extra)
        inputs['manifest']['entries'].append(dict(label=extra['label'], phase='confirmation', case_set=extra['label'], server_source=inputs['server_source']))
        plan = copy.deepcopy(inputs['plans']['confirm-pure-q025'])
        cases = plan['case_sets'].pop('confirm-pure-q025')
        for case in cases:
            case.update(payload_bytes=65536, records=2048, levels=[512])
        plan['case_sets'][extra['label']] = cases
        inputs['plans'][extra['label']] = plan
        inputs['analysis']['runs'].append(dict(label=extra['label'], available=True))
        plan = generator.build_plan(inputs)
        self.assertEqual(plan['metadata']['checks'], 3)
        self.assertTrue(any('addendum' in point.get('reason', '') for point in plan['metadata']['qualified_and_skipped']))

    def test_missing_round_generator_failure_and_unregistered_tie_fail_closed(self):
        for mutation in ('missing-round', 'generator-failure', 'policy', 'selected', 'server', 'prior-soak', 'incomplete'):
            inputs = fixture()
            if mutation == 'missing-round':
                inputs['analysis']['formal']['cpu_capacity'][0]['decision']['paths'][0]['points'][0]['decisions'].pop()
            elif mutation == 'generator-failure':
                inputs['analysis']['formal']['crawler_rate_bounds'][0]['decision']['paths'][0]['points'][1]['rounds'][0]['evidence_errors'] = ['generator lag']
            elif mutation == 'policy':
                inputs['plans']['confirm-pure-q025']['metadata']['soak_derivation']['pure_order'] = []
            elif mutation == 'selected':
                inputs['controls'] = dict(inputs['controls'], max_bytes=2 << 20)
            elif mutation == 'server':
                inputs['server_source'] = 'c' * 40
            elif mutation == 'prior-soak':
                inputs['manifest']['entries'].append(dict(label='old-soak', phase='soak'))
            else:
                inputs['analysis']['complete_inventory'] = False
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                generator.build_plan(inputs)


if __name__ == '__main__':
    unittest.main()
