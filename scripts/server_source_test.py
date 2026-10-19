import importlib.util
import json
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
        self.metadata = {'Module': {'Path': source.SERVER_MODULE}, 'Go': '1.27.1', 'Require': [], 'Replace': []}
        self.nested = {}
        for module, relative in source.NESTED.items():
            directory = root / relative
            directory.mkdir(parents=True)
            files = {'go.mod': 'module ' + module + '\n', 'LICENSE': 'public license\n',
                     'go.sum': 'nested sums\n', 'library.go': 'package library\n'}
            if module == 'google.golang.org/grpc':
                files['credentials/alts/auth.go'] = 'package alts\n'
            hashes = {}
            for name, data in files.items():
                (directory / name).parent.mkdir(parents=True, exist_ok=True)
                (directory / name).write_text(data)
                hashes[name] = source.digest(data.encode())
            new = []
            for name in ('WEIR_PATCH.diff', 'WEIR_PATCH.md'):
                data = 'patch metadata\n'
                (directory / name).write_text(data)
                entry = dict(path=name, sha256=source.digest(data.encode()))
                new.append(entry)
            test_entry = dict(path='library_test.go', sha256='b' * 64)
            new.append(test_entry)
            manifest = dict(module=module, version='v1.0.0', files=hashes, newFiles=new, patchedFiles=[])
            (directory / 'WEIR_UPSTREAM.json').write_text(json.dumps(manifest))
            requirement = dict(Path=module, Version='v1.0.0')
            replacement = {'Old': {'Path': module}, 'New': {'Path': './' + relative}}
            self.metadata['Require'].append(requirement)
            self.metadata['Replace'].append(replacement)
            self.nested[directory] = {'Module': {'Path': module}}
        (root / 'third_party' / 'README.md').write_text('bounded source patches\n')

    def mod(self, directory, environment):
        return self.nested.get(directory, self.metadata)

    def freeze(self, output):
        environment = {}
        with mock.patch.object(source, 'module_metadata', side_effect=self.mod):
            return source.freeze_server_source(self.root, output, environment)

    def change_manifest(self, change):
        path = self.root / 'third_party' / 'grpc' / 'WEIR_UPSTREAM.json'
        manifest = json.loads(path.read_text())
        change(manifest)
        path.write_text(json.dumps(manifest))


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
        (self.fixture.root / 'third_party' / 'unrelated').mkdir()
        (self.fixture.root / 'third_party' / 'unrelated' / 'unused.go').write_text('not allowed\n')
        tree, manifest = self.fixture.freeze(self.output)
        paths = {item['path'] for item in manifest}
        for relative in source.NESTED.values():
            for name in ('library.go', 'go.mod', 'go.sum', 'LICENSE', 'WEIR_UPSTREAM.json',
                         'WEIR_PATCH.diff', 'WEIR_PATCH.md'):
                self.assertIn(relative + '/' + name, paths)
        self.assertIn('third_party/grpc/credentials/alts/auth.go', paths)
        self.assertEqual((self.output / 'third_party/grpc/credentials/alts/auth.go').read_text(), 'package alts\n')
        self.assertNotIn('cmd/weir/main_test.go', paths)
        self.assertFalse(any('unrelated' in name or name.endswith('_test.go') for name in paths))
        source.verify_snapshot(self.output, manifest, tree)
        (self.output / 'cmd' / 'weir' / 'main.go').write_text('changed\n')
        with self.assertRaisesRegex(ValueError, 'hash mismatch'):
            source.verify_snapshot(self.output, manifest, tree)

    def test_legacy_lock_without_nested_replacements_remains_supported(self):
        self.fixture.metadata['Replace'] = []
        tree, manifest = self.fixture.freeze(self.output)
        self.assertEqual([entry['path'] for entry in manifest], ['cmd/weir/main.go', 'go.mod', 'go.sum'])
        source.verify_snapshot(self.output, manifest, tree)

    def test_missing_nested_module_and_metadata_fail_before_snapshot(self):
        for name in ('go.mod', 'go.sum', 'library.go', 'WEIR_UPSTREAM.json', 'WEIR_PATCH.diff', 'LICENSE'):
            with self.subTest(name=name):
                path = self.fixture.root / 'third_party' / 'grpc' / name
                data = path.read_bytes()
                path.unlink()
                with self.assertRaises((ValueError, FileNotFoundError)):
                    self.fixture.freeze(self.output)
                self.assertFalse(self.output.exists())
                path.write_bytes(data)

    def test_manifest_hash_and_nested_identity_mismatch_are_rejected(self):
        directory = self.fixture.root / 'third_party' / 'grpc'
        path = directory / 'library.go'
        path.write_text('changed production source\n')
        with self.assertRaisesRegex(ValueError, 'hash mismatch'):
            self.fixture.freeze(self.output)
        path.write_text('package library\n')
        self.fixture.nested[directory]['Module']['Path'] = 'wrong/module'
        with self.assertRaisesRegex(ValueError, 'identity'):
            self.fixture.freeze(self.output)

    def test_missing_or_extra_production_source_is_rejected(self):
        path = self.fixture.root / 'third_party' / 'grpc' / 'unexpected.go'
        path.write_text('package library\n')
        with self.assertRaisesRegex(ValueError, 'differs from manifest'):
            self.fixture.freeze(self.output)

    def test_root_module_and_unexpected_nested_module_are_rejected(self):
        self.fixture.metadata['Module']['Path'] = 'wrong/module'
        with self.assertRaisesRegex(ValueError, 'wrong module'):
            self.fixture.freeze(self.output)
        self.fixture.metadata['Module']['Path'] = source.SERVER_MODULE
        (self.fixture.root / 'cmd' / 'weir' / 'go.mod').write_text('module unexpected\n')
        with self.assertRaisesRegex(ValueError, 'unexpected nested module'):
            self.fixture.freeze(self.output)

    def test_replacements_are_exact_and_cannot_escape(self):
        original = self.fixture.metadata['Replace'][0]
        variants = [
            {'Old': {'Path': 'unexpected/module'}, 'New': {'Path': '../outside'}},
            {'Old': {'Path': 'google.golang.org/grpc'}, 'New': {'Path': '../grpc'}},
            {'Old': {'Path': 'google.golang.org/grpc', 'Version': 'v1.0.0'},
             'New': {'Path': './third_party/grpc'}},
            {'Old': {'Path': 'google.golang.org/grpc'},
             'New': {'Path': './third_party/grpc', 'Version': 'v2.0.0'}},
        ]
        for variant in variants:
            with self.subTest(variant=variant):
                self.fixture.metadata['Replace'][0] = variant
                with self.assertRaisesRegex(ValueError, 'replacement'):
                    self.fixture.freeze(self.output)
        self.fixture.metadata['Replace'][0] = original
        directory = self.fixture.root / 'third_party' / 'grpc'
        replacement = {'Old': {'Path': 'x'}, 'New': {'Path': '../escape'}}
        self.fixture.nested[directory]['Replace'] = [replacement]
        with self.assertRaisesRegex(ValueError, 'replacement'):
            self.fixture.freeze(self.output)

    def test_manifest_identity_path_and_hash_fail_closed(self):
        path = self.fixture.root / 'third_party' / 'grpc' / 'WEIR_UPSTREAM.json'
        original = path.read_text()
        variants = [
            {'module': 'wrong/module'}, {'version': 'v9.0.0'},
            {'files': {'../escape.go': 'a' * 64}}, {'files': {'/outside.go': 'a' * 64}},
            {'files': {'credentials/input.go': 'a' * 64}}, {'files': {'library.go': 'invalid'}},
            {'newFiles': [{'path': 'injected.pem', 'sha256': 'a' * 64}]},
            {'newFiles': [{'path': 'library.go', 'sha256': 'a' * 64}]},
        ]
        for variant in variants:
            with self.subTest(variant=variant):
                self.fixture.change_manifest(lambda manifest: manifest.update(variant))
                with self.assertRaises(ValueError):
                    self.fixture.freeze(self.output)
                self.assertFalse(self.output.exists())
                path.write_text(original)

    def test_symlinks_are_rejected_not_silently_omitted(self):
        path = self.fixture.root / 'third_party' / 'grpc' / 'library.go'
        path.unlink()
        path.symlink_to(self.fixture.root / 'cmd' / 'weir' / 'main.go')
        with self.assertRaisesRegex(ValueError, 'symlink'):
            self.fixture.freeze(self.output)
        path.unlink()
        path.write_text('package library\n')
        (self.fixture.root / 'internal').symlink_to(self.fixture.root / 'cmd', target_is_directory=True)
        with self.assertRaisesRegex(ValueError, 'symlink'):
            self.fixture.freeze(self.output)

    def test_public_grpc_credentials_cannot_read_secret_files_or_unmanifested_go(self):
        manifest_path = self.fixture.root / 'third_party/grpc/WEIR_UPSTREAM.json'
        original = manifest_path.read_text()
        secret_paths = ('credentials', 'credentials/token.pem', 'credentials/token.key',
                        'credentials/.env', 'credentials/.env.local', 'credentials/credentials')
        for name in secret_paths:
            with self.subTest(name=name):
                entry = dict(path=name, sha256='a' * 64)
                self.fixture.change_manifest(lambda manifest: manifest['newFiles'].append(entry))
                with mock.patch.object(source, 'read_file', wraps=source.read_file) as reader:
                    with self.assertRaises(ValueError):
                        self.fixture.freeze(self.output)
                requested = 'third_party/grpc/' + name
                self.assertFalse(any(call.args[1] == requested for call in reader.call_args_list))
                self.assertFalse(self.output.exists())
                manifest_path.write_text(original)
        extra = self.fixture.root / 'third_party/grpc/credentials/alts/unmanifested.go'
        extra.write_text('package alts\n')
        with mock.patch.object(source, 'read_file', wraps=source.read_file) as reader:
            with self.assertRaisesRegex(ValueError, 'differs from manifest'):
                self.fixture.freeze(self.output)
        self.assertFalse(any(call.args[1].endswith('/unmanifested.go') for call in reader.call_args_list))
        self.assertFalse(self.output.exists())

    def test_credentials_package_exception_does_not_apply_to_golua(self):
        directory = self.fixture.root / 'third_party/golua'
        path = directory / 'WEIR_UPSTREAM.json'
        manifest = json.loads(path.read_text())
        entry = dict(path='credentials/input.go', sha256='a' * 64)
        manifest['newFiles'].append(entry)
        path.write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, 'unsafe server source path'):
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

    def test_identity_rejects_wrong_source_toolchain_target_and_release_state(self):
        revision = 'a' * 40
        identity = dict(product='weir', revision=revision, version='local-' + revision,
                        state='clean-commit', dirty='false', go='go1.27.1', target='darwin/arm64')
        toolchain = ('go1.27.1', 'darwin/arm64')
        source.validate_identity(identity, toolchain, revision)
        for name, value in [('product', 'other'), ('revision', 'b' * 40), ('go', 'go1.26.0'),
                            ('target', 'linux/amd64'), ('state', 'dev'), ('dirty', 'true')]:
            with self.subTest(name=name):
                changed = dict(identity)
                changed[name] = value
                with self.assertRaises(ValueError):
                    source.validate_identity(changed, toolchain, revision)
        with self.assertRaises(ValueError):
            source.validate_identity(identity, toolchain)


if __name__ == '__main__':
    unittest.main()
