import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location('build_local_server', Path(__file__).with_name('build_local_server.py'))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class FrozenSourceTests(unittest.TestCase):
    def test_build_overrides_inherited_overlay_flags(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, output = root / 'source', root / 'server'
            source.mkdir()
            (source / 'go.mod').write_text('module github.com/batchstream/weir\n')
            (source / 'go.sum').write_text('fixed sums\n')
            identity = dict(product='weir', revision='unknown', version='dev', state='dev', dirty='unknown',
                            go='go1.27.1', target='darwin/arm64')
            metadata = {'Module': {'Path': 'github.com/batchstream/weir'}}

            def run_build(command, **kwargs):
                self.assertEqual(kwargs['env']['GOFLAGS'], '-mod=readonly')
                self.assertEqual(kwargs['env']['GOENV'], 'off')
                self.assertEqual(kwargs['env']['GOWORK'], 'off')
                self.assertEqual(kwargs['env']['CGO_ENABLED'], '0')
                self.assertEqual(kwargs['cwd'], output.resolve().with_suffix('.source'))
                output.write_bytes(b'built from frozen inputs')

            arguments = ['build_local_server.py', '--source', str(source), '--output', str(output)]
            responses = ['a' * 40, '?? cmd/weir/new.go\n',
                         'go1.27.1\ndarwin\narm64\ndarwin\narm64\n',
                         builder.json.dumps(metadata), builder.json.dumps(metadata), b'local diff',
                         builder.json.dumps(identity)]
            environment = dict(GOFLAGS='-overlay=/unrecorded-input.json -mod=mod', GIT_DIR='/unrecorded-source')
            with mock.patch('sys.argv', arguments), mock.patch.dict(builder.os.environ, environment), \
                    mock.patch('sys.stdout', new_callable=io.StringIO), \
                    mock.patch.object(builder.subprocess, 'check_output', side_effect=responses) as command, \
                    mock.patch.object(builder.subprocess, 'run', side_effect=run_build) as build:
                builder.main()
            self.assertEqual(build.call_count, 1)
            git_calls = [call for call in command.call_args_list if call.args[0][0] == 'git']
            for call in git_calls:
                self.assertNotIn('GIT_DIR', call.kwargs['env'])
            self.assertIn('--no-ext-diff', git_calls[-1].args[0])
            self.assertIn('--no-textconv', git_calls[-1].args[0])
            receipt = builder.json.loads(output.with_suffix('.receipt.json').read_text())
            self.assertEqual(receipt['sha256'], builder.digest(output.read_bytes()))
            self.assertTrue(receipt['source_dirty'])
            self.assertEqual(receipt['identity']['state'], 'dev')
            self.assertEqual(receipt['source_diff_sha256'], builder.digest(b'local diff'))
            self.assertEqual(receipt['source'], 'a' * 40)
            self.assertEqual(receipt['source_kind'], 'frozen local Go source tree')
            import performance_tools_test as tools

            options = dict(binary=output, root=root)
            consumed = tools.consume_produced_server(options)
            self.assertEqual(len(consumed['commands']), 4)
            for command in consumed['commands']:
                self.assertIn('-server-receipt', command)
            for saved in consumed['receipts']:
                self.assertEqual(saved['server_receipt_kind'], 'local')
                self.assertTrue(saved['server_receipt']['source_dirty'])
                self.assertEqual(saved['server_receipt']['source_diff_sha256'], receipt['source_diff_sha256'])

    def test_local_source_output_and_existing_artifact_symlinks_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            ordinary = root / 'source'
            ordinary.mkdir()
            linked = root / 'linked'
            linked.symlink_to(ordinary, target_is_directory=True)
            output = root / 'server'
            arguments = ['build_local_server.py', '--source', str(linked), '--output', str(output)]
            with mock.patch('sys.argv', arguments):
                with self.assertRaisesRegex(ValueError, 'symlinks'):
                    builder.main()
            arguments[2] = str(ordinary)
            for path in (output, output.with_suffix('.receipt.json'), output.with_suffix('.source')):
                with self.subTest(path=path):
                    path.symlink_to(root / 'absent')
                    with mock.patch('sys.argv', arguments):
                        with self.assertRaises(ValueError):
                            builder.main()
                    self.assertTrue(path.is_symlink())
                    path.unlink()

    def test_untracked_go_inputs_are_frozen_without_secret_or_symlink_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, frozen = root / 'source', root / 'frozen'
            source.mkdir()
            (source / 'go.mod').write_text('module local.invalid/source\n')
            (source / 'internal').mkdir()
            ordinary = source / 'internal' / 'new.go'
            ordinary.write_text('package internal\n')
            (source / 'internal' / '.env.go').write_text('excluded fixture\n')
            secrets = source / 'credentials'
            secrets.mkdir()
            (secrets / 'excluded.go').write_text('excluded fixture\n')
            (source / 'internal' / 'linked.go').symlink_to(secrets / 'excluded.go')
            (source / 'third_party' / 'grpc').mkdir(parents=True)
            (source / 'third_party' / 'grpc' / 'ignored.go').write_text('not a client input\n')
            tree, manifest = builder.freeze_source(source, frozen)
            self.assertEqual([item['path'] for item in manifest], ['go.mod', 'internal/new.go'])
            self.assertEqual((frozen / 'internal' / 'new.go').read_text(), 'package internal\n')
            ordinary.write_text('package changed\n')
            changed, _ = builder.freeze_source(source, root / 'changed')
            self.assertNotEqual(tree, changed)
            self.assertEqual((frozen / 'internal' / 'new.go').read_text(), 'package internal\n')


if __name__ == '__main__':
    unittest.main()
