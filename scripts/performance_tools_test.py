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
        if kind == 'full-git':
            identity.update(product='weir', version='local-' + versions['weir_source'],
                            state='clean-commit', dirty='false', go='go' + versions['go'], target='darwin/arm64')
            source_files = [dict(path=name, sha256='1' * 64)
                            for name in ('cmd/weir/main.go', 'go.mod', 'go.sum')]
            server.update(source_kind='verified full git checkout', source_dirty=False,
                          source_tree_sha256='b' * 64, source_files=source_files, go=versions['go'])
    receipt = dict(client_source_tree_sha256='d' * 64, client_binary_sha256='e' * 64,
                   server_binary_sha256=server['sha256'], server_receipt=server,
                   versions=versions, exit_code=0, command=['performance-lab', '-backend', 'mongo'])
    return receipt


def make_report(receipt):
    server, versions = receipt['server_receipt'], receipt['versions']
    provenance = dict(weir_binary_sha256=server['sha256'], weir_source=server['source'])
    for field in ('sdk', 'protocol', 'mongodb_image', 'elasticsearch_image'):
        provenance[field] = versions[field]
    if server.get('source_kind') == 'frozen local Go source tree':
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


def consume_produced_server(options):
    binary, root = options['binary'], options['root']
    versions = json.loads((Path(runner.__file__).resolve().parent.parent / 'versions.json').read_text())
    server = json.loads(binary.with_suffix('.receipt.json').read_text())
    report_receipt = dict(server_receipt=server, versions=versions)
    output, fixture = root / 'matrix', root / 'fixture'
    fixture.mkdir()
    commands = []

    def run_command(command, **kwargs):
        if command[0] == 'go':
            Path(command[command.index('-o') + 1]).write_bytes(b'frozen client')
        else:
            commands.append(command)
            report = make_report(report_receipt)
            report['provenance']['fixture_logs'] = str(fixture)
            target = Path(command[command.index('-output') + 1])
            target.mkdir()
            (target / 'mongo.json').write_text(json.dumps(report))
        result = subprocess.CompletedProcess(command, 0)
        return result

    arguments = ['run_performance_matrix.py', '--backend', 'mongo', '--weir', str(binary), '--output', str(output)]
    with mock.patch('sys.argv', arguments), mock.patch('sys.stdout', new_callable=io.StringIO), \
            mock.patch.object(runner.subprocess, 'run', side_effect=run_command), \
            mock.patch.object(runner.subprocess, 'check_output', side_effect=['f' * 40, b'']):
        runner.main()
    receipts = []
    for case in ('read', 'mixed', 'write', 'lua'):
        receipt = json.loads((output / (case + '.receipt.json')).read_text())
        report = json.loads((output / case / 'mongo.json').read_text())
        evidence.validate_report_provenance(report, receipt)
        receipts.append(receipt)
    result = dict(receipts=receipts, commands=commands)
    return result


