import copy
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import MagicMock, patch

from run_crawler import planned_command as crawler_command
from run_write_batching import planned_command as write_command
from run_tuning_plan import audit_live_batching, duration_ns, run_cases, validate_case


def write_case():
    return dict(id='count128', kind='write', backend='search', payload_bytes=1024,
                records=8192, levels=[128], write_percent=100, client_processes=4,
                database_cpus=1, rounds=1, warmup='10s', duration='20s',
                batching=dict(max_operations=128, max_bytes=8388608, max_wait='5ms', max_in_flight=16))


def crawler_case():
    case = write_case()
    for key in ('backend', 'payload_bytes', 'records', 'levels', 'write_percent'):
        del case[key]
    case.update(kind='crawler', profile='partial-update', rate=512, concurrency=512,
                paths=['direct', 'optimized'], visibility='application_acknowledged',
                seed_padding_bytes=16384, http_observer='disabled')
    return case


class TuningPlanTest(unittest.TestCase):
    def test_explicit_zero_controls_and_omitted_defaults_have_different_commands(self):
        case = write_case()
        case['batching']['max_wait'], case['batching']['max_in_flight'] = '0s', 0
        command = write_command(case, Path('measurement'))
        self.assertEqual(command[command.index('-backend-batch-wait') + 1], '0s')
        self.assertEqual(command[command.index('-backend-max-in-flight') + 1], '0')
        case['expected_batching'], case['batching'] = case['batching'], None
        self.assertEqual(validate_case(case), 2)
        command = write_command(case, Path('measurement'))
        self.assertNotIn('-backend-batch-limit', command)
        self.assertNotIn('-backend-max-in-flight', command)
        del case['expected_batching']
        with self.assertRaises(ValueError):
            validate_case(case)

    def test_crawler_plan_reuses_engine_and_preserves_two_path_rotation(self):
        case = crawler_case()
        self.assertEqual(validate_case(case), 2)
        command = crawler_command(case, Path('crawler.json'))
        self.assertEqual(command[command.index('-paths') + 1], 'direct,optimized')
        self.assertNotIn('-after-use-defaults', command)
        case['expected_batching'], case['batching'] = case['batching'], None
        self.assertEqual(validate_case(case), 2)
        command = crawler_command(case, Path('crawler.json'))
        self.assertIn('-after-use-defaults', command)
        self.assertIn('-capture-effective-config', command)

    def test_invalid_load_matrix_fails_before_external_work(self):
        for field, invalid in [('levels', [128, 128]), ('levels', [1028]), ('levels', [3]),
                               ('records', 100), ('payload_bytes', 1 << 20), ('rounds', True),
                               ('duration', '0s'), ('warmup', '61s'), ('client_processes', 9)]:
            case = write_case()
            case[field] = invalid
            with self.subTest(field=field, invalid=invalid), self.assertRaises(ValueError):
                validate_case(case)
        case = write_case()
        case['levels'] = [1024]
        self.assertEqual(validate_case(case), 2)
        case = crawler_case()
        case['paths'] = ['direct', 'direct']
        with self.assertRaises(ValueError):
            validate_case(case)

    def test_all_four_controls_are_required_and_unknown_controls_rejected(self):
        for name in ('max_operations', 'max_bytes', 'max_wait', 'max_in_flight'):
            case = write_case()
            del case['batching'][name]
            with self.assertRaises(ValueError):
                validate_case(case)
        case = write_case()
        case['hidden_knob'] = True
        with self.assertRaises(ValueError):
            validate_case(case)
        case = write_case()
        case['batching']['max_wait'] = '1.1s'
        with self.assertRaises(ValueError):
            validate_case(case)
        self.assertEqual(duration_ns('0.5ms'), 500000)

    def test_live_controls_never_treat_missing_or_duplicate_gauges_as_zero(self):
        case = write_case()
        values = dict(weir_store_batch_max_operations=128, weir_store_batch_max_bytes=8388608,
                      weir_store_batch_max_wait_seconds=.005, weir_store_active_executions_limit=16)
        snapshot = dict(metrics=[dict(name=name, value=value, labels=dict(store='search')) for name, value in values.items()])
        configured = dict(case['batching'], max_bytes=str(case['batching']['max_bytes']) + 'B')
        routes = dict(stores=[dict(name='search', batching=configured)])
        report = dict(parameters=dict(dataset=dict(backend='search')), provenance=dict(weir_routing_config=json.dumps(routes)),
                      pairs=[dict(weir=dict(server_metrics=dict(before=snapshot, after=copy.deepcopy(snapshot))))])
        audit_live_batching(report, case)
        report['pairs'][0]['weir']['server_metrics']['after']['metrics'].pop()
        with self.assertRaises(ValueError):
            audit_live_batching(report, case)
        report['pairs'][0]['weir']['server_metrics']['after'] = copy.deepcopy(snapshot)
        report['pairs'][0]['weir']['server_metrics']['after']['metrics'].append(snapshot['metrics'][0])
        with self.assertRaises(ValueError):
            audit_live_batching(report, case)

    def test_failed_case_is_retained_and_does_not_drop_remaining_declared_cases(self):
        cases = [write_case(), write_case()]
        cases[0]['id'], cases[1]['id'] = 'overload', 'next-level'
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            server = root / 'server'
            client = root / 'client'
            server.write_bytes(b'immutable server')
            client.write_bytes(b'immutable client')
            context = dict(server=server, client=client, crawler=client)
            with patch('run_tuning_plan.subprocess.Popen') as popen:
                first, second = MagicMock(), MagicMock()
                first.stdout, second.stdout = io.StringIO('retained failure\n'), io.StringIO('retained failure\n')
                first.wait.return_value, second.wait.return_value = 1, 1
                popen.side_effect = [first, second]
                self.assertFalse(run_cases(cases, root, context))
            self.assertEqual(popen.call_count, 2)
            self.assertEqual(json.loads((root / 'outcomes.json').read_text()),
                             [dict(id='overload', passed=False), dict(id='next-level', passed=False)])
            for case in cases:
                receipt = json.loads((root / case['id'] / 'receipt.json').read_text())
                self.assertEqual(receipt['exit_code'], 1)
                self.assertEqual(receipt['validation'], 'failed')
                self.assertIn('retained failure', (root / case['id'] / 'execution.log').read_text())


if __name__ == '__main__':
    unittest.main()
