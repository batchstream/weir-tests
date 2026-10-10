import copy
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

import performance_evidence as evidence
import run_performance_matrix as runner
import summarize_performance as summary


def make_receipt(kind, versions):
    identity = dict(revision='unknown' if kind == 'local' else versions['weir_source'])
    server = dict(source=versions['weir_source'], sha256='a' * 64, identity=identity)
    if kind == 'local':
        server.update(source_tree_sha256='b' * 64, source_diff_sha256='c' * 64,
                      source_dirty=True, source_kind='frozen local Go source tree')
    else:
        server['version'] = versions['weir_version']
    receipt = dict(client_source_tree_sha256='d' * 64, client_binary_sha256='e' * 64,
                   server_binary_sha256=server['sha256'], server_receipt=server,
                   versions=versions, exit_code=0, command=['performance-lab', '-backend', 'mongo'])
    return receipt


def make_report(receipt):
    server, versions = receipt['server_receipt'], receipt['versions']
    provenance = dict(weir_binary_sha256=server['sha256'], weir_source=server['source'])
    for field in ('sdk', 'protocol', 'mongodb_image', 'elasticsearch_image'):
        provenance[field] = versions[field]
    if 'source_tree_sha256' in server:
        provenance.update(weir_binary_revision=server['identity']['revision'],
                          weir_source_tree_sha256=server['source_tree_sha256'],
                          weir_source_diff_sha256=server['source_diff_sha256'],
                          weir_source_dirty=str(server['source_dirty']).lower(),
                          weir_source_kind='explicit local source build; unpublished changes may be present')
    else:
        provenance['weir_source_kind'] = 'locked immutable module source'
    commands = dict(find=100, update=0)
    latency = dict(p95_upper_bound_ns=5_000_000)
    resources = dict(samples=[])
    stage = dict(succeeded=100, elapsed_ns=1_000_000_000, successful_operations_per_second=100,
                 reads=100, writes=0, errors=0, indeterminate=0, verified=True,
                 latency=latency, resources=resources, timed_physical_database_command_deltas=commands)
    pair = dict(concurrency=8, batch_size=1, round=1, order='direct-first', direct=stage, weir=copy.deepcopy(stage))
    dataset = dict(backend='mongo', concurrency=8, namespace='owned', weir_seed='local')
    parameters = dict(dataset=dataset, concurrency_levels=[8], batch_sizes=[1], paired_rounds=1)
    report = dict(provenance=provenance, parameters=parameters, pairs=[pair])
    return report


