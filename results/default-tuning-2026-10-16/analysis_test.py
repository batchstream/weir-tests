"""Independent counterexamples for the prospectively frozen selection rules."""
import copy
import gzip
import importlib.util
import json
from pathlib import Path
import sys
import unittest
from unittest import mock

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


def startup_capacity_fixture():
    derivation = analysis.soak_derivation
    path = 'plans/default-pure-capacity-q0125-startup-fix-2026-10-16.json'
    raw = (analysis.REPO / path).read_bytes()
    plan = json.loads(raw)
    parent = dict(label=derivation.STARTUP_PARENT, phase='confirmation', case_set=derivation.STARTUP_PARENT,
                  client_source=derivation.PREPARATION_SOURCE, server_source=derivation.STARTUP_SERVER,
                  plan_path=path, plan_sha256=analysis.checksum(raw))
    rows = []
    for path_name in ('direct', 'weir'):
        for workers in (256, 512, 1024):
            for rd in (1, 2, 3):
                row = dict(label=parent['label'], phase='confirmation', case_set=parent['case_set'],
                           case_id=derivation.PURE_PARENTS[derivation.STARTUP_PARENT]['case_id'],
                           kind='write', profile=None, database_cpus=.125, path=path_name,
                           workers=workers, round=rd, usable=True, checks_passed=True, verified=True,
                           completed=60000, completed_s=1000, failed=0, unknown=0, timeout=0,
                           db_cpu_coverage=1, sustained_database_cpu_saturation=True,
                           db_cpu_quota_percent=99, fraction_measurement_at_90_percent_cpu=1,
                           cpu_intervals=[{} for _ in range(5)], host_headroom=True, host_coverage=1,
                           host_busy_percent=40, host_steal_percent=0, effective_host_cpus=4,
                           client_headroom=True, client_process_cpu=[dict(pid=pid, coverage=1, cpu_cores=.1) for pid in (1, 2, 3, 4)],
                           rate_stable=True, p99_ms=1, db_cpu_ms_per_completed=.12375)
                rows.append(row)
    manifest = dict(entries=[parent])
    formal_options = dict(manifest=manifest, rows=rows)
    with mock.patch.object(analysis, 'committed', return_value=raw):
        formal = analysis.formal_results(formal_options)
    selection = dict(selected=copy.deepcopy(derivation.STARTUP_CONTROLS))
    byte_selection = dict(selected=derivation.STARTUP_CONTROLS['max_bytes'])
    run = dict(label=parent['label'], available=True)
    actual = dict(complete_inventory=True, runs=[run], selection=selection, byte_selection=byte_selection, formal=formal)
    derivation_options = dict(analysis=actual, manifest=manifest, plans={parent['label']: plan},
                              controls=derivation.STARTUP_CONTROLS, server_source=derivation.STARTUP_SERVER)
    record = derivation.build_plan(derivation_options)
    result = dict(parent=parent, parent_raw=raw, parent_plan=plan, rows=rows, record=record,
                  run=run, selection=selection, byte_selection=byte_selection)
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


class ReportedDurationTests(unittest.TestCase):
    def test_go_report_durations_are_exact_integer_nanoseconds(self):
        cases = [('0s', 0), ('60s', 60_000_000_000), ('1m0s', 60_000_000_000),
                 ('5m0s', 300_000_000_000), ('1h2m3.000000004s', 3_723_000_000_004),
                 ('1.25ms', 1_250_000), ('1.5us', 1500), ('1.5µs', 1500), ('1.5μs', 1500),
                 ('0.000000001s', 1), ('2562047h47m16.854775807s', 9_223_372_036_854_775_807)]
        for value, expected in cases:
            with self.subTest(value=value):
                self.assertEqual(analysis.reported_duration_ns(value), expected)

    def test_invalid_negative_overflow_and_fractional_nanoseconds_fail_closed(self):
        cases = [None, 60, True, '', '0', '-1s', '+1s', ' 1m0s', '1m 0s', '1m0sx',
                 '1d', '1e3s', 'NaNs', 'Infms', '1.1ns', '0.0000000001s', '0.5ns0.5ns',
                 '9223372036.854775807000000000000000000001s',
                 '9223372036854775808ns', '2562047h47m16.854775808s']
        for value in cases:
            with self.subTest(value=value), self.assertRaises(ValueError):
                analysis.reported_duration_ns(value)

    def test_compound_raw_support_does_not_relax_plan_duration_validation(self):
        self.assertEqual(analysis.reported_duration_ns('1m0s'), analysis.runner.duration_ns('60s'))
        self.assertNotEqual(analysis.reported_duration_ns('1m0.000000001s'), analysis.runner.duration_ns('60s'))
        with self.assertRaises(ValueError):
            analysis.runner.duration_ns('1m0s')


