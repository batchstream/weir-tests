#!/usr/bin/env python3
"""Generate the prospectively declared confirmation cases after default selection."""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys


ADDENDUM_SOURCE = '778a3b1ef3d48a9b1bb1e6e10bdae4d8edbce61e'
ADDENDUM_PATH = 'results/default-tuning-2026-10-16/CONFIRMATION-ADDENDUM.md'
LOWER_BRACKET_SOURCE = '40d1ac7e9d0672e1018571ee61e60398aa6db8f8'
LOWER_BRACKET_PATH = 'results/default-tuning-2026-10-16/LOWER-BRACKET-SUPPLEMENT.md'
SELECTION_ANALYSIS_PATH = 'results/default-tuning-2026-10-16/analysis-selection-parser-fix/analysis.json'
SOAK_DERIVATION = dict(version=1, max_checks=3,
                       pure_case_sets=['confirm-pure-q025', 'confirm-pure-q1'],
                       pure_order=['workers_ascending', 'database_cpus_ascending', 'label_ascending', 'case_id_ascending'],
                       crawler_order=['lower_rate_descending', 'database_cpus_ascending', 'label_ascending'],
                       crawler_profiles=['partial-update', 'two-step'], warmup='60s', duration='300s',
                       rounds=1, client_processes=4, rerun_failed_soak=False)


