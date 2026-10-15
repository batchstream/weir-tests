"""Prospective capacity gates, including completion/drain stability and headroom."""
import math

MAX_COUNT = (1 << 64) - 1

WIDTH_NS = 10_000_000_000
COUNTERS = ('offered', 'rejected', 'completed', 'failed', 'unknown', 'timed_out')


def require(condition, message):
    if not condition:
        raise ValueError(message)


def finite(value, positive=False):
    return type(value) in (int, float) and math.isfinite(value) and (value > 0 if positive else value >= 0)


def merge_buckets(options):
    duration, pools, histogram = options['duration'], options['pools'], options['histogram']
    require(type(duration) is int and 0 < duration <= 600_000_000_000, 'invalid phase duration')
    require(set(options['totals']) == set(COUNTERS), 'all six stage counters required')
    count = (duration + WIDTH_NS - 1) // WIDTH_NS
    merged = []
    for index in range(count + 1):
        bucket = dict(offset_ns=min(index * WIDTH_NS, duration), width_ns=min(WIDTH_NS, duration - index * WIDTH_NS) if index < count else 0, drain=index == count, latency_histogram_buckets=[0] * 256, max_ns=0)
        bucket.update(dict.fromkeys(COUNTERS, 0))
        merged.append(bucket)
    if not isinstance(pools, list) or not pools or any(not isinstance(pool, list) or len(pool) != count + 1 for pool in pools):
        raise ValueError('missing completion/drain buckets')
    for pool in pools:
        client_totals = dict.fromkeys(COUNTERS, 0)
        for index, bucket in enumerate(pool):
            target = merged[index]
            if type(bucket['offset_ns']) is not int or type(bucket['width_ns']) is not int or type(bucket['drain']) is not bool or any(bucket[key] != target[key] for key in ('offset_ns', 'width_ns', 'drain')) or type(bucket['invalid_offsets']) is not int or bucket['invalid_offsets'] != 0:
                raise ValueError('bucket layout or offset differs')
            if any(type(bucket[key]) is not int or not 0 <= bucket[key] <= MAX_COUNT for key in COUNTERS) or bucket['unknown'] > bucket['failed'] or bucket['timed_out'] > bucket['failed']:
                raise ValueError('invalid bucket status counter')
            counts, maximum = bucket['latency_histogram_buckets'], bucket['latency']['max_observed_ns']
            require(isinstance(counts, list) and len(counts) == 256 and all(type(value) is int and 0 <= value <= MAX_COUNT for value in counts) and sum(counts) <= MAX_COUNT and type(maximum) is int and maximum >= 0 and (sum(counts) > 0 or maximum == 0), 'invalid bucket histogram/max')
            observed = histogram(counts, maximum)
            if any(bucket['latency'][key] != value for key, value in observed.items()) or observed['samples'] != bucket['completed'] + bucket['failed']:
                raise ValueError('bucket histogram/status mismatch')
            for key in COUNTERS:
                target[key] += bucket[key]
                client_totals[key] += bucket[key]
                require(target[key] <= MAX_COUNT and client_totals[key] <= MAX_COUNT, 'bucket total overflow')
            target['max_ns'] = max(target['max_ns'], bucket['latency']['max_observed_ns'])
            target['latency_histogram_buckets'] = [a + b for a, b in zip(target['latency_histogram_buckets'], bucket['latency_histogram_buckets'])]
        require(client_totals['offered'] == client_totals['completed'] + client_totals['failed'] + client_totals['rejected'], 'client/worker arrivals do not conserve')
    totals = {key: sum(bucket[key] for bucket in merged) for key in COUNTERS}
    for key, expected in options['totals'].items():
        if type(expected) is not int or not 0 <= expected <= MAX_COUNT or totals[key] != expected:
            raise ValueError('bucket total differs from stage: ' + key)
    pooled, maximum = [0] * 256, 0
    for bucket in merged:
        bucket['latency'] = histogram(bucket['latency_histogram_buckets'], bucket['max_ns'])
        pooled = [a + b for a, b in zip(pooled, bucket['latency_histogram_buckets'])]
        maximum = max(maximum, bucket['max_ns'])
    observed = histogram(pooled, maximum)
    if any(options['latency'][key] != value for key, value in observed.items()):
        raise ValueError('time buckets differ from stage latency')
    return merged


