"""Offline regression checks for module preparation and immutable inputs."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import download_modules
from server_source_test import ServerFixture


class DownloadModulesTests(unittest.TestCase):
    def test_candidate_cli_selects_candidate_graph(self):
        source = Path('.tools/server-candidate')
        arguments = ['download_modules.py', '--server-source', str(source)]
        with patch('sys.argv', arguments), patch.object(download_modules, 'download_modules') as prepare:
            download_modules.main()
        prepare.assert_called_once_with(source, server=True)

    def test_unused_checksums_stay_in_the_temporary_module(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            inputs = {'go.mod': b'module example.com/test\n', 'go.sum': b'fixed checksums\n'}
            for name, content in inputs.items():
                (root / name).write_bytes(content)

            def prepare(args, **kwargs):
                self.assertEqual(args, ['go', 'mod', 'download', 'all'])
                work = kwargs['cwd']
                self.assertNotEqual(work, root)
                self.assertEqual(kwargs['env']['GOWORK'], 'off')
                self.assertEqual(kwargs['env']['GOTOOLCHAIN'], 'local')
                self.assertEqual(kwargs['env']['GOFLAGS'], '-mod=readonly')
                self.assertEqual(kwargs['env']['GOPROXY'], 'off')
                self.assertEqual(kwargs['env']['GOSUMDB'], 'off')
                for name, content in inputs.items():
                    self.assertEqual((work / name).read_bytes(), content)
                (work / 'go.sum').write_bytes(b'additional unused graph checksums\n')

            env = dict(GOWORK='/local/go.work', GOPROXY='off', GOSUMDB='off')
            with patch.dict(os.environ, env), patch.object(download_modules.subprocess, 'run', side_effect=prepare) as command:
                download_modules.download_modules(root)
            self.assertFalse(command.call_args.kwargs['cwd'].exists())
            self.assertEqual(inputs, {name: (root / name).read_bytes() for name in inputs})

    def test_download_failure_preserves_fixed_inputs(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            inputs = {'go.mod': b'module example.com/test\n', 'go.sum': b'fixed checksums\n'}
            for name, content in inputs.items():
                (root / name).write_bytes(content)
            failure = subprocess.CalledProcessError(1, ['go', 'mod', 'download', 'all'])
            with patch.object(download_modules.subprocess, 'run', side_effect=failure) as command:
                with self.assertRaises(subprocess.CalledProcessError):
                    download_modules.download_modules(root)
            self.assertFalse(command.call_args.kwargs['cwd'].exists())
            self.assertEqual(inputs, {name: (root / name).read_bytes() for name in inputs})

    def test_candidate_graph_keeps_validated_nested_replacements_and_public_checksums(self):
        with tempfile.TemporaryDirectory() as directory:
            fixture = ServerFixture(Path(directory) / 'candidate')
            original = {path.relative_to(fixture.root): path.read_bytes()
                        for path in fixture.root.rglob('go.*')}

            def prepare(args, **kwargs):
                self.assertEqual(args, ['go', 'mod', 'download', 'all'])
                work, env = kwargs['cwd'], kwargs['env']
                self.assertNotEqual(work, fixture.root)
                for name, content in original.items():
                    self.assertEqual((work / name).read_bytes(), content)
                for name, expected in dict(GOENV='off', GOWORK='off', GOTOOLCHAIN='local',
                                           GOFLAGS='-mod=readonly', GOPROXY='https://proxy.golang.org',
                                           GOSUMDB='sum.golang.org', GOPRIVATE='', GONOPROXY='',
                                           GONOSUMDB='', GOINSECURE='').items():
                    self.assertEqual(env[name], expected)
                (work / 'go.sum').write_bytes(b'temporary graph checksums\n')
                (work / 'third_party/grpc/go.sum').write_bytes(b'temporary nested checksums\n')

            private = dict(GOPROXY='off', GOSUMDB='off', GOPRIVATE='*', GONOSUMDB='*')
            with patch.dict(os.environ, private), \
                    patch('server_source.module_metadata', side_effect=fixture.mod), \
                    patch.object(download_modules.subprocess, 'run', side_effect=prepare) as command:
                download_modules.download_modules(fixture.root, server=True)
            self.assertFalse(command.call_args.kwargs['cwd'].exists())
            self.assertEqual(original, {name: (fixture.root / name).read_bytes() for name in original})

    def test_invalid_nested_patch_is_rejected_before_candidate_download(self):
        with tempfile.TemporaryDirectory() as directory:
            fixture = ServerFixture(Path(directory) / 'candidate')
            (fixture.root / 'third_party/grpc/library.go').write_text('package changed\n')
            with patch('server_source.module_metadata', side_effect=fixture.mod), \
                    patch.object(download_modules.subprocess, 'run') as command:
                with self.assertRaises(ValueError):
                    download_modules.download_modules(fixture.root, server=True)
            command.assert_not_called()

    def test_candidate_download_failure_keeps_source_metadata_unchanged(self):
        with tempfile.TemporaryDirectory() as directory:
            fixture = ServerFixture(Path(directory) / 'candidate')
            original = {path.relative_to(fixture.root): path.read_bytes()
                        for path in fixture.root.rglob('go.*')}
            failure = subprocess.CalledProcessError(1, ['go', 'mod', 'download', 'all'])
            with patch('server_source.module_metadata', side_effect=fixture.mod), \
                    patch.object(download_modules.subprocess, 'run', side_effect=failure) as command:
                with self.assertRaises(subprocess.CalledProcessError):
                    download_modules.download_modules(fixture.root, server=True)
            self.assertFalse(command.call_args.kwargs['cwd'].exists())
            self.assertEqual(original, {name: (fixture.root / name).read_bytes() for name in original})

    def test_checkout_mutation_is_still_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'go.mod').write_text('module example.com/test\n')
            (root / 'go.sum').write_text('fixed checksums\n')

            def change_checkout(*args, **kwargs):
                (root / 'go.sum').write_text('unexpected checkout modification\n')

            with patch.object(download_modules.subprocess, 'run', side_effect=change_checkout):
                with self.assertRaisesRegex(RuntimeError, 'module preparation changed go.sum'):
                    download_modules.download_modules(root)


if __name__ == '__main__':
    unittest.main()
