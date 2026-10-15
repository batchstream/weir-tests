#!/usr/bin/env python3
"""Derive at most three registered five-minute checks; never execute them."""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

POLICY = dict(version=1, max_checks=3,
              pure_case_sets=['confirm-pure-q025', 'confirm-pure-q1'],
              pure_order=['workers_ascending', 'database_cpus_ascending', 'label_ascending', 'case_id_ascending'],
              crawler_order=['lower_rate_descending', 'database_cpus_ascending', 'label_ascending'],
              crawler_profiles=['partial-update', 'two-step'], warmup='60s', duration='300s',
              rounds=1, client_processes=4, rerun_failed_soak=False)
INDEPENDENT_CRAWLER_PARENTS = {
    'confirm-partial-independent-q025': 'partial-update',
    'confirm-crawler-independent-q025': 'two-step',
}
CANDIDATE_CAPACITY_REGISTRATION = dict(
    candidate_capacity_supplement='results/default-tuning-2026-10-16/CANDIDATE-CAPACITY-SUPPLEMENT.md',
    candidate_capacity_supplement_source='f7b01989520378d8794915361c1790b4f0eaec5a',
    candidate_capacity_supplement_sha256='769672a99683520d6ec6a522277d2d3befab2c060951fc66d92bf5ee0a8a2ada')
PURE_QUOTA_PARENT = 'confirm-pure-q0125'
PURE_QUOTA_REGISTRATION = dict(
    pure_capacity_quota_supplement='results/default-tuning-2026-10-16/PURE-CAPACITY-QUOTA-SUPPLEMENT.md',
    pure_capacity_quota_supplement_source='f50bddd0537866354b564e3607202fe3c460faaa',
    pure_capacity_quota_supplement_sha256='e52d8433f9939174987451470fa075d8419e01acad9a58ed7db65ab5eaff1e89')
ORIGINAL_SOURCE = 'dc7c798a83d2d1700c3ff5b13f678f8af1e55d37'
PREPARATION_SOURCE = '2220c18eae4cbe5d079753155638f44cc7dc649b'
STARTUP_PARENT = 'confirm-pure-q0125-startup-fix'
STARTUP_REGISTRATION = dict(
    startup_preparation_correction='results/default-tuning-2026-10-16/STARTUP-PREPARATION-CORRECTION.md',
    startup_preparation_correction_source='ebf806f263bc47db45c120bdbbcaee35390dd234',
    startup_preparation_correction_sha256='5afd761ed613cc9b6c0fdae1aa376d59d0463818203286975a50fe7edbc86c5e',
    preparation_source=PREPARATION_SOURCE,
    preparation_backends_sha256='3c29e2474c7e521fc75ea9eb95b4ab3c3946bd69d792fd38fc86e6f029adcea8',
    preparation_test_sha256='e79a64c1685c71ba6ba74851824700892d36d42e116eb5bcf3211990c4b398a3')
PURE_PARENTS = {
    PURE_QUOTA_PARENT: dict(case_id='default-pure-q0125', registration=PURE_QUOTA_REGISTRATION),
    STARTUP_PARENT: dict(case_id='default-pure-q0125-startup-fix', registration=STARTUP_REGISTRATION),
}
PURE_QUOTA_SHAPE = dict(kind='write', backend='search', database_cpus=.125, client_processes=4,
                       rounds=3, warmup='60s', duration='60s', payload_bytes=1024, records=8192,
                       write_percent=100, lua_mutations=False, levels=[256, 512, 1024], batching=None)
STARTUP_CONTROLS = dict(max_operations=128, max_bytes=4194304, max_wait='20ms', max_in_flight=16)
STARTUP_SERVER = '084572d79b7a53230801297f90ca2363fd864913'


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def committed(repository, source, path):
    require(isinstance(source, str) and re.fullmatch('[0-9a-f]{40}', source), 'full immutable source required')
    require(isinstance(path, str) and path.startswith('plans/') and Path(path).suffix == '.json' and '..' not in Path(path).parts, 'ordinary committed plan path required')
    return subprocess.check_output(['git', 'show', source + ':' + path], cwd=repository)


