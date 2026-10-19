import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location('prepare_weir', Path(__file__).with_name('prepare_weir.py'))
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


class SourceTests(unittest.TestCase):
    def setUp(self):
        self.pins = dict(weir_source='a' * 40, weir_version='v0.1.1-0.20261003014747-aaaaaaaaaaaa', weir_sum='h1:source')
        self.module = dict(Path='github.com/batchstream/weir', Version=self.pins['weir_version'],
                           Sum=self.pins['weir_sum'], Origin=dict(Hash=self.pins['weir_source']))

    def test_exact_source(self):
        prepare.validate_source(self.pins, self.module)

    def test_floating_source_and_wrong_artifact_are_rejected(self):
        for key, value in [('Path', 'another/module'), ('Version', 'main'), ('Sum', 'h1:different'),
                           ('Origin', dict(Hash='b' * 40)), ('Replace', dict(Path='../weir')), ('Error', 'missing')]:
            with self.subTest(key=key):
                changed = dict(self.module)
                changed[key] = value
                with self.assertRaises(ValueError):
                    prepare.validate_source(self.pins, changed)
        changed = dict(self.pins, weir_source='main')
        with self.assertRaises(ValueError):
            prepare.validate_source(changed, self.module)


class PreparationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.pins = dict(weir_source='a' * 40, weir_version='v0.2.1-0.20261015175329-aaaaaaaaaaaa',
                         weir_sum='h1:fixed', go='1.27.1')
        (self.root / 'versions.json').write_text(json.dumps(self.pins))
        self.output = self.root / '.tools' / 'weir'
        self.identity = dict(product='weir', revision=self.pins['weir_source'],
                             version='local-' + self.pins['weir_source'], state='clean-commit', dirty='false',
                             go='go1.27.1', target='darwin/arm64')
        self.metadata = {'Module': {'Path': 'github.com/batchstream/weir'}, 'Go': '1.27.1'}
        self.source_directory = None
        self.build_directories = []

    def checkout(self, directory, revision, environment):
        self.assertEqual(revision, self.pins['weir_source'])
        directory.mkdir()
        (directory / 'go.mod').write_text('module github.com/batchstream/weir\n\ngo 1.27.1\n')
        (directory / 'go.sum').write_text('fixed sums\n')
        (directory / 'cmd' / 'weir').mkdir(parents=True)
        (directory / 'cmd' / 'weir' / 'main.go').write_text('package main\nfunc main() {}\n')
        self.source_directory = directory

    def command_output(self, command, **kwargs):
        if command[:2] == ['go', 'env']:
            return 'go1.27.1\ndarwin\narm64\ndarwin\narm64\n'
        if command[:4] == ['go', 'mod', 'download', '-json']:
            origin = dict(Hash=self.pins['weir_source'])
            module = dict(Path='github.com/batchstream/weir', Version=self.pins['weir_version'],
                          Sum=self.pins['weir_sum'], Origin=origin,
                          Dir=str(self.root / 'module-cache-must-never-be-used'))
            return json.dumps(module)
        if command == ['go', 'mod', 'edit', '-json']:
            return json.dumps(self.metadata)
        if command[-1] == 'version':
            return json.dumps(self.identity)
        self.fail('unexpected subprocess: ' + repr(command))

    def run_go(self, command, **kwargs):
        directory = kwargs['cwd']
        self.build_directories.append(directory)
        self.assertNotEqual(directory, self.source_directory)
        self.assertEqual(directory.name, 'snapshot')
        self.assertEqual(kwargs['env']['GOWORK'], 'off')
        self.assertEqual(kwargs['env']['GOPROXY'], 'off')
        self.assertEqual(kwargs['env']['GOSUMDB'], 'off')
        self.assertEqual(kwargs['env']['GOTOOLCHAIN'], 'local')
        self.assertTrue((directory / 'cmd' / 'weir' / 'main.go').is_file())
        if command[:2] == ['go', 'build']:
            self.assertIn('-X main.sourceRevision=' + self.pins['weir_source'], command)
            binary = Path(command[command.index('-o') + 1])
            binary.write_bytes(b'verified binary')

    def invoke(self):
        arguments = ['prepare_weir.py', '--offline']
        with mock.patch.object(prepare, 'ROOT', self.root), mock.patch('sys.argv', arguments), \
                mock.patch('sys.stdout', new_callable=io.StringIO), \
                mock.patch.object(prepare, 'checkout_source', side_effect=self.checkout), \
                mock.patch.object(prepare, 'check_checkout') as clean, \
                mock.patch.object(prepare.subprocess, 'check_output', side_effect=self.command_output), \
                mock.patch.object(prepare.subprocess, 'run', side_effect=self.run_go):
            prepare.main()
        return clean

    def test_locked_preparation_never_builds_module_cache_or_checkout(self):
        clean = self.invoke()
        self.assertEqual(len(self.build_directories), 3)
        clean.assert_called_once()
        receipt = json.loads(self.output.with_suffix('.receipt.json').read_text())
        self.assertEqual(receipt['source'], self.pins['weir_source'])
        self.assertEqual(receipt['version'], self.pins['weir_version'])
        self.assertEqual(receipt['go'], self.pins['go'])
        self.assertEqual(receipt['source_kind'], 'verified full git checkout')
        self.assertFalse(receipt['source_dirty'])
        self.assertEqual(receipt['identity'], self.identity)
        self.assertEqual(receipt['sha256'], prepare.hashlib.sha256(b'verified binary').hexdigest())
        self.assertEqual(len(receipt['source_files']), 3)
        self.build_directories.clear()
        clean = self.invoke()
        self.assertEqual(self.build_directories, [])
        clean.assert_not_called()

    def test_failed_identity_is_not_published_and_cannot_fallback(self):
        self.identity['target'] = 'linux/amd64'
        with self.assertRaisesRegex(ValueError, 'identity'):
            self.invoke()
        self.assertFalse(self.output.exists())
        self.assertFalse(self.output.with_suffix('.receipt.json').exists())

    def test_wrong_root_module_is_not_built_or_published(self):
        self.metadata['Module']['Path'] = 'wrong/module'
        with self.assertRaisesRegex(ValueError, 'module identity'):
            self.invoke()
        self.assertFalse(self.build_directories)
        self.assertFalse(self.output.exists())

    def test_existing_receipt_version_digest_source_and_identity_must_match(self):
        self.invoke()
        receipt_path = self.output.with_suffix('.receipt.json')
        receipt = json.loads(receipt_path.read_text())
        variants = [dict(version='old-version'), dict(source='b' * 40), dict(go='1.26.0'),
                    dict(sha256='0' * 64), dict(source_kind='module cache'),
                    dict(source_tree_sha256='invalid'), {'identity': {'revision': 'old'}}]
        for variant in variants:
            with self.subTest(variant=variant):
                changed = dict(receipt)
                changed.update(variant)
                receipt_path.write_text(json.dumps(changed))
                with self.assertRaises(ValueError):
                    self.invoke()
                self.assertEqual(self.output.read_bytes(), b'verified binary')
        old_receipt = {key: value for key, value in receipt.items()
                       if key in ('source', 'version', 'go', 'sha256', 'identity')}
        receipt_path.write_text(json.dumps(old_receipt))
        with self.assertRaises(ValueError):
            self.invoke()

    def test_orphan_receipt_and_binary_symlink_are_preserved(self):
        self.output.parent.mkdir()
        receipt_path = self.output.with_suffix('.receipt.json')
        receipt_path.write_text('{}')
        with self.assertRaisesRegex(ValueError, 'orphaned'):
            self.invoke()
        receipt_path.unlink()
        self.output.symlink_to(self.root / 'missing')
        with self.assertRaisesRegex(ValueError, 'symlinks'):
            self.invoke()
        self.assertTrue(self.output.is_symlink())


if __name__ == '__main__':
    unittest.main()
