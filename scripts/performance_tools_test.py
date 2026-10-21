import copy
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

import run_performance_matrix as runner
import summarize_performance as summary


def make_report(schema=4):
    provenance = dict(weir_binary_sha256='a' * 64)
    commands = dict(find=100, update=0)
    latency = dict(p95_upper_bound_ns=5_000_000)
    resources = dict(samples=[])
    stage = dict(succeeded=100, elapsed_ns=1_000_000_000, successful_operations_per_second=100,
                 reads=100, writes=0, errors=0, indeterminate=0, business_operation_timeouts=0, verified=True,
                 latency=latency, resources=resources, timed_physical_database_command_deltas=commands)
    pair = dict(concurrency=8, batch_size=1, round=1, order='direct-first', direct=stage, weir=copy.deepcopy(stage))
    dataset = dict(backend='mongo', concurrency=8, namespace='owned', weir_seed='local')
    parameters = dict(dataset=dataset, concurrency_levels=[8], batch_sizes=[1], paired_rounds=1)
    report = dict(schema=schema, provenance=provenance, parameters=parameters, pairs=[pair])
    return report


class MatrixRunnerTests(unittest.TestCase):
    def run_matrix(self, arguments, returncode=0):
        commands = []

        def run_command(command, **kwargs):
            if command[0] == 'go':
                Path(command[command.index('-o') + 1]).write_bytes(b'client')
            else:
                commands.append(command)
            result = subprocess.CompletedProcess(command, returncode)
            return result

        argv = ['run_performance_matrix.py', *arguments]
        with mock.patch('sys.argv', argv), \
                mock.patch('sys.stdout', new_callable=io.StringIO), \
                mock.patch.object(runner.subprocess, 'run', side_effect=run_command):
            runner.main()
        return commands

    def test_cases_forward_matrix_parameters_to_weir_lab(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'matrix'
            arguments = ['--backend', 'mongo', '--weir', 'server', '--output', str(output),
                         '--database-cpus', '1', '--concurrency-levels', '4,16,64,256',
                         '--client-processes', '4', '--records', '2048', '--payload-bytes', '512', '--rounds', '5',
                         '--bulk', '--bulk-concurrency-levels', '4,16', '--mongod', 'mongod']
            commands = self.run_matrix(arguments)
            self.assertEqual(len(commands), 6)
            for command in commands:
                mode = command[command.index('-mode') + 1]
                self.assertEqual(command[command.index('-database-cpus') + 1], '1.0')
                self.assertEqual(command[command.index('-records') + 1], '2048')
                self.assertEqual(command[command.index('-payload-bytes') + 1], '512')
                self.assertEqual(command[command.index('-rounds') + 1], '5')
                self.assertEqual(command[command.index('-warmup-duration') + 1], '10s')
                self.assertEqual(command[command.index('-weir') + 1], str(Path('server').resolve()))
                self.assertEqual(command[command.index('-mongod') + 1], str(Path('mongod').resolve()))
                expected_levels = '4,16' if mode == 'bulk-saturation' else '4,16,64,256'
                self.assertEqual(command[command.index('-concurrency-levels') + 1], expected_levels)
                self.assertEqual(command[command.index('-batch-sizes') + 1], '32' if mode == 'bulk-saturation' else '1')
            lua = ['-lua-mutations' in command for command in commands]
            expected_lua = [False, False, False, True, False, False]
            self.assertEqual(lua, expected_lua)
            self.assertTrue((output / 'read.log').exists())

    def test_failed_case_stops_the_matrix(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'matrix'
            with self.assertRaises(SystemExit):
                arguments = ['--backend', 'search', '--output', str(output)]
                self.run_matrix(arguments, returncode=3)
            self.assertTrue((output / 'read.log').exists())
            self.assertFalse((output / 'mixed.log').exists())


class SummaryTests(unittest.TestCase):
    def write_matrices(self, root):
        before, after = root / 'before', root / 'after'
        for phase in (before, after):
            for case in ('read', 'mixed', 'write', 'lua'):
                (phase / case).mkdir(parents=True)
                (phase / case / 'mongo.json').write_text(json.dumps(make_report()))
        return before, after

    def summarize(self, before, after, output):
        arguments = ['summarize_performance.py', '--before', str(before), '--after', str(after),
                     '--backend', 'mongo', '--output', str(output)]
        with mock.patch('sys.argv', arguments), mock.patch('sys.stdout', new_callable=io.StringIO):
            summary.main()
        return json.loads(output.with_suffix('.json').read_text())

    def test_complete_and_partial_matrices(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            before, after = self.write_matrices(root)
            result = self.summarize(before, after, root / 'comparison')
            self.assertEqual(result['status'], 'complete')
            self.assertEqual(len(result['rows']), 4)
            self.assertEqual(result['rows'][0]['weir_change_percent'], 0)
            (after / 'lua' / 'mongo.json').unlink()
            result = self.summarize(before, after, root / 'partial')
            self.assertEqual(result['status'], 'partial')
            expected = [dict(case='lua', missing=['after'])]
            self.assertEqual(result['missing_cases'], expected)

    def test_mismatched_schema_parameters_or_round_order_are_rejected(self):
        for change in ('schema', 'parameters', 'order'):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                before, after = self.write_matrices(root)
                path = after / 'mixed' / 'mongo.json'
                report = json.loads(path.read_text())
                if change == 'schema':
                    report['schema'] = 3
                elif change == 'parameters':
                    report['parameters']['dataset']['concurrency'] = 16
                else:
                    report['pairs'][0]['order'] = 'weir-first'
                path.write_text(json.dumps(report))
                with self.assertRaisesRegex(SystemExit, 'mixed: (before/after|report schema)'):
                    self.summarize(before, after, root / 'comparison')


class PhysicalCountTests(unittest.TestCase):
    def test_only_complete_available_observations_are_aggregated(self):
        first_counts, second_counts = dict(find=3, update=0), dict(find=4, update=0)
        first = dict(timed_physical_database_command_deltas=first_counts)
        second = dict(timed_physical_database_command_deltas=second_counts)
        total, reasons = summary.summarize_physical_counts([first, second])
        expected = dict(find=7, update=0)
        self.assertEqual(total, expected)
        self.assertEqual(reasons, [])
        reset_counts = dict(find=0, update=0)
        unavailable = dict(timed_physical_database_command_deltas=reset_counts,
                           physical_database_command_observation_unavailable='counter reset during stage')
        empty_counts = {}
        empty = dict(timed_physical_database_command_deltas=empty_counts)
        negative_counts = dict(find=-1, update=0)
        negative = dict(timed_physical_database_command_deltas=negative_counts)
        missing_counts = dict(find=4)
        missing_command = dict(timed_physical_database_command_deltas=missing_counts)
        missing = {}
        for value, explanation in ((unavailable, 'counter reset during stage'), (empty, 'missing'),
                                   (missing, 'missing'), (negative, 'reset'), (missing_command, 'differ')):
            with self.subTest(explanation=explanation):
                total, reasons = summary.summarize_physical_counts([first, value])
                self.assertIsNone(total)
                self.assertIn(explanation, reasons[0])


if __name__ == '__main__':
    unittest.main()
