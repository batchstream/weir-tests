#!/usr/bin/env python3
"""Report the twelve preregistered independent-confirmation read guards."""
import argparse
import hashlib
import json
import math
from pathlib import Path


def read_guard(options):
    backend, workers, rd, rows = (options[key] for key in ('backend', 'workers', 'round', 'rows'))
    case_set = 'confirm-' + backend + '-read'
    errors, measurements, values = [], {}, {}
    for variant in ('default', 'legacy'):
        case_id = variant + '-' + backend + '-read-q1'
        for path, name in (('weir', variant + '_weir'), ('direct', variant + '_native')):
            matches = [row for row in rows if row.get('case_set') == case_set
                       and row.get('case_id') == case_id and row.get('path') == path
                       and row.get('workers') == workers and row.get('round') == rd]
            observations = []
            for row in matches:
                observation = dict(label=row.get('label'), case_id=case_id, path=path,
                                   usable=row.get('usable') is True,
                                   qualification_errors=row.get('qualification_errors'), error=row.get('error'))
                for field in ('completed_s', 'p99_ms'):
                    value = row.get(field)
                    valid = type(value) in (int, float) and math.isfinite(value) and (value > 0 if field == 'completed_s' else value >= 0)
                    observation[field] = value if valid else None
                    if not valid:
                        errors.append(name + '.' + field + ' must be finite and ' + ('positive' if field == 'completed_s' else 'nonnegative'))
                if row.get('phase') != 'confirmation' or row.get('kind') != 'write':
                    errors.append(name + ': expected confirmation write row')
                if row.get('usable') is not True:
                    errors.append(name + ': usable is not true')
                observations.append(observation)
            measurements[name] = dict(count=len(matches), observations=observations)
            if len(matches) != 1:
                errors.append(name + ': expected one row, found ' + str(len(matches)))
            else:
                values[name] = observations[0]
    numeric = dict(default_native_qps_ratio=None, legacy_native_qps_ratio=None,
                   normalized_qps_ratio=None, p99_limit_ms=None)
    throughput_passed, latency_passed = None, None
    if not errors:
        labels = [value['label'] for value in values.values()]
        if not all(isinstance(label, str) and label for label in labels) or len(set(labels)) != 1:
            errors.append('the four rows must identify one confirmation workflow')
        else:
            default_ratio = values['default_weir']['completed_s'] / values['default_native']['completed_s']
            legacy_ratio = values['legacy_weir']['completed_s'] / values['legacy_native']['completed_s']
            normalized = default_ratio / legacy_ratio if legacy_ratio > 0 else math.inf
            limit = max(values['legacy_weir']['p99_ms'] * 1.25, values['legacy_weir']['p99_ms'] + 2)
            if not all(math.isfinite(value) for value in (default_ratio, legacy_ratio, normalized, limit)):
                errors.append('read guard arithmetic is not finite')
            else:
                numeric.update(default_native_qps_ratio=default_ratio, legacy_native_qps_ratio=legacy_ratio,
                               normalized_qps_ratio=normalized, p99_limit_ms=limit)
                throughput_passed, latency_passed = normalized >= .90, values['default_weir']['p99_ms'] <= limit
                if not throughput_passed:
                    errors.append('normalized QPS ratio is below 0.90')
                if not latency_passed:
                    errors.append('default p99 upper bound exceeds max(legacy p99 * 1.25, legacy p99 + 2ms)')
    result = dict(backend=backend, case_set=case_set, workers=workers, round=rd,
                  measurements=measurements, values=numeric, throughput_passed=throughput_passed,
                  latency_passed=latency_passed, passed=not errors, errors=errors)
    return result


def build_report(analysis):
    input_errors = []
    if not isinstance(analysis, dict):
        analysis = {}
        input_errors.append('analysis must be an object')
    if type(analysis.get('schema')) is not int or analysis['schema'] != 1:
        input_errors.append('analysis schema must be 1')
    if analysis.get('complete_inventory') is not True:
        input_errors.append('analysis complete_inventory must be true')
    rows = analysis.get('rows')
    if not isinstance(rows, list):
        input_errors.append('analysis rows must be an array')
        rows = []
    if any(not isinstance(row, dict) for row in rows):
        input_errors.append('every analysis row must be an object')
        rows = [row for row in rows if isinstance(row, dict)]
    guards = []
    for backend in ('search', 'mongo'):
        for workers in (4, 128):
            for rd in (1, 2, 3):
                options = dict(backend=backend, workers=workers, round=rd, rows=rows)
                guards.append(read_guard(options))
    formulas = dict(throughput='(default_weir_qps / default_native_qps) / (legacy_weir_qps / legacy_native_qps) >= 0.90',
                    latency='default_weir_p99_upper_ms <= max(legacy_weir_p99_upper_ms * 1.25, legacy_weir_p99_upper_ms + 2)')
    result = dict(schema=1, purpose='Independent confirmation of the frozen read guards; no selection',
                  formulas=formulas,
                  guards=guards, input_errors=input_errors,
                  passed=not input_errors and all(guard['passed'] for guard in guards))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--analysis', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    for path in (args.analysis, args.output):
        if path.suffix != '.json' or path.name.startswith('.env') or path.is_symlink():
            raise ValueError('ordinary non-secret JSON paths required')
    raw = args.analysis.read_bytes()
    report = build_report(json.loads(raw))
    report['analysis_sha256'] = hashlib.sha256(raw).hexdigest()
    report['report_script_sha256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    with args.output.open('x') as output:
        output.write(json.dumps(report, indent=2, allow_nan=False) + '\n')
    summary = dict(output=str(args.output), guards=len(report['guards']), passed=report['passed'])
    print(json.dumps(summary))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
