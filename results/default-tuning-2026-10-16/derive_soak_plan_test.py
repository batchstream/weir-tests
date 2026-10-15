import copy
import importlib.util
from pathlib import Path
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


class SoakGeneratorTests(unittest.TestCase):
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

    def test_no_qualified_gate_means_no_check(self):
        inputs = fixture()
        for item in inputs['analysis']['formal']['cpu_capacity']:
            item['decision']['paths'][0]['points'][0]['capacity_qualified'] = False
        for item in inputs['analysis']['formal']['crawler_rate_bounds']:
            item['decision']['paths'][0]['soak_eligible'] = False
        plan = generator.build_plan(inputs)
        self.assertEqual(plan['case_sets'], {})
        self.assertEqual(plan['metadata']['checks'], 0)

    def test_large_body_regression_is_excluded_even_if_qualified(self):
        inputs = fixture()
        extra = copy.deepcopy(inputs['analysis']['formal']['cpu_capacity'][0])
        extra['label'] = 'confirm-search-large-body'
        inputs['analysis']['formal']['cpu_capacity'].append(extra)
        inputs['manifest']['entries'].append(dict(label=extra['label'], phase='confirmation', case_set=extra['label'], server_source=inputs['server_source']))
        inputs['plans'][extra['label']] = copy.deepcopy(inputs['plans']['confirm-pure-q025'])
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
