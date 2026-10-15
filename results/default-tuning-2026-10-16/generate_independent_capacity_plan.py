#!/usr/bin/env python3
"""Generate the prospective candidate-only supplement without reading results."""
import copy
import hashlib
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
SOURCE = '682e725df1a81e29f4d4983d8711df1989c4adad'
PARENT = 'plans/default-confirmation-2026-10-16.json'
SUPPLEMENT_SOURCE = 'f7b01989520378d8794915361c1790b4f0eaec5a'
SUPPLEMENT = 'results/default-tuning-2026-10-16/CANDIDATE-CAPACITY-SUPPLEMENT.md'


def committed(source, path):
    return subprocess.check_output(['git', 'show', source + ':' + path], cwd=ROOT)


def build_plan():
    parent_raw = committed(SOURCE, PARENT)
    parent = json.loads(parent_raw)
    metadata = copy.deepcopy(parent['metadata'])
    metadata.update(
        phase='candidate_only_capacity_supplement',
        parent_confirmation_plan=PARENT,
        parent_confirmation_source=SOURCE,
        parent_confirmation_sha256=hashlib.sha256(parent_raw).hexdigest(),
        candidate_capacity_supplement=SUPPLEMENT,
        candidate_capacity_supplement_source=SUPPLEMENT_SOURCE,
        candidate_capacity_supplement_sha256=hashlib.sha256(committed(SUPPLEMENT_SOURCE, SUPPLEMENT)).hexdigest())
    case_sets = {}
    for profile, rates in (('partial', [32, 64, 128, 256, 512, 1024, 2048, 4096]),
                           ('crawler', [8, 17, 34, 68, 136, 272, 544])):
        name = 'confirm-' + profile + '-independent-q025'
        template = parent['case_sets']['confirm-' + profile + '-q025'][0]
        cases = []
        for rate in rates:
            case = copy.deepcopy(template)
            case.update(id='default-' + profile + '-independent-q025-r' + str(rate),
                        rate=rate, paths=['optimized'])
            cases.append(case)
        case_sets[name] = cases
    sys.path.insert(0, str(ROOT / 'scripts'))
    from run_tuning_plan import duration_ns, validate_case
    inventory = {}
    for name, cases in case_sets.items():
        stages = sum(validate_case(case) for case in cases)
        minutes = sum(validate_case(case) * (duration_ns(case['warmup']) + duration_ns(case['duration'])) for case in cases) / 60e9
        inventory[name] = dict(cases=len(cases), timed_stages=stages, warmup_and_timed_minutes=minutes)
    metadata['inventory'] = inventory
    plan = dict(schema=parent['schema'], server_source=parent['server_source'],
                description='Registered independent omitted-default capacity ladders; original paired confirmation retained',
                metadata=metadata, case_sets=case_sets)
    return plan


if __name__ == '__main__':
    print(json.dumps(build_plan(), indent=2))