def build_plan(repository, server_source, controls):
    if re.fullmatch('[0-9a-f]{40}', server_source) is None:
        raise ValueError('server source must be a full immutable commit')
    sys.path.insert(0, str(repository / 'scripts'))
    from run_tuning_plan import duration_ns, validate_batching, validate_case
    validate_batching(controls)
    if controls['max_in_flight'] == 0:
        raise ValueError('the selected protective default must have a positive execution budget')
    selection_path = repository / 'plans/default-tuning-2026-10-16.json'
    selection_raw = selection_path.read_bytes()
    selection = json.loads(selection_raw)
    triples = {(case['batching']['max_operations'], duration_ns(case['batching']['max_wait']),
                case['batching']['max_in_flight']) for case in selection['case_sets']['select-pure']}
    selected_triple = (controls['max_operations'], duration_ns(controls['max_wait']), controls['max_in_flight'])
    byte_targets = {case['batching']['max_bytes'] for case in selection['case_sets']['select-bytes']}
    if selected_triple not in triples or controls['max_bytes'] not in byte_targets:
        raise ValueError('selected controls must belong to the prospectively declared count/wait/slot and byte inventories')
    method_path = Path('results/default-tuning-2026-10-16/PLAN.md')
    method = repository / method_path
    method_sha256 = hashlib.sha256(method.read_bytes()).hexdigest()
    addendum_raw = subprocess.check_output(['git', 'show', ADDENDUM_SOURCE + ':' + ADDENDUM_PATH], cwd=repository)
    lower_bracket_raw = subprocess.check_output(['git', 'show', LOWER_BRACKET_SOURCE + ':' + LOWER_BRACKET_PATH], cwd=repository)
    selection_analysis_raw = subprocess.check_output(['git', 'show', LOWER_BRACKET_SOURCE + ':' + SELECTION_ANALYSIS_PATH], cwd=repository)
    write_defaults = dict(kind='write', database_cpus=1, client_processes=4,
                          rounds=3, warmup='30s', duration='30s', payload_bytes=1024,
                          records=8192, write_percent=100, lua_mutations=False)
    crawler_defaults = dict(kind='crawler', database_cpus=0.25, client_processes=4,
                            rounds=3, warmup='60s', duration='60s', concurrency=512,
                            paths=['direct', 'optimized'], http_observer='disabled', page_size=1)
    case_sets = {}
    for tag, quota in [('q025', 0.25), ('q1', 1)]:
        case = dict(write_defaults, id='default-pure-' + tag, variant='default',
                    backend='search', database_cpus=quota, levels=[128, 512, 1024],
                    warmup='60s', duration='60s')
        case_sets['confirm-pure-' + tag] = [case]
    case_sets['confirm-partial-q025'] = []
    for rate in [128, 256, 512, 1024, 2048, 4096]:
        case = dict(crawler_defaults, id='default-partial-q025-r' + str(rate), variant='default',
                    profile='partial-update', rate=rate, visibility='application_acknowledged',
                    seed_padding_bytes=16384)
        case_sets['confirm-partial-q025'].append(case)
    case_sets['confirm-crawler-q025'] = []
    for rate in [34, 68, 136, 272, 544]:
        case = dict(crawler_defaults, id='default-crawler-q025-r' + str(rate), variant='default',
                    profile='two-step', rate=rate, visibility='query_visible', seed_padding_bytes=0)
        case_sets['confirm-crawler-q025'].append(case)
    case_sets['confirm-partial-lower-q025'] = []
    for rate in [32, 64, 128, 256]:
        case = dict(crawler_defaults, id='default-partial-lower-q025-r' + str(rate), variant='default',
                    profile='partial-update', rate=rate, visibility='application_acknowledged',
                    seed_padding_bytes=16384)
        case_sets['confirm-partial-lower-q025'].append(case)
    case_sets['confirm-crawler-lower-q025'] = []
    for rate in [8, 17, 34]:
        case = dict(crawler_defaults, id='default-crawler-lower-q025-r' + str(rate), variant='default',
                    profile='two-step', rate=rate, visibility='query_visible', seed_padding_bytes=0)
        case_sets['confirm-crawler-lower-q025'].append(case)
    case = dict(crawler_defaults, id='default-partial-q1-r512', variant='default',
                profile='partial-update', rate=512, database_cpus=1,
                visibility='application_acknowledged', seed_padding_bytes=16384)
    case_sets['confirm-partial-q1'] = [case]
    case = dict(crawler_defaults, id='default-crawler-q1-r68', variant='default',
                profile='two-step', rate=68, database_cpus=1,
                visibility='query_visible', seed_padding_bytes=0)
    case_sets['confirm-crawler-q1'] = [case]
    for backend in ['search', 'mongo']:
        name = 'confirm-' + backend + '-read'
        case_sets[name] = []
        for variant in ['default', 'legacy']:
            case = dict(write_defaults, id=variant + '-' + backend + '-read-q1', variant=variant,
                        backend=backend, levels=[4, 128], write_percent=0)
            case_sets[name].append(case)
    case_sets['confirm-mongo-regression'] = []
    for profile, write_percent, lua_mutations in [('write', 100, False), ('mixed', 10, False), ('lua', 100, True)]:
        for variant in ['default', 'legacy']:
            case = dict(write_defaults, id=variant + '-mongo-' + profile + '-q1', variant=variant,
                        backend='mongo', levels=[128], write_percent=write_percent, lua_mutations=lua_mutations)
            case_sets['confirm-mongo-regression'].append(case)
    case_sets['confirm-search-large-body'] = []
    for variant in ['default', 'legacy']:
        case = dict(write_defaults, id=variant + '-search-large-body-q1', variant=variant,
                    backend='search', levels=[512], payload_bytes=65536, records=2048,
                    warmup='60s', duration='60s',
                    description='Independent joint byte/count regression; not new selection or a sustained-capacity candidate')
        case_sets['confirm-search-large-body'].append(case)
    legacy = dict(max_operations=32, max_bytes=8388608, max_wait='0s', max_in_flight=0)
    inventory = {}
    ids = set()
    for name, cases in case_sets.items():
        stages, measurement_ns = 0, 0
        for case in cases:
            variant = case.pop('variant')
            if variant == 'default':
                case['batching'] = None
                case['expected_batching'] = dict(controls)
            else:
                case['batching'] = dict(legacy)
            if case['id'] in ids:
                raise ValueError('duplicate confirmation case identity')
            ids.add(case['id'])
            case_stages = validate_case(case)
            stages += case_stages
            measurement_ns += case_stages * (duration_ns(case['warmup']) + duration_ns(case['duration']))
        if stages > 256 or measurement_ns > 100 * 60 * 1000000000:
            raise ValueError('case set exceeds the existing finite workflow budget: ' + name)
        inventory[name] = dict(cases=len(cases), timed_stages=stages,
                               warmup_and_timed_minutes=measurement_ns / 60e9)
    if sum(entry['timed_stages'] for entry in inventory.values()) != 252:
        raise ValueError('confirmation inventory no longer matches the prospectively declared 252-stage design')
    metadata = dict(phase='independent_confirmation', method=method_path.as_posix(),
                    method_sha256=method_sha256, selected_controls=dict(controls),
                    selection_plan_sha256=hashlib.sha256(selection_raw).hexdigest(),
                    confirmation_addendum=ADDENDUM_PATH, confirmation_addendum_source=ADDENDUM_SOURCE,
                    confirmation_addendum_sha256=hashlib.sha256(addendum_raw).hexdigest(),
                    lower_bracket_supplement=LOWER_BRACKET_PATH, lower_bracket_supplement_source=LOWER_BRACKET_SOURCE,
                    lower_bracket_supplement_sha256=hashlib.sha256(lower_bracket_raw).hexdigest(),
                    selection_analysis=SELECTION_ANALYSIS_PATH, selection_analysis_source=LOWER_BRACKET_SOURCE,
                    selection_analysis_sha256=hashlib.sha256(selection_analysis_raw).hexdigest(),
                    joint_regression_case_set='confirm-search-large-body',
                    soak_derivation=copy.deepcopy(SOAK_DERIVATION),
                    p99_slo_seconds=5, inventory=inventory,
                    five_minute_checks='Excluded here: dispatch only after the frozen qualifying lower-bound rules pass')
    plan = dict(schema=1, server_source=server_source,
                description='Independent confirmation of actual omitted defaults; four OS clients; see frozen PLAN.md',
                metadata=metadata, case_sets=case_sets)
    return plan


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repository', type=Path, default=Path.cwd())
    parser.add_argument('--server-source', required=True)
    parser.add_argument('--max-operations', type=int, required=True)
    parser.add_argument('--max-bytes', type=int, required=True)
    parser.add_argument('--max-wait', required=True)
    parser.add_argument('--max-in-flight', type=int, required=True)
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    repository = args.repository.resolve()
    controls = dict(max_operations=args.max_operations, max_bytes=args.max_bytes,
                    max_wait=args.max_wait, max_in_flight=args.max_in_flight)
    plan = build_plan(repository, args.server_source, controls)
    output = args.output if args.output is not None else repository / 'plans/default-confirmation-2026-10-16.json'
    if output.suffix != '.json' or output.name.startswith('.env'):
        raise ValueError('output must be an ordinary JSON plan')
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open('x') as stream:
        stream.write(json.dumps(plan, indent=2) + '\n')
    summary = dict(output=str(output), server_source=args.server_source,
                   case_sets=plan['metadata']['inventory'],
                   timed_stages=sum(entry['timed_stages'] for entry in plan['metadata']['inventory'].values()))
    print(json.dumps(summary, indent=2))


if __name__ == '__main__':
    main()
