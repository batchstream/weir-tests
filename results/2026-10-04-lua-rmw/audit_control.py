import json
import pathlib
import runpy

archive = pathlib.Path(__file__).resolve().parent
helpers = runpy.run_path(str(archive.parent / '2026-10-04-single-request' / 'reviewer_audit.py'))
root = archive / 'control'
summary = {
    'manual_run': 37172507455,
    'test_source': '9d9e4ce1610881330049b69c4a0de7d872003af5',
    'server_source': '514deab214dd801dd4fc95c5184435b6a9a17a3d',
    'reports': [],
    'cleanup': [],
}
for file in sorted(root.rglob('mongo.json')) + sorted(root.rglob('search.json')):
    report = json.loads(file.read_text())
    provenance = report['provenance']
    limit = int(provenance['weir_max_batch_operations'])
    backend = report['parameters']['dataset']['backend']
    assert provenance['test_source'] == summary['test_source']
    assert provenance['weir_source'] == summary['server_source']
    assert provenance['test_source_dirty'] == 'false'
    assert provenance['sdk'] == 'v0.4.2' and provenance['protocol'] == 'v0.2.1'
    assert report['parameters']['dataset']['lua_mutations'] is True
    assert report['parameters']['records_per_business_request'] == 1
    assert report['parameters']['client_process_count'] == 4
    assert report['database_cpu_quota']['observed_cpus'] == 1
    row = {
        'backend': backend, 'batch_limit': limit, 'stages': 0,
        'successful_operations': 0, 'points': report['points'],
        'saturation': report['database_saturated_comparisons'],
        'aggregation': [], 'native_global_update_counter_excess': 0,
    }
    for pair in report['pairs']:
        options = {
            'concurrency': pair['concurrency'], 'backend': backend,
            'limit': limit, 'records': 2048, 'threshold': 90,
        }
        for path in ('direct', 'weir'):
            stage = pair[path]
            helpers['check_stage'](stage, options)
            row['stages'] += 1
            row['successful_operations'] += stage['succeeded']
            if backend == 'mongo':
                commands = stage['timed_physical_database_command_deltas']
                before = stage['timed_database_counters_before']['physical_command_totals']
                after = stage['timed_database_counters_after']['physical_command_totals']
                assert commands['abortTransaction'] == after['abortTransaction'] - before['abortTransaction'] == 0
                assert commands['commitTransaction'] == after['commitTransaction'] - before['commitTransaction']
                assert commands['find'] == commands['commitTransaction'] > 0
                if path == 'direct':
                    assert commands['find'] == stage['succeeded']
                    assert commands['update'] >= stage['succeeded'] and commands['bulkWrite'] == 0
                    row['native_global_update_counter_excess'] += commands['update'] - stage['succeeded']
                else:
                    assert commands['find'] == commands['bulkWrite'] and commands['update'] == 0
            if path == 'weir' and pair['concurrency'] == 128:
                metrics = stage['server_metrics']
                counters = metrics['timed_counter_deltas']
                aggregation = {
                    'round': pair['round'], 'records': stage['succeeded'],
                    'invocations': counters['weir_store_batch_operations_count'],
                    'physical_commands': stage.get('timed_physical_database_command_deltas'),
                    'batch_average': metrics['timed_adapter_batch_average'],
                }
                row['aggregation'].append(aggregation)
    assert row['stages'] == 18
    summary['reports'].append(row)
for file in sorted(root.rglob('cleanup.json')):
    receipt = json.loads(file.read_text())
    assert not receipt['errors']
    assert all(item['removed'] for item in receipt['containers'])
    assert all(item['stopped'] for item in receipt['processes'])
    summary['cleanup'].append(receipt)
assert len(summary['cleanup']) == 2
summary['timed_stages'] = sum(row['stages'] for row in summary['reports'])
summary['successful_operations'] = sum(row['successful_operations'] for row in summary['reports'])
summary['status'] = 'PASS'
pathlib.Path('/tmp/weir-lua-root-control-recomputed.json').write_text(json.dumps(summary, indent=2) + '\n')
print('PASS', summary['timed_stages'], 'timed stages', summary['successful_operations'], 'successful operations, 2 fixtures cleaned')
