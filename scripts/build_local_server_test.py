import importlib.util
from pathlib import Path
import tempfile
import unittest


spec = importlib.util.spec_from_file_location('build_local_server', Path(__file__).with_name('build_local_server.py'))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class FrozenSourceTests(unittest.TestCase):
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