class PreparationSourceTests(unittest.TestCase):
    def test_unchanged_go_tree_cannot_hide_per_client_embedded_drift(self):
        manifest = json.loads((FOLDER / 'manifest.json').read_bytes())
        legacy = next(entry for entry in manifest['entries'] if entry['case_set'] == 'confirm-pure-q025')
        plan_path = 'plans/default-pure-capacity-q0125-startup-fix-2026-10-16.json'
        correction_raw = (analysis.REPO / plan_path).read_bytes()
        corrected = dict(label='startup', phase='confirmation', case_set=analysis.soak_derivation.STARTUP_PARENT,
                         client_source=analysis.soak_derivation.PREPARATION_SOURCE,
                         server_source=analysis.soak_derivation.STARTUP_SERVER, plan_path=plan_path,
                         plan_sha256=analysis.checksum(correction_raw))
        legacy_raw = analysis.committed(analysis.REPO, legacy['client_source'], legacy['plan_path'])
        prep_tree, _ = analysis.prior.source_snapshot(analysis.REPO, analysis.soak_derivation.PREPARATION_SOURCE)
        analysis.prior.source_snapshot(analysis.REPO, analysis.soak_derivation.ORIGINAL_SOURCE)
        for entry, raw in ((legacy, legacy_raw), (corrected, correction_raw)):
            analysis.prior.source_snapshot(analysis.REPO, entry['client_source'])
            embedded = []
            for path in analysis.EMBEDDED_INPUTS:
                embedded.extend([analysis.committed(analysis.REPO, analysis.soak_derivation.ORIGINAL_SOURCE, path),
                                 analysis.committed(analysis.REPO, entry['client_source'], path)])
            embedded[3] += b'changed embedded program'
            client_manifest = dict(entries=[entry])
            with self.subTest(label=entry['label']), mock.patch.object(analysis, 'preparation_source_proof', return_value=prep_tree), mock.patch.object(analysis, 'committed', return_value=raw), mock.patch.object(analysis.subprocess, 'check_output', side_effect=embedded), self.assertRaises(ValueError):
                analysis.measurement_source_proof(client_manifest)

    def test_corrected_soak_eligibility_comes_from_audited_rows_not_metadata(self):
        for mutation in ('valid', 'actual-unqualified', 'actual-different-point', 'tie-spoof'):
            inputs = startup_capacity_fixture()
            parent, record = inputs['parent'], inputs['record']
            if mutation == 'actual-unqualified':
                for row in inputs['rows']:
                    if row['path'] == 'weir':
                        row['rate_stable'] = False
            elif mutation == 'actual-different-point':
                inputs['rows'][9]['sustained_database_cpu_saturation'] = False
            elif mutation == 'tie-spoof':
                record['case_sets']['soak-pure'][0]['levels'] = [512]
                record['metadata']['startup_preparation_parent']['workers'] = 512
                for point in record['metadata']['qualified_and_skipped']:
                    if 'workers' in point:
                        point['selected'] = point['workers'] == 512
            raw = json.dumps(record).encode()
            entry = dict(label='soak-pure', phase='soak', case_set='soak-pure', client_source=analysis.soak_derivation.PREPARATION_SOURCE,
                         server_source=analysis.soak_derivation.STARTUP_SERVER, plan_path='plans/soak-test.json', plan_sha256=analysis.checksum(raw))
            parent_entries = {parent['label']: parent}
            parent_plans = {parent['label']: inputs['parent_plan']}
            preflight = dict(entry=entry, plan=record, entries=parent_entries, plans=parent_plans)
            analysis.soak_derivation.validate_startup_plan(preflight)
            manifest = dict(entries=[parent, entry])
            soak_run = dict(label=entry['label'], available=True)
            options = dict(manifest=manifest, rows=inputs['rows'], runs=[inputs['run'], soak_run],
                           selection=inputs['selection'], byte_selection=inputs['byte_selection'])
            with self.subTest(mutation=mutation), mock.patch.object(analysis, 'committed', side_effect=[raw, inputs['parent_raw'], inputs['parent_raw']]):
                if mutation == 'valid':
                    analysis.validate_startup_soak_derivation(options)
                else:
                    with self.assertRaises(ValueError):
                        analysis.validate_startup_soak_derivation(options)

    def test_frozen_preparation_diff_and_all_embeds_match_registration(self):
        tree = analysis.preparation_source_proof()
        expected, _ = analysis.prior.source_snapshot(analysis.REPO, analysis.soak_derivation.PREPARATION_SOURCE)
        self.assertEqual(tree, expected)
        registration = analysis.soak_derivation.STARTUP_REGISTRATION
        document = analysis.committed(analysis.REPO, registration['startup_preparation_correction_source'], registration['startup_preparation_correction'])
        self.assertEqual(analysis.checksum(document), registration['startup_preparation_correction_sha256'])

    def test_preparation_audit_rejects_source_test_and_embedded_byte_drift(self):
        base, prep = analysis.soak_derivation.ORIGINAL_SOURCE, analysis.soak_derivation.PREPARATION_SOURCE
        raw = [analysis.committed(analysis.REPO, base, 'internal/fixture/backends.go'),
               analysis.committed(analysis.REPO, prep, 'internal/fixture/backends.go'),
               analysis.committed(analysis.REPO, prep, 'internal/fixture/backends_deadline_test.go')]
        for name in ('common.lua', 'product.lua', 'offer.lua', 'native.painless'):
            path = 'internal/benchmark/crawlerdata/' + name
            raw.extend([analysis.committed(analysis.REPO, base, path), analysis.committed(analysis.REPO, prep, path)])
        for index in (1, 2, 4):
            corrupted = list(raw)
            corrupted[index] += b'\n'
            with self.subTest(index=index), mock.patch.object(analysis, 'committed', side_effect=corrupted), self.assertRaises(ValueError):
                analysis.preparation_source_proof()

    def test_cprep_tree_is_admitted_only_for_the_registered_confirmation(self):
        path = 'plans/default-pure-capacity-q0125-startup-fix-2026-10-16.json'
        original = json.loads((analysis.REPO / path).read_bytes())
        prep = analysis.soak_derivation.PREPARATION_SOURCE
        tree, _ = analysis.prior.source_snapshot(analysis.REPO, prep)
        for mutation in ('valid', 'old-source', 'wrong-cohort', 'missing-registration', 'wrong-registration', 'changed-controls', 'second-attempt'):
            plan = copy.deepcopy(original)
            entry = dict(label='startup', phase='confirmation', case_set=analysis.soak_derivation.STARTUP_PARENT,
                         client_source=prep, server_source=plan['server_source'], plan_path=path)
            if mutation == 'old-source':
                entry['client_source'] = analysis.soak_derivation.ORIGINAL_SOURCE
            elif mutation == 'wrong-cohort':
                entry['case_set'] = 'confirm-pure-q025'
            elif mutation == 'missing-registration':
                plan['metadata'].pop('startup_preparation_correction_source')
            elif mutation == 'wrong-registration':
                plan['metadata']['startup_preparation_correction_sha256'] = '0' * 64
            elif mutation == 'changed-controls':
                plan['metadata']['selected_controls']['max_operations'] = 64
            raw = json.dumps(plan).encode()
            entry['plan_sha256'] = analysis.checksum(raw)
            manifest = dict(entries=[entry])
            if mutation == 'second-attempt':
                duplicate = dict(entry, label='another-startup-attempt')
                manifest['entries'].append(duplicate)
            with self.subTest(mutation=mutation), mock.patch.object(analysis, 'preparation_source_proof', return_value=tree), mock.patch.object(analysis, 'committed', return_value=raw):
                if mutation == 'valid':
                    analysis.measurement_source_proof(manifest)
                else:
                    with self.assertRaises(ValueError):
                        analysis.measurement_source_proof(manifest)

    def test_previous_clients_keep_the_exact_c0_tree_requirement(self):
        manifest = json.loads((FOLDER / 'manifest.json').read_bytes())
        analysis.measurement_source_proof(manifest)
        _, source = analysis.prior.source_snapshot(analysis.REPO, analysis.soak_derivation.PREPARATION_SOURCE)
        original_tree, original = analysis.prior.source_snapshot(analysis.REPO, analysis.soak_derivation.ORIGINAL_SOURCE)
        with mock.patch.object(analysis, 'preparation_source_proof', return_value='f' * 64), mock.patch.object(analysis.prior, 'source_snapshot', side_effect=[(original_tree, original), ('e' * 64, source)]), self.assertRaises(ValueError):
            analysis.measurement_source_proof(manifest)

    def test_exact_derived_pure_soak_can_use_cprep_with_its_bound_parent(self):
        derivation = analysis.soak_derivation
        path = 'plans/default-pure-capacity-q0125-startup-fix-2026-10-16.json'
        parent_raw = (analysis.REPO / path).read_bytes()
        parent = dict(label=derivation.STARTUP_PARENT, phase='confirmation', case_set=derivation.STARTUP_PARENT,
                      client_source=derivation.PREPARATION_SOURCE, server_source=derivation.STARTUP_SERVER,
                      plan_path=path, plan_sha256=analysis.checksum(parent_raw))
        proof = {key: parent[key] for key in ('label', 'case_set', 'client_source', 'plan_path', 'plan_sha256')}
        proof.update(case_id=derivation.PURE_PARENTS[derivation.STARTUP_PARENT]['case_id'], workers=512)
        point = dict(label=parent['label'], case_id=proof['case_id'], workers=512, database_cpus=.125, selected=True)
        metadata = dict(derivation.STARTUP_REGISTRATION, phase='soak', selected_controls=derivation.STARTUP_CONTROLS,
                        soak_derivation=derivation.POLICY, retry_failed_soak=False, checks=1,
                        startup_preparation_parent=proof, qualified_and_skipped=[point])
        case = dict(derivation.PURE_QUOTA_SHAPE, id='soak-pure', levels=[512], rounds=1,
                    duration='300s', expected_batching=derivation.STARTUP_CONTROLS)
        plan = dict(schema=1, server_source=derivation.STARTUP_SERVER, metadata=metadata, case_sets={'soak-pure': [case]})
        raw = json.dumps(plan).encode()
        entry = dict(label='soak-pure', phase='soak', case_set='soak-pure', client_source=derivation.PREPARATION_SOURCE,
                     server_source=derivation.STARTUP_SERVER, plan_path='plans/soak-test.json', plan_sha256=analysis.checksum(raw))
        tree, _ = analysis.prior.source_snapshot(analysis.REPO, derivation.PREPARATION_SOURCE)
        for repeated in (False, True):
            manifest = dict(entries=[parent, entry])
            inputs = [parent_raw, raw]
            if repeated:
                manifest['entries'].append(dict(entry, label='another-soak-attempt'))
                inputs.append(raw)
            with self.subTest(repeated=repeated), mock.patch.object(analysis, 'preparation_source_proof', return_value=tree), mock.patch.object(analysis, 'committed', side_effect=inputs):
                if repeated:
                    with self.assertRaises(ValueError):
                        analysis.measurement_source_proof(manifest)
                else:
                    analysis.measurement_source_proof(manifest)