def validate_startup_plan(options):
    entry, plan, entries, plans = (options[key] for key in ('entry', 'plan', 'entries', 'plans'))
    metadata = plan['metadata']
    require(all(metadata.get(key) == value for key, value in STARTUP_REGISTRATION.items()), 'startup correction registration differs')
    require(metadata.get('soak_derivation') == POLICY, 'startup correction changes the registered derivation policy')
    require(plan['server_source'] == entry['server_source'] == STARTUP_SERVER and metadata['selected_controls'] == STARTUP_CONTROLS, 'startup correction server or selected defaults differ')
    expected = dict(PURE_QUOTA_SHAPE, id=PURE_PARENTS[STARTUP_PARENT]['case_id'], expected_batching=STARTUP_CONTROLS)
    if entry['phase'] == 'confirmation' and entry['case_set'] == STARTUP_PARENT:
        require(metadata.get('phase') == 'independent_confirmation' and plan['case_sets'].get(STARTUP_PARENT) == [expected], 'startup confirmation shape differs')
        return
    require(entry['phase'] == 'soak' and entry['case_set'] == 'soak-pure', 'startup source is not registered for this cohort')
    require(metadata.get('phase') == 'soak' and metadata.get('retry_failed_soak') is False and type(metadata.get('checks')) is int and 1 <= metadata['checks'] <= POLICY['max_checks'], 'startup soak changes the registered phase/check/retry policy')
    proof = metadata.get('startup_preparation_parent')
    fields = {'label', 'case_set', 'case_id', 'client_source', 'plan_path', 'plan_sha256', 'workers'}
    require(isinstance(proof, dict) and set(proof) == fields, 'startup soak lacks exact parent proof')
    require(proof['case_set'] == STARTUP_PARENT and proof['case_id'] == expected['id'] and type(proof['workers']) is int and proof['workers'] in (256, 512), 'startup soak parent reference is not registered')
    require(proof['label'] in entries and proof['label'] in plans, 'startup soak parent is absent from confirmation inventory')
    parent = entries[proof['label']]
    require(parent['phase'] == 'confirmation' and all(parent.get(key) == proof[key] for key in ('label', 'case_set', 'client_source', 'plan_path', 'plan_sha256')), 'startup soak parent identity differs')
    parent_options = dict(entry=parent, plan=plans[proof['label']], entries=entries, plans=plans)
    validate_startup_plan(parent_options)
    selected = [point for point in metadata.get('qualified_and_skipped', []) if point.get('selected') is True and 'workers' in point]
    require(len(selected) == 1 and all(selected[0].get(key) == proof[key] for key in ('label', 'case_id', 'workers')) and selected[0].get('database_cpus') == .125, 'startup soak did not select its registered parent')
    expected.update(id='soak-pure', levels=[proof['workers']], rounds=1, duration='300s')
    require(plan['case_sets'].get('soak-pure') == [expected], 'startup soak shape differs')