class ProvenanceTests(unittest.TestCase):
    def setUp(self):
        self.versions = dict(weir_source='f' * 40, weir_version='v0.1.1-0.pinned', sdk='v0.10.0',
                             protocol='v0.8.0', mongodb_image='mongo@sha256:pinned',
                             elasticsearch_image='elasticsearch@sha256:pinned')

    def test_local_and_locked_receipts_match_actual_report(self):
        for kind in ('local', 'locked'):
            with self.subTest(kind=kind):
                receipt = make_receipt(kind, self.versions)
                report = make_report(receipt)
                evidence.validate_report_provenance(report, receipt)

    def test_each_actual_provenance_field_is_required_and_bound(self):
        for kind in ('local', 'locked'):
            receipt = make_receipt(kind, self.versions)
            original = make_report(receipt)
            for field in original['provenance']:
                for replacement in ('missing', 'different'):
                    with self.subTest(kind=kind, field=field, replacement=replacement):
                        report = copy.deepcopy(original)
                        if replacement == 'missing':
                            del report['provenance'][field]
                        else:
                            report['provenance'][field] = 'different'
                        with self.assertRaisesRegex(ValueError, field):
                            evidence.validate_report_provenance(report, receipt)

    def test_incomplete_or_inconsistent_receipts_cannot_certify_reports(self):
        original = make_receipt('local', self.versions)
        report = make_report(original)
        failures = [('client_binary_sha256', None), ('client_source_tree_sha256', ''),
                    ('server_receipt', None), ('validation_error', 'binary replaced'),
                    ('server_binary_before_sha256', '0' * 64), ('server_binary_after_sha256', '0' * 64)]
        for field, value in failures:
            with self.subTest(field=field):
                receipt = copy.deepcopy(original)
                receipt[field] = value
                with self.assertRaises(ValueError):
                    evidence.validate_report_provenance(report, receipt)
        for field in ('source', 'source_tree_sha256', 'source_diff_sha256', 'source_dirty', 'identity', 'sha256'):
            with self.subTest(server_field=field):
                receipt = copy.deepcopy(original)
                del receipt['server_receipt'][field]
                with self.assertRaises(ValueError):
                    evidence.validate_report_provenance(report, receipt)

    def test_locked_receipt_cannot_claim_an_unpinned_source(self):
        receipt = make_receipt('locked', self.versions)
        receipt['server_receipt']['source'] = '0' * 40
        with self.assertRaisesRegex(ValueError, 'pinned manifest'):
            evidence.validate_report_provenance(make_report(receipt), receipt)

    def test_summary_cli_rejects_one_replaced_actual_report(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            before, after = root / 'before', root / 'after'
            for phase in (before, after):
                phase.mkdir()
                for case in ('read', 'mixed', 'write', 'lua'):
                    receipt = make_receipt('local', self.versions)
                    report = make_report(receipt)
                    (phase / case).mkdir()
                    (phase / case / 'mongo.json').write_text(json.dumps(report))
                    (phase / (case + '.receipt.json')).write_text(json.dumps(receipt))
            output = root / 'comparison'
            arguments = ['summarize_performance.py', '--before', str(before), '--after', str(after),
                         '--backend', 'mongo', '--output', str(output)]
            with mock.patch('sys.argv', arguments), mock.patch('sys.stdout', new_callable=io.StringIO):
                summary.main()
            self.assertEqual(json.loads(output.with_suffix('.json').read_text())['status'], 'complete')
            changed = after / 'mixed' / 'mongo.json'
            report = json.loads(changed.read_text())
            report['provenance']['weir_binary_sha256'] = '0' * 64
            changed.write_text(json.dumps(report))
            with mock.patch('sys.argv', arguments):
                with self.assertRaisesRegex(SystemExit, 'mixed after:.*weir_binary_sha256'):
                    summary.main()


class MatrixRunnerTests(unittest.TestCase):
    def test_automatic_and_explicit_receipts_select_local_or_locked_lab_flags(self):
        versions = json.loads((Path(runner.__file__).resolve().parent.parent / 'versions.json').read_text())
        for kind in ('local', 'locked'):
            for explicit in (False, True):
                with self.subTest(kind=kind, explicit=explicit), tempfile.TemporaryDirectory() as directory:
                    root = Path(directory)
                    weir, output = root / 'server', root / 'matrix'
                    weir.write_bytes(b'frozen server')
                    receipt = make_receipt(kind, versions)
                    receipt['server_receipt']['sha256'] = hashlib.sha256(weir.read_bytes()).hexdigest()
                    receipt['server_binary_sha256'] = receipt['server_receipt']['sha256']
                    server_receipt_path = weir.with_suffix('.receipt.json')
                    server_receipt_path.write_text(json.dumps(receipt['server_receipt']))
                    arguments = ['run_performance_matrix.py', '--backend', 'mongo', '--weir', str(weir), '--output', str(output)]
                    if explicit:
                        arguments += ['--server-receipt', str(server_receipt_path)]
                    commands = []
                    fixture = root / 'fixture'
                    fixture.mkdir()

                    def run_command(command, **kwargs):
                        if command[0] == 'go':
                            Path(command[command.index('-o') + 1]).write_bytes(b'frozen client')
                        else:
                            commands.append(command)
                            report = make_report(receipt)
                            report['provenance']['fixture_logs'] = str(fixture)
                            target = Path(command[command.index('-output') + 1])
                            target.mkdir()
                            (target / 'mongo.json').write_text(json.dumps(report))
                        result = subprocess.CompletedProcess(command, 0)
                        return result

                    with mock.patch('sys.argv', arguments), mock.patch('sys.stdout', new_callable=io.StringIO), \
                            mock.patch.object(runner.subprocess, 'run', side_effect=run_command), \
                            mock.patch.object(runner.subprocess, 'check_output', side_effect=['f' * 40, b'']):
                        runner.main()
                    self.assertEqual(len(commands), 4)
                    for command in commands:
                        self.assertEqual('-server-receipt' in command, kind == 'local')
                        if kind == 'local':
                            self.assertEqual(command[command.index('-server-receipt') + 1], str(output.resolve() / 'server.receipt.json'))
                    saved = json.loads((output / 'read.receipt.json').read_text())
                    self.assertEqual(saved['server_receipt'], receipt['server_receipt'])
                    self.assertEqual(saved['server_receipt_kind'], kind)
                    self.assertEqual(saved['server_binary_before_sha256'], saved['server_binary_after_sha256'])

    def test_binary_replacement_before_or_during_case_aborts_matrix_and_retains_evidence(self):
        versions = json.loads((Path(runner.__file__).resolve().parent.parent / 'versions.json').read_text())
        for when in ('before', 'during'):
            with self.subTest(when=when), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                weir, output = root / 'server', root / 'matrix'
                weir.write_bytes(b'initial server')
                receipt = make_receipt('local', versions)
                receipt['server_receipt']['sha256'] = hashlib.sha256(weir.read_bytes()).hexdigest()
                weir.with_suffix('.receipt.json').write_text(json.dumps(receipt['server_receipt']))
                arguments = ['run_performance_matrix.py', '--backend', 'mongo', '--weir', str(weir), '--output', str(output)]

                def run_command(command, **kwargs):
                    if command[0] == 'go':
                        Path(command[command.index('-o') + 1]).write_bytes(b'frozen client')
                        if when == 'before':
                            weir.write_bytes(b'replacement server')
                    else:
                        weir.write_bytes(b'replacement server')
                    result = subprocess.CompletedProcess(command, 0)
                    return result

                with mock.patch('sys.argv', arguments), mock.patch('sys.stdout', new_callable=io.StringIO), \
                        mock.patch.object(runner.subprocess, 'run', side_effect=run_command) as run, \
                        mock.patch.object(runner.subprocess, 'check_output', side_effect=['f' * 40, b'']):
                    with self.assertRaisesRegex(ValueError, 'server binary changed ' + when + ' read'):
                        runner.main()
                self.assertEqual(run.call_count, 1 if when == 'before' else 2)
                saved = json.loads((output / 'read.receipt.json').read_text())
                if when == 'during':
                    self.assertEqual(saved['exit_code'], 0)
                    self.assertNotEqual(saved['server_binary_after_sha256'], saved['server_binary_sha256'])
                else:
                    self.assertNotEqual(saved['server_binary_before_sha256'], saved['server_binary_sha256'])
                self.assertIn('changed ' + when + ' read', saved['validation_error'])
                self.assertFalse((output / 'mixed.receipt.json').exists())


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