class RawPreflightTests(unittest.TestCase):
    def test_four_real_confirmation_cases_match_declared_durations(self):
        manifest = json.loads((FOLDER / 'manifest.json').read_text())
        expected_sets = {'confirm-pure-q025', 'confirm-pure-q1', 'confirm-search-large-body'}
        entries = [entry for entry in manifest['entries'] if entry['case_set'] in expected_sets]
        self.assertEqual({entry['case_set'] for entry in entries}, expected_sets)
        checked = []
        for entry in entries:
            plan = json.loads(analysis.committed(analysis.REPO, entry['client_source'], entry['plan_path']))
            folder = FOLDER / 'raw' / entry['label']
            for case in plan['case_sets'][entry['case_set']]:
                report = analysis.load(analysis.prior.one(folder, case['id'] + '/measurement/search.json.gz'))
                params = report['parameters']
                for report_field, plan_field in [('warmup_duration', 'warmup'), ('measurement_duration', 'duration')]:
                    with self.subTest(label=entry['label'], case_id=case['id'], field=report_field):
                        self.assertEqual(params[report_field], '1m0s')
                        self.assertEqual(analysis.reported_duration_ns(params[report_field]), analysis.runner.duration_ns(case[plan_field]))
                        with self.assertRaises(ValueError):
                            analysis.runner.duration_ns(params[report_field])
                receipt = analysis.load(analysis.prior.one(folder, case['id'] + '/receipt.json.gz'))
                options = dict(folder=folder, entry=entry, case=case, receipt=receipt)
                rows = analysis.process_write(options)
                self.assertEqual(len(rows), len(case['levels']) * case['rounds'] * 2)
                checked.append((entry['label'], case['id']))
        self.assertEqual(len(checked), 4)
        self.assertEqual(len(set(checked)), 4)

    def test_real_mongo_read_has_no_search_http_observations(self):
        manifest = json.loads((FOLDER / 'manifest.json').read_text())
        entry = next(item for item in manifest['entries'] if item['case_set'] == 'select-mongo-read')
        plan = json.loads(analysis.committed(analysis.REPO, entry['client_source'], entry['plan_path']))
        case = next(item for item in plan['case_sets'][entry['case_set']] if item['id'] == 'c128-w5ms-s16')
        folder = FOLDER / 'raw' / entry['label']
        receipt = analysis.load(analysis.prior.one(folder, case['id'] + '/receipt.json.gz'))
        rows = analysis.process_write(dict(folder=folder, entry=entry, case=case, receipt=receipt))
        self.assertEqual(len(rows), 4)
        self.assertTrue(all(row['usable'] for row in rows), [row['error'] for row in rows])
        self.assertTrue(all('search_metric_deltas' not in row for row in rows))

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
