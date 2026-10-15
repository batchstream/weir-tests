#!/usr/bin/env python3
"""Generate exactly the registered wait sensitivity cases from committed inputs."""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[2]
PARENT_SOURCE = '682e725df1a81e29f4d4983d8711df1989c4adad'
PARENT_PATH = 'plans/default-confirmation-2026-10-16.json'
SUPPLEMENT_SOURCE = '5969c629970861f6f99e0d816c73192b90454ae9'
SUPPLEMENT_PATH = 'results/default-tuning-2026-10-16/WAIT-SENSITIVITY-SUPPLEMENT.md'
SUPPLEMENT_SHA256 = 'af98e75b0360963602fd72a01ae602ef70b94483d2983da53240a24a054b2acc'
SELECTOR_PATH = 'results/default-tuning-2026-10-16/wait_sensitivity_rules.py'
CLIENT_GO_REFERENCE = 'f06a346a8e0f37f0f8b2ae6539d9a7ce6dff5b27'


def build_plan(repository, selector_source):
    if re.fullmatch('[0-9a-f]{40}', selector_source) is None:
        raise ValueError('selector_source requires a full immutable commit')
    parent_raw = subprocess.check_output(['git', 'show', PARENT_SOURCE + ':' + PARENT_PATH], cwd=repository)
    parent = json.loads(parent_raw)
    document = subprocess.check_output(['git', 'show', SUPPLEMENT_SOURCE + ':' + SUPPLEMENT_PATH], cwd=repository)
    if hashlib.sha256(document).hexdigest() != SUPPLEMENT_SHA256:
        raise ValueError('registered wait supplement checksum differs')
    selector = subprocess.check_output(['git', 'show', selector_source + ':' + SELECTOR_PATH], cwd=repository)
    if selector != (repository / SELECTOR_PATH).read_bytes():
        raise ValueError('working selector differs from its committed source')
    anchor = dict(max_operations=128, max_bytes=4194304, max_wait='20ms', max_in_flight=16)
    case_sets = {}
    for tag, waits in [('partial', [20, 80, 40, 160]), ('crawler', [160, 40, 80, 20])]:
        name = 'wait-' + tag + '-q1'
        cases = []
        for wait in waits:
            case = copy.deepcopy(parent['case_sets']['confirm-' + tag + '-q1'][0])
            controls = dict(anchor, max_wait=str(wait) + 'ms')
            case.update(id=name + '-w' + str(wait) + 'ms', paths=['optimized'],
                        batching=controls, expected_batching=copy.deepcopy(controls))
            cases.append(case)
        case_sets[name] = cases
    sys.path.insert(0, str(repository / 'scripts'))
    from run_tuning_plan import duration_ns, validate_case
    inventory = {}
    for name, cases in case_sets.items():
        stages = sum(validate_case(case) for case in cases)
        minutes = sum(validate_case(case) * (duration_ns(case['warmup']) + duration_ns(case['duration'])) for case in cases) / 60e9
        inventory[name] = dict(cases=len(cases), timed_stages=stages, warmup_and_timed_minutes=minutes)
    metadata = dict(phase='wait_sensitivity',
                    parent_confirmation_plan=PARENT_PATH,
                    parent_confirmation_source=PARENT_SOURCE,
                    parent_confirmation_sha256=hashlib.sha256(parent_raw).hexdigest(),
                    wait_sensitivity_supplement=SUPPLEMENT_PATH,
                    wait_sensitivity_supplement_source=SUPPLEMENT_SOURCE,
                    wait_sensitivity_supplement_sha256=SUPPLEMENT_SHA256,
                    wait_selector=SELECTOR_PATH, wait_selector_source=selector_source,
                    wait_selector_sha256=hashlib.sha256(selector).hexdigest(),
                    client_go_reference=CLIENT_GO_REFERENCE, anchor_controls=anchor,
                    candidate_waits_ms=[20, 40, 80, 160], inventory=inventory)
    plan = dict(schema=parent['schema'], server_source=parent['server_source'],
                description='Registered four-wait sensitivity; original workload semantics and all failures retained',
                metadata=metadata, case_sets=case_sets)
    return plan


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--selector-source', required=True)
    args = parser.parse_args()
    print(json.dumps(build_plan(ROOT, args.selector_source), indent=2))
