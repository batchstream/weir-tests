"""Offline regression checks for module preparation and immutable inputs."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import download_modules


class DownloadModulesTests(unittest.TestCase):
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
