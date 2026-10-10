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
            (source / 'go.mod').write_text('module local.invalid/source\n')
            identity = dict(revision='unknown')

            def run_build(command, **kwargs):
                self.assertEqual(kwargs['env']['GOFLAGS'], '-mod=readonly')
                self.assertEqual(kwargs['env']['GOENV'], 'off')
                self.assertEqual(kwargs['env']['GOWORK'], 'off')
                self.assertEqual(kwargs['env']['CGO_ENABLED'], '0')
                self.assertEqual(kwargs['cwd'], output.resolve().with_suffix('.source'))
                output.write_bytes(b'built from frozen inputs')

            arguments = ['build_local_server.py', '--source', str(source), '--output', str(output)]
            responses = ['a' * 40, '', b'', builder.json.dumps(identity)]
            environment = dict(GOFLAGS='-overlay=/unrecorded-input.json -mod=mod')
            with mock.patch('sys.argv', arguments), mock.patch.dict(builder.os.environ, environment), \
                    mock.patch('sys.stdout', new_callable=io.StringIO), \
                    mock.patch.object(builder.subprocess, 'check_output', side_effect=responses), \
                    mock.patch.object(builder.subprocess, 'run', side_effect=run_build) as build:
                builder.main()
            self.assertEqual(build.call_count, 1)
            receipt = builder.json.loads(output.with_suffix('.receipt.json').read_text())
            self.assertEqual(receipt['sha256'], builder.digest(output.read_bytes()))

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
            tree, manifest = builder.freeze_source(source, frozen)
            self.assertEqual([item['path'] for item in manifest], ['go.mod', 'internal/new.go'])
            self.assertEqual((frozen / 'internal' / 'new.go').read_text(), 'package internal\n')
            ordinary.write_text('package changed\n')
            changed, _ = builder.freeze_source(source, root / 'changed')
            self.assertNotEqual(tree, changed)
            self.assertEqual((frozen / 'internal' / 'new.go').read_text(), 'package internal\n')


if __name__ == '__main__':
    unittest.main()