def stability(options):
    buckets, duration = options['buckets'], options['duration']
    require(type(duration) is int and 0 < duration <= 600_000_000_000, 'invalid stability duration')
    count = (duration + WIDTH_NS - 1) // WIDTH_NS
    require(isinstance(buckets, list) and len(buckets) == count + 1, 'missing regular or drain stability bucket')
    for index, bucket in enumerate(buckets):
        offset = min(index * WIDTH_NS, duration)
        require(bucket['offset_ns'] == offset and bucket['width_ns'] == (min(WIDTH_NS, duration-offset) if index < count else 0) and bucket['drain'] == (index == count), 'stability bucket layout differs')
    regular = [bucket for bucket in buckets if not bucket['drain']]
    halves = []
    for left, right in ((0, duration // 2), (duration // 2, duration)):
        span = sum(max(0, min(right, b['offset_ns'] + b['width_ns']) - max(left, b['offset_ns'])) for b in regular)
        completed = sum(b['completed'] for b in regular if left <= b['offset_ns'] < right)
        halves.append(completed * 1e9 / span if span else None)
    exact_halves = duration % (2 * WIDTH_NS) == 0
    if not exact_halves:
        halves = [None, None]
    stable = bool(exact_halves and halves[0] and halves[1] >= halves[0] * .9)
    clean = not any(bucket[key] for bucket in buckets for key in ('rejected', 'failed', 'unknown', 'timed_out'))
    stable = stable and clean
    backlog, backlog_series = 0, []
    if options.get('rate'):
        require(finite(options['rate'], True) and buckets[-1]['offered'] == buckets[-1]['rejected'] == 0, 'invalid rate or arrivals after scheduled window')
        for bucket in regular:
            backlog += bucket['offered'] - bucket['completed'] - bucket['failed'] - bucket['rejected']
            if backlog < 0:
                raise ValueError('completion-time backlog is negative')
            backlog_series.append(backlog)
        third = max(0, len(regular) // 3 - 1)
        stable = stable and backlog_series[-1] <= backlog_series[third] + options['rate']
    minutes = []
    if duration >= 300_000_000_000:
        require(duration % 60_000_000_000 == 0, 'sustained check requires whole minutes')
        for start in range(0, duration, 60_000_000_000):
            pieces = [b for b in regular if start <= b['offset_ns'] < start + 60_000_000_000]
            span = sum(b['width_ns'] for b in pieces)
            pooled = [sum(b['latency_histogram_buckets'][i] for b in pieces) for i in range(256)]
            observed = options['histogram'](pooled, max((b['max_ns'] for b in pieces), default=0))
            minute = dict(start_ns=start, completed_s=sum(b['completed'] for b in pieces) * 1e9 / span, p99_ms=observed['p99_upper_bound_ns'] / 1e6)
            minutes.append(minute)
        for before, after in zip(minutes, minutes[1:]):
            stable = stable and after['completed_s'] >= before['completed_s'] * .9
        stable = stable and all(minute['completed_s'] > 0 and minute['p99_ms'] <= 5000 for minute in minutes)
        stable = stable and minutes[-1]['completed_s'] >= minutes[0]['completed_s'] * .9
    result = dict(rate_stable=bool(stable), first_half_completed_s=halves[0], second_half_completed_s=halves[1], backlog_by_regular_bucket=backlog_series, minutes=minutes, final_first_minute_ratio=minutes[-1]['completed_s']/minutes[0]['completed_s'] if minutes and minutes[0]['completed_s'] else None, clean_status_counts=clean)
    return result


def evidence_errors(row):
    reasons = []
    if not row.get('checks_passed') or not row.get('verified') or not finite(row.get('completed'), True) or not finite(row.get('completed_s'), True):
        reasons.append('source/measurement/postflight unavailable')
    if not finite(row.get('db_cpu_coverage')) or row['db_cpu_coverage'] < .95:
        reasons.append('confirmation CPU coverage below 95%')
    if any(not finite(row.get(key)) or row[key] != 0 for key in ('failed', 'unknown', 'timeout')):
        reasons.append('failed/unknown/timed-out operations')
    return reasons


def headroom_errors(row):
    reasons = []
    if not row.get('host_headroom') or not finite(row.get('host_coverage')) or row['host_coverage'] < .9 or not finite(row.get('host_busy_percent')) or row['host_busy_percent'] > 85 or not finite(row.get('host_steal_percent')) or row['host_steal_percent'] > 5 or not finite(row.get('effective_host_cpus'), True):
        reasons.append('host headroom unavailable or exceeded')
    clients = row.get('client_process_cpu')
    effective = row.get('effective_host_cpus')
    effective = effective if finite(effective, True) else 0
    if not row.get('client_headroom') or not isinstance(clients, list) or len(clients) != 4 or len({client.get('pid') for client in clients}) != 4 or any(not finite(client.get('coverage')) or client['coverage'] < .9 or not finite(client.get('cpu_cores')) or client['cpu_cores'] >= effective * .8 for client in clients):
        reasons.append('four owned clients lack demonstrated CPU headroom')
    return reasons


def cpu_errors(row):
    reasons = []
    if not row.get('sustained_database_cpu_saturation') or not finite(row.get('db_cpu_quota_percent')) or row['db_cpu_quota_percent'] < 90 or not finite(row.get('fraction_measurement_at_90_percent_cpu')) or row['fraction_measurement_at_90_percent_cpu'] < .8 or len(row.get('cpu_intervals', [])) < 5:
        reasons.append('sustained 90% database CPU pressure unavailable')
    return reasons


def group(rows, rates=False):
    require(rows, 'empty formal workload group')
    fields = ['label', 'case_set', 'kind', 'profile', 'database_cpus', 'phase']
    if not rates:
        fields.append('case_id')
    require(all(len({row.get(key) for row in rows}) == 1 for key in fields), 'mixed workload, phase or database quota')
    require(all(row.get('phase') not in (None, 'selection') and finite(row.get('database_cpus'), True) for row in rows), 'capacity requires formal data and measured quota')


def capacity(options):
    rows, levels = options['rows'], options['levels']
    group(rows)
    require(len(levels) >= 2 and all(type(value) is int and value > 0 for value in levels) and levels == sorted(set(levels)), 'invalid declared concurrency ladder')
    indexed = {}
    for row in rows:
        key = row['path'], row['workers'], row['round']
        require(row['path'] in ('direct', 'weir') and row['workers'] in levels and type(row['round']) is int and row['round'] in (1, 2, 3) and key not in indexed, 'unexpected or duplicate capacity point')
        indexed[key] = row
    output = []
    for path in ('direct', 'weir'):
        points = []
        for index, workers in enumerate(levels):
            rounds = [indexed.get((path, workers, rd)) for rd in (1, 2, 3)]
            following = [indexed.get((path, levels[index + 1], rd)) for rd in (1, 2, 3)] if index + 1 < len(levels) else [None] * 3
            decisions = []
            for rd, (first, last) in enumerate(zip(rounds, following), 1):
                reasons = []
                for name, row in (('point', first), ('following', last)):
                    if row is None:
                        reasons.append(name + ' missing')
                    else:
                        reasons.extend(name + ': ' + reason for reason in evidence_errors(row) + headroom_errors(row) + cpu_errors(row))
                        if not row.get('usable') or not row.get('rate_stable'):
                            reasons.append(name + ': unqualified or unstable')
                if not reasons and last['completed_s'] > first['completed_s'] * 1.1:
                    reasons.append('next-concurrency gain exceeds 10%')
                decisions.append(dict(round=rd, passed=not reasons, reasons=reasons))
            points.append(dict(workers=workers, capacity_qualified=all(item['passed'] for item in decisions), rounds=rounds, following=following, decisions=decisions))
        chosen = next((point for point in points if point['capacity_qualified']), None)
        output.append(dict(path=path, selected_workers=chosen['workers'] if chosen else None, capacity_qualified=bool(chosen), points=points))
    result = dict(database_cpus=rows[0]['database_cpus'], paths=output, both_qualified=all(path['capacity_qualified'] for path in output), comparable_actual_load_higher_capacity=False)
    if result['both_qualified']:
        sides = [next(point for point in item['points'] if point['workers'] == item['selected_workers'])['rounds'] for item in output]
        paired = [dict(round=index + 1, qps_ratio=last['completed_s'] / first['completed_s'], actual_cpu_ratio=last['db_cpu_quota_percent'] / first['db_cpu_quota_percent']) for index, (first, last) in enumerate(zip(*sides))]
        result['paired_rounds'] = paired
        result['comparable_actual_load_higher_capacity'] = all(pair['qps_ratio'] > 1 and .95 <= pair['actual_cpu_ratio'] <= 1.05 for pair in paired)
    return result


def crawler_evidence_errors(row):
    reasons = evidence_errors(row) + headroom_errors(row)
    if not finite(row.get('max_generator_lag_ms')) or row['max_generator_lag_ms'] > 50:
        reasons.append('generator lag exceeds 50ms or unavailable')
    if any(type(row.get(key)) is not int or row[key] < 0 for key in ('offered', 'rejected', 'completed')) or row.get('offered') != row.get('completed', 0) + row.get('failed', 0) + row.get('rejected', 0):
        reasons.append('whole-stage arrival counts do not conserve')
    return reasons


def crawler_rate_bounds(options):
    rows, rates = options['rows'], options['rates']
    group(rows, True)
    paths = options.get('paths', ['direct', 'optimized'])
    require(all(row['kind'] == 'crawler' for row in rows) and rates and all(type(value) is int and value > 0 for value in rates) and rates == sorted(set(rates)) and len(set(paths)) == len(paths), 'invalid declared crawler rate ladder')
    indexed = {}
    for row in rows:
        key = row['path'], row['rate'], row['round']
        require(row['path'] in paths and row['rate'] in rates and type(row['round']) is int and row['round'] in (1, 2, 3) and key not in indexed, 'unexpected or duplicate crawler rate point')
        indexed[key] = row
    outputs = []
    for path in paths:
        points = []
        for rate in rates:
            rounds = []
            for rd in (1, 2, 3):
                row = indexed.get((path, rate, rd))
                errors = ['missing declared round'] if row is None else crawler_evidence_errors(row)
                slo = []
                if row is not None:
                    if row.get('offered') != row.get('completed') or row.get('rejected') != 0:
                        slo.append('not all arrivals completed')
                    if not finite(row.get('p99_ms')) or row['p99_ms'] > 5000:
                        slo.append('p99 exceeds 5 seconds or unavailable')
                    if not finite(row.get('drain_s')) or row['drain_s'] > 3:
                        slo.append('drain exceeds 3 seconds or unavailable')
                    if not row.get('rate_stable'):
                        slo.append('completion/backlog unstable')
                passed = not errors and not slo and row.get('usable')
                overloaded = not errors and bool(slo) and finite(row.get('p99_ms')) and finite(row.get('drain_s')) and type(row.get('rate_stable')) is bool
                rounds.append(dict(round=rd, passed=bool(passed), service_overload=bool(overloaded), evidence_errors=errors, slo_errors=slo))
            points.append(dict(rate=rate, qualified=all(item['passed'] for item in rounds), service_overload=all(item['service_overload'] for item in rounds), rounds=rounds))
        good = [point for point in points if point['qualified']]
        lower = good[-1] if good else None
        next_index = rates.index(lower['rate']) + 1 if lower else len(rates)
        following = points[next_index] if next_index < len(points) else None
        upper = following if following and following['service_overload'] else None
        outputs.append(dict(path=path, lower_rate=lower['rate'] if lower else None, upper_rate=upper['rate'] if upper else None, bounded=bool(lower and upper), soak_eligible=path != 'direct' and bool(lower and upper), points=points))
    result = dict(database_cpus=rows[0]['database_cpus'], profile=rows[0].get('profile'), paths=outputs)
    return result


def soak(options):
    row, kind = options['row'], options['kind']
    require(kind in ('write', 'crawler') and row['kind'] == kind and row.get('phase') not in (None, 'selection'), 'invalid sustained-check workload/phase')
    arguments = dict(buckets=row['time_buckets'], duration=300_000_000_000, histogram=options['histogram'])
    if kind == 'crawler':
        arguments['rate'] = row['rate']
    observed = stability(arguments)
    reasons = evidence_errors(row) + headroom_errors(row)
    if not row.get('usable') or not finite(row.get('elapsed_s')) or row['elapsed_s'] < 300 or not finite(row.get('p99_ms')) or row['p99_ms'] > 5000:
        reasons.append('five-minute measurement or p99 gate failed')
    if kind == 'write':
        reasons.extend(cpu_errors(row))
    else:
        reasons.extend(crawler_evidence_errors(row))
        if row.get('offered') != row.get('completed') or row.get('rejected') != 0 or not finite(row.get('drain_s')) or row['drain_s'] > 3:
            reasons.append('five-minute arrivals/drain gate failed')
    if not observed['rate_stable']:
        reasons.append('five-minute completion/backlog/latency instability')
    result = dict(passed=not reasons, errors=list(dict.fromkeys(reasons)), stability=observed)
    return result