def validate_context(options):
    analysis, manifest, plans, controls = (options[key] for key in ('analysis', 'manifest', 'plans', 'controls'))
    require(analysis.get('complete_inventory') is True, 'full audited confirmation inventory required')
    require(not any(entry['phase'] == 'soak' for entry in manifest['entries']), 'a declared soak already exists; failed/past soaks are never regenerated')
    selected = analysis['selection']['selected']
    require(selected and all(selected[key] == controls[key] for key in ('max_operations', 'max_wait', 'max_in_flight')) and analysis['byte_selection']['selected'] == controls['max_bytes'], 'selected controls differ from frozen selection output')
    require(set(controls) == {'max_operations', 'max_bytes', 'max_wait', 'max_in_flight'} and controls['max_in_flight'] > 0, 'four protective controls required')
    require(re.fullmatch('[0-9a-f]{40}', options['server_source']), 'full immutable selected-default server required')
    entries = {entry['label']: entry for entry in manifest['entries']}
    require(len(entries) == len(manifest['entries']), 'duplicate workflow label')
    require(sum(entry['case_set'] == STARTUP_PARENT for entry in entries.values()) <= 1, 'startup correction permits only one confirmation attempt')
    runs = analysis['runs']
    require(len(runs) == len(entries) and {run['label'] for run in runs} == set(entries) and all(run.get('available') is True for run in runs), 'analysis/manifest workflow inventories differ or are incomplete')
    for label, entry in entries.items():
        if entry['phase'] != 'confirmation':
            continue
        plan = plans[label]
        require(entry['server_source'] == plan['server_source'] == options['server_source'], 'confirmation uses another server')
        require(plan['metadata']['soak_derivation'] == POLICY, 'soak tie/derivation policy was not registered before confirmation')
        require(plan['metadata']['selected_controls'] == controls, 'confirmation controls differ')
        independent_profile = INDEPENDENT_CRAWLER_PARENTS.get(entry['case_set'])
        if independent_profile is not None:
            require(all(plan['metadata'].get(key) == value for key, value in CANDIDATE_CAPACITY_REGISTRATION.items()), 'candidate-only parent lacks the fixed supplement source/hash registration')
        require(entry['case_set'] in plan['case_sets'], 'confirmation parent case-set is missing')
        cases = plan['case_sets'][entry['case_set']]
        registration = PURE_PARENTS.get(entry['case_set'])
        if registration is not None:
            require(all(plan['metadata'].get(key) == value for key, value in registration['registration'].items()), 'additional pure parent lacks the fixed supplement source/hash registration')
            expected_case = dict(PURE_QUOTA_SHAPE, id=registration['case_id'], expected_batching=controls)
            require(cases == [expected_case], 'additional pure parent case differs from registration')
            if entry['case_set'] == STARTUP_PARENT:
                startup_options = dict(entry=entry, plan=plan, entries=entries, plans=plans)
                validate_startup_plan(startup_options)
        for case in cases:
            if independent_profile is not None:
                require(case['kind'] == 'crawler' and case['profile'] == independent_profile, 'candidate-only parent profile differs from registration')
            if case['kind'] == 'crawler':
                expected_paths = ['optimized'] if independent_profile is not None else ['direct', 'optimized']
                require(case['paths'] == expected_paths, 'crawler parent paths are not registered')
    return entries


def parent_case(options, label, case_id):
    entry = options['entries'][label]
    require(entry['phase'] == 'confirmation', 'soak parent must be independent confirmation')
    cases = options['plans'][label]['case_sets'][entry['case_set']]
    matches = [case for case in cases if case['id'] == case_id]
    require(len(matches) == 1, 'missing/duplicate parent case')
    case = matches[0]
    require(case['batching'] is None and case['expected_batching'] == options['controls'] and case['client_processes'] == 4 and case['rounds'] == 3 and case['warmup'] == case['duration'] == '60s', 'parent is not the registered omitted-default three-round confirmation')
    return case


def three_passed(decisions):
    return len(decisions) == 3 and {item['round'] for item in decisions} == {1, 2, 3} and all(item['passed'] is True for item in decisions)


