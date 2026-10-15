"""Prospective deterministic selection; raw auditing produces the input rows."""
import math

WRITE_SETS = ('select-pure', 'select-partial', 'select-crawler')
READ_SETS = ('select-search-read', 'select-mongo-read')
LEGACY = 'c32-w0s-s0'


def choose(options):
    rows, plan = options['rows'], options['plan']
    indexed = {}
    for row in rows:
        if row['path'] not in ('weir', 'optimized'):
            continue
        key = row['case_set'], row['case_id'], row['workers']
        if key in indexed:
            raise ValueError('selection needs exactly one measured round per point')
        indexed[key] = row
    ranked = []
    for case in plan['case_sets']['select-pure']:
        entry = dict(id=case['id'], controls=case['batching'], eligible=True, reasons=[])
        points = [indexed.get((name, case['id'], 512)) for name in WRITE_SETS]
        if case['batching']['max_in_flight'] == 0:
            entry['reasons'].append('unlimited legacy execution control is not a protective default')
        if any(not point or not point['usable'] for point in points):
            entry['reasons'].append('missing or unqualified write profile')
        guards = []
        for name in READ_SETS:
            for workers in (4, 128):
                point, legacy = indexed.get((name, case['id'], workers)), indexed.get((name, LEGACY, workers))
                direct = next((r for r in rows if r['case_set'] == name and r['case_id'] == case['id'] and r['workers'] == workers and r['path'] == 'direct'), None)
                old_direct = next((r for r in rows if r['case_set'] == name and r['case_id'] == LEGACY and r['workers'] == workers and r['path'] == 'direct'), None)
                guard = dict(case_set=name, workers=workers, passed=False, normalized_qps=None, p99_limit_ms=None)
                if all(r and r['usable'] for r in (point, legacy, direct, old_direct)):
                    normalized = (point['completed_s'] / direct['completed_s']) / (legacy['completed_s'] / old_direct['completed_s'])
                    limit = max(legacy['p99_ms'] * 1.25, legacy['p99_ms'] + 2)
                    guard.update(passed=normalized >= .9 and point['p99_ms'] <= limit, normalized_qps=normalized, p99_limit_ms=limit)
                guards.append(guard)
        if not all(g['passed'] for g in guards):
            entry['reasons'].append('Search/Mongo read regression guard failed or unavailable')
        entry.update(eligible=not entry['reasons'], read_guards=guards)
        entry['points'] = {name: point for name, point in zip(WRITE_SETS, points)}
        ranked.append(entry)
    eligible = [entry for entry in ranked if entry['eligible']]
    if not eligible:
        result = dict(selected=None, ranking=ranked, error='no candidate meets the prospectively declared constraints')
        return result
    maxima = {name: max(entry['points'][name]['completed_s'] for entry in eligible) for name in WRITE_SETS}
    minima = {name: min(entry['points'][name]['db_cpu_ms_per_completed'] for entry in eligible) for name in WRITE_SETS}
    for entry in eligible:
        entry['throughput_score'] = min(entry['points'][name]['completed_s'] / maxima[name] for name in WRITE_SETS)
        entry['cpu_score'] = math.prod(entry['points'][name]['db_cpu_ms_per_completed'] / minima[name] for name in WRITE_SETS) ** (1 / 3)
    maximum = max(entry['throughput_score'] for entry in eligible)
    frontier = [entry for entry in eligible if entry['throughput_score'] >= maximum * .95]
    minimum = min(entry['cpu_score'] for entry in frontier)
    plateau = [entry for entry in frontier if entry['cpu_score'] <= minimum * 1.05]
    plateau.sort(key=lambda entry: (entry['points']['select-pure']['wait_ns'], entry['controls']['max_operations'], entry['controls']['max_in_flight'], entry['id']))
    result = dict(selected=dict(plateau[0]['controls']), selected_id=plateau[0]['id'], ranking=ranked, throughput_frontier=[entry['id'] for entry in frontier], cpu_plateau=[entry['id'] for entry in plateau], error=None)
    return result


def choose_bytes(rows):
    eligible = [row for row in rows if row['case_set'] == 'select-bytes' and row['path'] == 'weir' and row['usable']]
    if not eligible:
        result = dict(selected=None, error='no usable byte calibration', points=rows)
        return result
    maximum = max(row['completed_s'] for row in eligible)
    minimum = min(row['db_cpu_ms_per_completed'] for row in eligible)
    joint = [row for row in eligible if row['completed_s'] >= maximum * .95 and row['db_cpu_ms_per_completed'] <= minimum * 1.05]
    for row in eligible:
        row['byte_score'] = min(row['completed_s'] / maximum, minimum / row['db_cpu_ms_per_completed'])
    if not joint:
        best = max(row['byte_score'] for row in eligible)
        joint = [row for row in eligible if row['byte_score'] >= best * .95]
    selected = min(joint, key=lambda row: row['max_bytes'])
    binding = []
    for point in eligible:
        witnesses = []
        for higher in eligible:
            if higher['max_bytes'] <= point['max_bytes']:
                continue
            bucket = [item for item in higher.get('http_body_buckets', []) if float(item['labels']['le']) == point['max_bytes']]
            attempts = higher.get('search_bulk_attempts')
            if len(bucket) == 1 and attempts and bucket[0]['delta'] is not None:
                witnesses.append(dict(higher_cap=higher['max_bytes'], requests_over_target=attempts - bucket[0]['delta'], attempts=attempts))
        binding.append(dict(max_bytes=point['max_bytes'], binding_demonstrated=any(item['requests_over_target'] > 0 for item in witnesses), witnesses=witnesses))
    result = dict(selected=selected['max_bytes'], points=eligible, binding=binding, error=None)
    return result
