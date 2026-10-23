import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location('server_source', Path(__file__).with_name('server_source.py'))
source = importlib.util.module_from_spec(spec)
spec.loader.exec_module(source)


class ServerFixture:
    def __init__(self, root):
        root = root.resolve()
        self.root = root
        root.mkdir()
        (root / 'go.mod').write_text('module github.com/batchstream/weir\n\ngo 1.27.1\n')
        (root / 'go.sum').write_text('fixed dependency sums\n')
        (root / 'cmd' / 'weir').mkdir(parents=True)
        (root / 'cmd' / 'weir' / 'main.go').write_text('package main\nfunc main() {}\n')
        (root / 'internal' / 'server').mkdir(parents=True)
        (root / 'internal' / 'server' / 'server.go').write_text('package server\n')
        self.metadata = {'Module': {'Path': source.SERVER_MODULE}, 'Go': '1.27.1', 'Require': [], 'Replace': []}

    def mod(self, directory, environment):
        return self.metadata

    def freeze(self, output):
        environment = {}
        with mock.patch.object(source, 'module_metadata', side_effect=self.mod):
            return source.freeze_server_source(self.root, output, environment)


class ServerSnapshotTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.fixture = ServerFixture(self.root / 'source')
        self.output = self.root / 'snapshot'
        self.environment = {}

    def test_complete_server_profile_is_hashed_and_bounded(self):
        (self.fixture.root / 'cmd' / 'weir' / 'main_test.go').write_text('not production\n')
        (self.fixture.root / 'third_party' / 'unrelated').mkdir(parents=True)
        (self.fixture.root / 'third_party' / 'unrelated' / 'unused.go').write_text('not allowed\n')
        (self.fixture.root / 'internal' / 'credentials').mkdir()
        (self.fixture.root / 'internal' / 'credentials' / 'token.go').write_text('not source\n')
        tree, manifest = self.fixture.freeze(self.output)
        paths = [item['path'] for item in manifest]
        self.assertEqual(paths, ['cmd/weir/main.go', 'go.mod', 'go.sum', 'internal/server/server.go'])
        source.verify_snapshot(self.output, manifest, tree)
        (self.output / 'cmd' / 'weir' / 'main.go').write_text('changed\n')
        with self.assertRaisesRegex(ValueError, 'hash mismatch'):
            source.verify_snapshot(self.output, manifest, tree)

    def test_module_replacements_are_rejected(self):
        variants = [
            {'Old': {'Path': 'google.golang.org/grpc'}, 'New': {'Path': './third_party/grpc'}},
            {'Old': {'Path': 'unexpected/module'}, 'New': {'Path': '../outside'}},
            {'Old': {'Path': 'google.golang.org/grpc'}, 'New': {'Path': 'example.com/grpc', 'Version': 'v1.0.0'}},
        ]
        for variant in variants:
            with self.subTest(variant=variant):
                self.fixture.metadata['Replace'] = [variant]
                with self.assertRaisesRegex(ValueError, 'must not replace'):
                    self.fixture.freeze(self.output)
                self.assertFalse(self.output.exists())

    def test_root_module_and_unexpected_nested_module_are_rejected(self):
        self.fixture.metadata['Module']['Path'] = 'wrong/module'
        with self.assertRaisesRegex(ValueError, 'wrong module'):
            self.fixture.freeze(self.output)
        self.fixture.metadata['Module']['Path'] = source.SERVER_MODULE
        (self.fixture.root / 'cmd' / 'weir' / 'go.mod').write_text('module unexpected\n')
        with self.assertRaisesRegex(ValueError, 'unexpected nested module'):
            self.fixture.freeze(self.output)

    def test_symlinks_are_rejected_not_silently_omitted(self):
        path = self.fixture.root / 'internal' / 'server' / 'server.go'
        path.unlink()
        path.symlink_to(self.fixture.root / 'cmd' / 'weir' / 'main.go')
        with self.assertRaisesRegex(ValueError, 'symlink'):
            self.fixture.freeze(self.output)
        path.unlink()
        path.write_text('package server\n')
        (self.fixture.root / 'pkg').symlink_to(self.fixture.root / 'cmd', target_is_directory=True)
        with self.assertRaisesRegex(ValueError, 'symlink'):
            self.fixture.freeze(self.output)

    def test_snapshot_rejects_manifest_corruption_extra_files_and_escaping_links(self):
        tree, manifest = self.fixture.freeze(self.output)
        with self.assertRaisesRegex(ValueError, 'manifest'):
            source.verify_snapshot(self.output, list(reversed(manifest)), tree)
        extra = self.output / 'extra.go'
        extra.write_text('package extra\n')
        with self.assertRaisesRegex(ValueError, 'manifest'):
            source.verify_snapshot(self.output, manifest, tree)
        extra.unlink()
        extra.symlink_to(self.fixture.root / 'cmd' / 'weir' / 'main.go')
        with self.assertRaisesRegex(ValueError, 'symlink'):
            source.verify_snapshot(self.output, manifest, tree)

    def test_snapshot_module_metadata_cannot_change_during_copy(self):
        changed = dict(self.fixture.metadata, Go='9.0.0')

        def mod(directory, environment):
            if directory == self.output:
                return changed
            return self.fixture.mod(directory, environment)

        with mock.patch.object(source, 'module_metadata', side_effect=mod):
            with self.assertRaisesRegex(ValueError, 'changed while freezing'):
                source.freeze_server_source(self.fixture.root, self.output, self.environment)


class SourceBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.environment = {}

    def test_git_environment_removes_inherited_configuration(self):
        inherited = dict(GIT_CONFIG_COUNT='1', GIT_CONFIG_KEY_0='url.file:///private.insteadOf',
                         GIT_CONFIG_VALUE_0='https://github.com/', GIT_DIR='/outside',
                         GIT_WORK_TREE='/outside', GOFLAGS='-overlay=/outside', GOTOOLCHAIN='auto')
        with mock.patch.dict(os.environ, inherited):
            environment = source.build_environment()
        for name in inherited:
            if name.startswith('GIT_'):
                self.assertNotIn(name, environment)
        self.assertEqual(environment['GIT_CONFIG_GLOBAL'], '/dev/null')
        self.assertEqual(environment['GOFLAGS'], '-mod=readonly')
        self.assertEqual(environment['GOTOOLCHAIN'], 'local')

    def test_checkout_is_exact_public_isolated_and_clean(self):
        revision = 'a' * 40
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary) / 'checkout'
            responses = [b'', b'', b'', revision.encode() + b'\n', b'']
            with mock.patch.object(source.subprocess, 'check_output', side_effect=responses) as command:
                source.checkout_source(directory, revision, source.build_environment())
            calls = [call.args[0] for call in command.call_args_list]
            self.assertIn('--template=', calls[0])
            self.assertEqual(calls[1][-4:], ['fetch', '--depth=1', source.PUBLIC_SOURCE, revision])
            for call in calls:
                self.assertIn('credential.helper=', call)
                self.assertIn('core.hooksPath=/dev/null', call)
                self.assertIn('protocol.file.allow=never', call)
            self.assertIn('--ignored', calls[-1])

    def test_wrong_sha_dirty_tree_and_network_failure_have_no_fallback(self):
        for responses in ([b'b' * 40, b''], [b'a' * 40, b'?? extra.go\n']):
            with mock.patch.object(source.subprocess, 'check_output', side_effect=responses):
                with self.assertRaises(ValueError):
                    source.check_checkout(Path('/unused'), 'a' * 40, self.environment)
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary) / 'checkout'
            failure = subprocess.CalledProcessError(1, ['git', 'fetch'])
            with mock.patch.object(source.subprocess, 'check_output', side_effect=[b'', failure]) as command:
                with self.assertRaises(subprocess.CalledProcessError):
                    source.checkout_source(directory, 'a' * 40, self.environment)
            self.assertEqual(command.call_count, 2)
            with self.assertRaises(ValueError):
                source.checkout_source(Path(temporary) / 'floating', 'main', self.environment)

    def test_toolchain_and_target_are_both_enforced(self):
        with mock.patch.object(source.subprocess, 'check_output', return_value='go1.27.1\ndarwin\narm64\ndarwin\narm64\n'):
            self.assertEqual(source.native_toolchain(self.environment, '1.27.1'), ('go1.27.1', 'darwin/arm64'))
        for value in ('go1.26.0\ndarwin\narm64\ndarwin\narm64\n',
                      'go1.27.1\ndarwin\narm64\nlinux\namd64\n'):
            with mock.patch.object(source.subprocess, 'check_output', return_value=value):
                with self.assertRaises(ValueError):
                    source.native_toolchain(self.environment, '1.27.1')

    def test_identity_rejects_wrong_source_toolchain_target_and_version(self):
        revision = 'a' * 40
        identity = dict(product='weir', version='dev', revision=revision, go='go1.27.1', target='darwin/arm64')
        toolchain = ('go1.27.1', 'darwin/arm64')
        source.validate_identity(identity, toolchain, revision)
        for name, value in [('product', 'other'), ('version', 'v0.2.0'), ('revision', 'b' * 40),
                            ('go', 'go1.26.0'), ('target', 'linux/amd64'), ('state', 'dev')]:
            with self.subTest(name=name):
                changed = dict(identity)
                changed[name] = value
                with self.assertRaises(ValueError):
                    source.validate_identity(changed, toolchain, revision)
        missing = dict(identity)
        del missing['revision']
        with self.assertRaises(ValueError):
            source.validate_identity(missing, toolchain, revision)


if __name__ == '__main__':
    unittest.main()