def build_plan(options):
    entries = validate_context(options)
    context = dict(options, entries=entries)
    formal = options['analysis']['formal']
    pure, crawlers, retained = [], {name: [] for name in POLICY['crawler_profiles']}, []
    for result in formal['cpu_capacity']:
        label = result['label']
        require(label in entries, 'capacity parent workflow absent')
        if entries[label]['case_set'] not in POLICY['pure_case_sets'] and entries[label]['case_set'] not in PURE_PARENTS:
            retained.append(dict(label=label, case_id=result['case_id'], reason='Not a 1 KiB primary capacity case; addendum/regression excluded'))
            continue
        case = parent_case(context, label, result['case_id'])
        require(case['kind'] == 'write' and case['backend'] == 'search' and case['payload_bytes'] == 1024 and case['records'] == 8192 and case['write_percent'] == 100 and not case.get('lua_mutations'), 'pure parent workload differs')
        require(case['database_cpus'] == result['decision']['database_cpus'], 'pure capacity quota differs from parent')
        paths = [path for path in result['decision']['paths'] if path['path'] == 'weir']
        require(len(paths) == 1, 'missing/duplicate Weir capacity result')
        for point in paths[0]['points']:
            if not point['capacity_qualified']:
                continue
            require(three_passed(point['decisions']) and point['workers'] in case['levels'], 'capacity point lacks three qualified rounds')
            candidate = dict(label=label, case_id=case['id'], workers=point['workers'], database_cpus=case['database_cpus'], case=case)
            if entries[label]['case_set'] == STARTUP_PARENT:
                require(point['workers'] in (256, 512), 'startup parent has no following registered concurrency')
                proof = {key: entries[label][key] for key in ('label', 'case_set', 'client_source', 'plan_path', 'plan_sha256')}
                proof.update(case_id=case['id'], workers=point['workers'])
                candidate['startup_parent'] = proof
            pure.append(candidate)
    for result in formal['crawler_rate_bounds']:
        label, decision = result['label'], result['decision']
        require(label in entries, 'crawler parent workflow absent')
        profile = decision['profile']
        require(profile in crawlers, 'undeclared crawler profile')
        matches = [path for path in decision['paths'] if path['path'] == 'optimized']
        require(len(matches) == 1, 'missing/duplicate optimized rate-bound result')
        bound = matches[0]
        for point in bound['points']:
            if point['qualified'] and point['rate'] != bound['lower_rate']:
                retained.append(dict(label=label, profile=profile, rate=point['rate'], reason='Qualified rate is below the highest qualified rate; no replacement soak'))
        if not bound['soak_eligible']:
            retained.append(dict(label=label, profile=profile, reason='No qualified lower bound with an immediate genuine service-overload upper bound'))
            continue
        points = sorted(bound['points'], key=lambda point: point['rate'])
        index = next(i for i, point in enumerate(points) if point['rate'] == bound['lower_rate'])
        require(bound['bounded'] is True and index + 1 < len(points), 'rate bound has no next declared point')
        lower, upper = points[index:index+2]
        require(lower['qualified'] is True and three_passed(lower['rounds']) and upper['rate'] == bound['upper_rate'] and upper['service_overload'] is True and len(upper['rounds']) == 3 and {rd['round'] for rd in upper['rounds']} == {1, 2, 3} and all(rd['service_overload'] is True and not rd['evidence_errors'] for rd in upper['rounds']), 'rate bound lacks three qualified and genuine overload rounds')
        entry = entries[label]
        cases = options['plans'][label]['case_sets'][entry['case_set']]
        cases = [case for case in cases if case['profile'] == profile and case['rate'] == lower['rate']]
        require(len(cases) == 1, 'lower-bound case is missing or ambiguous')
        case = parent_case(context, label, cases[0]['id'])
        require(case['kind'] == 'crawler' and case['database_cpus'] == decision['database_cpus'] and case.get('page_size', 1) == 1, 'crawler parent topology differs')
        crawlers[profile].append(dict(label=label, case_id=case['id'], rate=lower['rate'], upper_rate=upper['rate'], database_cpus=case['database_cpus'], case=case))
    pure.sort(key=lambda point: (point['workers'], point['database_cpus'], point['label'], point['case_id']))
    chosen = []
    if pure:
        chosen.append(('soak-pure', pure[0]))
    for profile, name in (('partial-update', 'soak-partial'), ('two-step', 'soak-crawler')):
        crawlers[profile].sort(key=lambda point: (-point['rate'], point['database_cpus'], point['label']))
        if crawlers[profile]:
            chosen.append((name, crawlers[profile][0]))
    case_sets = {}
    for name, point in chosen:
        case = copy.deepcopy(point['case'])
        case.update(id=name, warmup='60s', duration='300s', rounds=1, client_processes=4, batching=None, expected_batching=dict(options['controls']))
        if case['kind'] == 'write':
            case['levels'] = [point['workers']]
        else:
            case['paths'] = ['optimized']
        case_sets[name] = [case]
    qualified = pure + [point for candidates in crawlers.values() for point in candidates]
    for point in qualified:
        visible = {key: value for key, value in point.items() if key not in ('case', 'startup_parent')}
        visible['selected'] = any(point is candidate for _, candidate in chosen)
        visible['reason'] = 'Selected by preregistered order' if visible['selected'] else 'Qualified but later in preregistered order; not another check'
        retained.append(visible)
    metadata = dict(phase='soak', soak_derivation=copy.deepcopy(POLICY), selected_controls=dict(options['controls']), input_sha256=options.get('input_sha256', {}), checks=len(chosen), qualified_and_skipped=retained, skipped_workloads=[name for name in ('soak-pure', 'soak-partial', 'soak-crawler') if name not in case_sets], retry_failed_soak=False)
    if chosen and chosen[0][0] == 'soak-pure' and 'startup_parent' in chosen[0][1]:
        metadata.update(STARTUP_REGISTRATION)
        metadata['startup_preparation_parent'] = copy.deepcopy(chosen[0][1]['startup_parent'])
    plan = dict(schema=1, server_source=options['server_source'], description='Derived five-minute checks of qualified omitted defaults; no dispatch or retries', metadata=metadata, case_sets=case_sets)
    if 'startup_preparation_parent' in metadata:
        entry = dict(label='soak-pure', phase='soak', case_set='soak-pure', server_source=options['server_source'])
        startup_options = dict(entry=entry, plan=plan, entries=entries, plans=options['plans'])
        validate_startup_plan(startup_options)
    return plan


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repository', type=Path, default=Path.cwd())
    parser.add_argument('--analysis', type=Path, required=True)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--selected', type=Path, required=True)
    parser.add_argument('--server-source', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    require(args.output.suffix == '.json' and not args.output.name.startswith('.env'), 'ordinary JSON output required')
    for key in ('analysis', 'manifest', 'selected'):
        path = getattr(args, key)
        require(path.suffix == '.json' and not path.name.startswith('.env') and path.name != 'credentials' and not path.is_symlink(), 'ordinary non-secret JSON input required')
    raw = {key: getattr(args, key).read_bytes() for key in ('analysis', 'manifest', 'selected')}
    analysis, manifest, controls = (json.loads(raw[key]) for key in ('analysis', 'manifest', 'selected'))
    plans = {}
    for entry in manifest['entries']:
        if entry['phase'] == 'confirmation':
            data = committed(args.repository, entry['client_source'], entry['plan_path'])
            require(sha(data) == entry['plan_sha256'], 'confirmation plan SHA256 differs')
            plans[entry['label']] = json.loads(data)
    options = dict(analysis=analysis, manifest=manifest, controls=controls, plans=plans, server_source=args.server_source, input_sha256={key: sha(value) for key, value in raw.items()})
    plan = build_plan(options)
    sys.path.insert(0, str(args.repository / 'scripts'))
    from run_tuning_plan import validate_case
    stages = sum(validate_case(case) for cases in plan['case_sets'].values() for case in cases)
    with args.output.open('x') as output:
        output.write(json.dumps(plan, indent=2) + '\n')
    print(json.dumps(dict(output=str(args.output), checks=plan['metadata']['checks'], timed_stages=stages, skipped=plan['metadata']['skipped_workloads'])))


if __name__ == '__main__':
    main()