class ProvenanceTests(unittest.TestCase):
    def setUp(self):
        self.versions = dict(weir_source='f' * 40, weir_version='v0.1.1-0.pinned', sdk='v0.10.0', go='1.27.1',
                             protocol='v0.8.0', mongodb_image='mongo@sha256:pinned',
                             elasticsearch_image='elasticsearch@sha256:pinned')

    def test_local_and_locked_receipts_match_actual_report(self):
        for kind in ('local', 'locked', 'full-git'):
            with self.subTest(kind=kind):
                receipt = make_receipt(kind, self.versions)
                report = make_report(receipt)
                evidence.validate_report_provenance(report, receipt)

    def test_each_actual_provenance_field_is_required_and_bound(self):
        for kind in ('local', 'locked', 'full-git'):
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

    def test_full_git_receipt_is_locked_not_local(self):
        receipt = make_receipt('full-git', self.versions)
        server = receipt['server_receipt']
        self.assertNotIn('source_diff_sha256', server)
        self.assertEqual(evidence.validate_server_receipt(server, server['sha256'], self.versions), 'locked')
        evidence.validate_report_provenance(make_report(receipt), receipt)

    def test_full_git_receipt_rejects_wrong_lock_identity_and_mixed_provenance(self):
        receipt = make_receipt('full-git', self.versions)
        original = receipt['server_receipt']
        changes = [dict(source='0' * 40), dict(version='wrong'), dict(go='1.26.0'),
                   dict(sha256='0' * 64), dict(source_dirty=True), dict(source_dirty=0),
                   dict(source_tree_sha256='invalid'), dict(source_kind='unverified'),
                   dict(source_kind='frozen local Go source tree'), dict(source_diff_sha256='c' * 64),
                   dict(source_kind='frozen local Go source tree', source_diff_sha256='c' * 64),
                   dict(source_files=[]), dict(source_files='not a manifest')]
        for field in ('product', 'revision', 'version', 'go', 'state', 'dirty', 'target'):
            identity = dict(original['identity'])
            identity[field] = 'wrong'
            changes.append(dict(identity=identity))
        for change in changes:
            with self.subTest(change=change):
                changed = copy.deepcopy(original)
                changed.update(change)
                with self.assertRaises(ValueError):
                    evidence.validate_server_receipt(changed, original['sha256'], self.versions)
        for field in ('source_kind', 'source_dirty', 'source_tree_sha256', 'source_files', 'go', 'identity'):
            with self.subTest(missing=field):
                changed = copy.deepcopy(original)
                del changed[field]
                with self.assertRaises(ValueError):
                    evidence.validate_server_receipt(changed, original['sha256'], self.versions)

    def test_full_git_manifest_metadata_is_checked_without_reading_files(self):
        receipt = make_receipt('full-git', self.versions)
        original = receipt['server_receipt']
        path_variants = ('../escape.go', '/outside.go', 'cmd//weir/main.go', '.env', 'credentials',
                         'third_party/grpc/private.key', 'third_party/golua/credentials/token.go',
                         'third_party/unrelated/source.go', 'cmd/weir/main_test.go',
                         'cmd/weir/testdata/extra.go', 'internal/vendor/extra.go')
        manifests = [list(reversed(original['source_files'])), original['source_files'][:-1],
                     original['source_files'] + [original['source_files'][-1]]]
        for path in path_variants:
            entry = dict(path=path, sha256='1' * 64)
            manifests.append(sorted(original['source_files'] + [entry], key=lambda item: item['path']))
        broken = copy.deepcopy(original['source_files'])
        broken[0]['sha256'] = 'not a digest'
        manifests.append(broken)
        for manifest in manifests:
            with self.subTest(manifest=manifest):
                changed = dict(original, source_files=manifest)
                with self.assertRaises(ValueError):
                    evidence.validate_server_receipt(changed, original['sha256'], self.versions)

    def test_full_git_nested_manifest_metadata_accepts_only_complete_public_source_paths(self):
        receipt = make_receipt('full-git', self.versions)
        server = receipt['server_receipt']
        paths = ['third_party/README.md', 'third_party/grpc/credentials/alts/auth.go']
        for relative in ('third_party/grpc', 'third_party/golua'):
            paths.extend(relative + '/' + name for name in
                         ('go.mod', 'go.sum', 'LICENSE', 'WEIR_UPSTREAM.json', 'WEIR_PATCH.diff', 'WEIR_PATCH.md'))
        server['source_files'].extend(dict(path=name, sha256='2' * 64) for name in paths)
        server['source_files'].sort(key=lambda entry: entry['path'])
        self.assertEqual(evidence.validate_server_receipt(server, server['sha256'], self.versions), 'locked')
        for name in paths:
            if name.endswith('.go') or name.endswith('go.sum'):
                continue
            with self.subTest(missing=name):
                changed = copy.deepcopy(server)
                changed['source_files'] = [entry for entry in changed['source_files'] if entry['path'] != name]
                with self.assertRaisesRegex(ValueError, 'nested module provenance'):
                    evidence.validate_server_receipt(changed, server['sha256'], self.versions)

    def test_local_receipt_does_not_lose_tree_diff_dirty_or_identity_checks(self):
        receipt = make_receipt('local', self.versions)
        original = receipt['server_receipt']
        for field, value in [('source_kind', 'verified full git checkout'), ('source_kind', 'unknown'),
                             ('source_tree_sha256', 'invalid'), ('source_diff_sha256', None),
                             ('source_dirty', 0), ('identity', None), ('sha256', '0' * 64)]:
            with self.subTest(field=field, value=value):
                changed = dict(original)
                changed[field] = value
                with self.assertRaises(ValueError):
                    evidence.validate_server_receipt(changed, original['sha256'], self.versions)
        changed = dict(original)
        del changed['source_kind']
        with self.assertRaises(ValueError):
            evidence.validate_server_receipt(changed, original['sha256'], self.versions)

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
        for kind in ('local', 'locked', 'full-git'):
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
                    arguments = ['run_performance_matrix.py', '--backend', 'mongo', '--weir', str(weir), '--output', str(output),
                                 '--database-cpus', '1', '--concurrency-levels', '4,16,64,256',
                                 '--client-processes', '4', '--records', '2048', '--payload-bytes', '512', '--rounds', '5',
                                 '--bulk', '--bulk-concurrency-levels', '4,16']
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
                    self.assertEqual(len(commands), 6)
                    for command in commands:
                        self.assertEqual('-server-receipt' in command, kind == 'local')
                        self.assertEqual(command[command.index('-database-cpus') + 1], '1.0')
                        self.assertEqual(command[command.index('-records') + 1], '2048')
                        self.assertEqual(command[command.index('-payload-bytes') + 1], '512')
                        self.assertEqual(command[command.index('-rounds') + 1], '5')
                        expected_levels = '4,16' if command[command.index('-mode') + 1] == 'bulk-saturation' else '4,16,64,256'
                        self.assertEqual(command[command.index('-concurrency-levels') + 1], expected_levels)
                        if kind == 'local':
                            self.assertEqual(command[command.index('-server-receipt') + 1], str(output.resolve() / 'server.receipt.json'))
                    saved = json.loads((output / 'read.receipt.json').read_text())
                    self.assertEqual(saved['server_receipt'], receipt['server_receipt'])
                    self.assertEqual(saved['server_receipt_kind'], 'local' if kind == 'local' else 'locked')
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
