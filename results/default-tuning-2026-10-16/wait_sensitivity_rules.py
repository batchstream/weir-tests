"""Choose only from the preregistered four-wait, two-profile sensitivity cohort."""
import argparse
import hashlib
import json
from pathlib import Path
from statistics import geometric_mean

from capacity_rules import crawler_evidence_errors, finite


WAITS_MS = (20, 40, 80, 160)
PROFILES = {'wait-partial-q1': ('partial-update', 512),
            'wait-crawler-q1': ('two-step', 68)}


def choose_wait(rows):
    indexed = {}
    for row in rows:
        cohort = row.get('case_set')
        if cohort not in PROFILES:
            continue
        wait_ns, rd = row.get('wait_ns'), row.get('round')
        if type(wait_ns) is not int or wait_ns not in [wait * 1_000_000 for wait in WAITS_MS] or type(rd) is not int or rd not in (1, 2, 3):
            raise ValueError('unexpected wait or round in sensitivity cohort')
        wait = wait_ns // 1_000_000
        profile, rate = PROFILES[cohort]
        controls = dict(max_operations=128, max_bytes=4194304,
                        max_wait=str(wait) + 'ms', max_in_flight=16)
        identity = dict(phase='wait_sensitivity', case_id=cohort + '-w' + str(wait) + 'ms',
                        path='optimized', kind='crawler', profile=profile, rate=rate,
                        workers=512, database_cpus=1, controls=controls)
        if any(row.get(key) != value for key, value in identity.items()):
            raise ValueError('sensitivity row identity or controls differ from the fixed plan')
        key = wait, cohort, rd
        if key in indexed:
            raise ValueError('duplicate sensitivity profile/round')
        indexed[key] = row

    ranking = []
    for wait in WAITS_MS:
        reasons = []
        for cohort, (_, rate) in PROFILES.items():
            for rd in (1, 2, 3):
                row = indexed.get((wait, cohort, rd))
                prefix = cohort + ' round ' + str(rd) + ': '
                if row is None:
                    reasons.append(prefix + 'missing')
                    continue
                reasons.extend(prefix + message for message in crawler_evidence_errors(row))
                if row.get('usable') is not True or row.get('rate_stable') is not True or row.get('error'):
                    reasons.append(prefix + 'unqualified or unstable')
                audit = row.get('original_go_audit')
                if not isinstance(audit, dict) or audit.get('exit_code') != 0:
                    reasons.append(prefix + 'original Go audit did not pass')
                if row.get('offered') != rate * 60 or row.get('completed') != rate * 60 or row.get('rejected') != 0:
                    reasons.append(prefix + 'not all fixed-window arrivals completed')
                if not finite(row.get('p99_ms')) or row['p99_ms'] > 5000 or not finite(row.get('drain_s')) or row['drain_s'] > 3:
                    reasons.append(prefix + 'p99/drain SLO failed or unavailable')
                if not finite(row.get('db_cpu_ms_per_completed'), True):
                    reasons.append(prefix + 'CPU per completed unit unavailable')
        entry = dict(wait_ms=wait, qualified=not reasons, eligible=False,
                     reasons=reasons, comparisons=[], per_round_cpu_ratios=[], cpu_score=None)
        ranking.append(entry)

    result = dict(schema=1, outcome='inconclusive', selected_wait_ms=None,
                  anchor_qualified=ranking[0]['qualified'], ranking=ranking,
                  cpu_plateau_waits_ms=[])
    if not result['anchor_qualified']:
        return result

    for entry in ranking:
        if not entry['qualified']:
            continue
        wait = entry['wait_ms']
        ratios = []
        for rd in (1, 2, 3):
            round_ratios = []
            for cohort in PROFILES:
                point, anchor = indexed[wait, cohort, rd], indexed[20, cohort, rd]
                throughput_ratio = point['completed_s'] / anchor['completed_s']
                cpu_ratio = point['db_cpu_ms_per_completed'] / anchor['db_cpu_ms_per_completed']
                comparison = dict(case_set=cohort, round=rd,
                                  throughput_ratio=throughput_ratio if finite(throughput_ratio, True) else None,
                                  cpu_per_completed_unit_ratio=cpu_ratio if finite(cpu_ratio, True) else None)
                entry['comparisons'].append(comparison)
                if not finite(throughput_ratio, True) or not .95 <= throughput_ratio <= 1.05:
                    entry['reasons'].append(cohort + ' round ' + str(rd) + ': completed throughput differs by more than 5%')
                if not finite(cpu_ratio, True) or cpu_ratio > 1.05:
                    entry['reasons'].append(cohort + ' round ' + str(rd) + ': individual CPU ratio exceeds 1.05 or is unavailable')
                round_ratios.append(cpu_ratio)
            if not all(finite(value, True) for value in round_ratios):
                continue
            score = geometric_mean(round_ratios)
            entry['per_round_cpu_ratios'].append(dict(round=rd, geometric_mean=score))
            ratios.extend(round_ratios)
            if wait != 20 and score >= .95:
                entry['reasons'].append('round ' + str(rd) + ': CPU reduction does not exceed 5%')
        if len(ratios) == 6:
            entry['cpu_score'] = geometric_mean(ratios)
        entry['eligible'] = not entry['reasons'] and entry['cpu_score'] is not None

    eligible = [entry for entry in ranking if entry['eligible']]
    best = min(entry['cpu_score'] for entry in eligible)
    plateau = [entry['wait_ms'] for entry in eligible if entry['cpu_score'] <= best * 1.05]
    result.update(outcome='selected', selected_wait_ms=min(plateau),
                  best_cpu_score=best, cpu_plateau_waits_ms=plateau)
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('analysis', type=Path)
    args = parser.parse_args()
    raw = args.analysis.read_bytes()
    choice = choose_wait(json.loads(raw)['rows'])
    choice['analysis_sha256'] = hashlib.sha256(raw).hexdigest()
    choice['selector_sha256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    print(json.dumps(choice, indent=2, allow_nan=False))
