import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import archive_workflow


class WorkflowArchiveTest(unittest.TestCase):
    def options(self, root):
        options = argparse.Namespace(repo='batchstream/weir-tests', run=123,
                                     source='a' * 40, output=root / 'archive')
        return options

    def metadata(self, **changes):
        metadata = dict(databaseId=123, status='completed', conclusion='success',
                        headSha='a' * 40, url='https://github.com/batchstream/weir-tests/actions/runs/123')
        metadata.update(changes)
        return json.dumps(metadata).encode()

    def download(self, command, **kwargs):
        self.assertEqual(kwargs['timeout'], 300)
        directory = Path(command[command.index('--dir') + 1])
        target = directory / 'artifact' / 'nested' / 'report.json'
        target.parent.mkdir(parents=True)
        target.write_bytes(b'complete raw evidence\n')
        result = subprocess.CompletedProcess(command, 0, stdout='', stderr='')
        return result

    def verify(self, output):
        inventory = json.loads((output / 'files.json').read_text())
        self.assertEqual({entry['path'] for entry in inventory},
                         {path.relative_to(output).as_posix() for path in output.rglob('*.gz')})
        for entry in inventory:
            compressed = (output / entry['path']).read_bytes()
            raw = gzip.decompress(compressed)
            self.assertEqual(hashlib.sha256(compressed).hexdigest(), entry['gzip_sha256'])
            self.assertEqual(hashlib.sha256(raw).hexdigest(), entry['sha256'])
            self.assertEqual(len(raw), entry['bytes'])
            self.assertEqual(compressed[4:8], b'\0' * 4)
        return inventory

    def test_success_and_negative_conclusions_retain_all_raw_paths_and_both_hashes(self):
        for conclusion in ('success', 'failure', 'cancelled', 'timed_out'):
            with self.subTest(conclusion=conclusion), tempfile.TemporaryDirectory() as directory:
                options = self.options(Path(directory))
                with patch.object(archive_workflow.subprocess, 'check_output', return_value=self.metadata(conclusion=conclusion)), \
                        patch.object(archive_workflow.subprocess, 'run', side_effect=self.download):
                    summary = archive_workflow.archive(options)
                self.assertEqual(summary['conclusion'], conclusion)
                self.assertTrue(summary['artifact_download_complete'])
                inventory = self.verify(options.output)
                self.assertEqual(len(inventory), 3)
                report = options.output / 'artifacts/artifact/nested/report.json.gz'
                self.assertEqual(gzip.decompress(report.read_bytes()), b'complete raw evidence\n')

    def test_wrong_head_run_url_and_incomplete_workflow_are_rejected_before_download(self):
        changes = [dict(headSha='b' * 40), dict(databaseId=124), dict(url='https://example.invalid/123'),
                   dict(status='in_progress'), dict(conclusion=None)]
        for change in changes:
            with self.subTest(change=change), tempfile.TemporaryDirectory() as directory:
                options = self.options(Path(directory))
                with patch.object(archive_workflow.subprocess, 'check_output', return_value=self.metadata(**change)), \
                        patch.object(archive_workflow.subprocess, 'run') as download:
                    with self.assertRaises(ValueError):
                        archive_workflow.archive(options)
                download.assert_not_called()
                self.assertFalse(options.output.exists())

    def test_existing_output_and_invalid_run_are_never_modified(self):
        with tempfile.TemporaryDirectory() as directory:
            options = self.options(Path(directory))
            options.output.mkdir()
            marker = options.output / 'existing'
            marker.write_text('preserve')
            with patch.object(archive_workflow.subprocess, 'check_output') as request:
                with self.assertRaises(FileExistsError):
                    archive_workflow.archive(options)
            request.assert_not_called()
            self.assertEqual(marker.read_text(), 'preserve')
            options.output = Path(directory) / 'different'
            options.run = -1
            with self.assertRaises(ValueError):
                archive_workflow.archive(options)

    def test_parent_secret_directories_and_symlinks_are_rejected_before_payload_open(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for index, relative in enumerate(('.env.private/file.txt', 'credentials/nested/data.json',
                                              '.ssh/config', 'certs/client.KEY', 'certificate.pem/data.json')):
                parent = root / str(index)
                target = parent / relative
                target.parent.mkdir(parents=True)
                target.write_text('synthetic fixture must never be read')
                with patch.object(Path, 'open', side_effect=AssertionError('payload opened')):
                    with self.assertRaises(ValueError):
                        archive_workflow.artifact_files(parent)
            ordinary = root / 'ordinary'
            ordinary.mkdir()
            (ordinary / 'link').symlink_to(root / 'missing')
            with patch.object(Path, 'open', side_effect=AssertionError('symlink opened')):
                with self.assertRaises(ValueError):
                    archive_workflow.artifact_files(ordinary)

    def test_partial_download_failure_is_retained_with_metadata_and_raw_files(self):
        with tempfile.TemporaryDirectory() as directory:
            options = self.options(Path(directory))

            def partial(command, **kwargs):
                result = self.download(command, **kwargs)
                result.returncode, result.stderr = 1, 'download interrupted'
                return result

            with patch.object(archive_workflow.subprocess, 'check_output', return_value=self.metadata()), \
                    patch.object(archive_workflow.subprocess, 'run', side_effect=partial):
                summary = archive_workflow.archive(options)
            self.assertFalse(summary['artifact_download_complete'])
            self.verify(options.output)
            receipt = json.loads(gzip.decompress((options.output / 'download.json.gz').read_bytes()))
            self.assertEqual(receipt['exit_code'], 1)
            self.assertIn('interrupted', receipt['stderr'])
            self.assertTrue((options.output / 'workflow.json.gz').is_file())
            self.assertTrue((options.output / 'artifacts/artifact/nested/report.json.gz').is_file())

    def test_download_timeout_retains_auditable_failure_receipt(self):
        with tempfile.TemporaryDirectory() as directory:
            options = self.options(Path(directory))
            error = subprocess.TimeoutExpired(['gh', 'run', 'download'], 300, output=b'partial output', stderr=b'timeout detail')
            with patch.object(archive_workflow.subprocess, 'check_output', return_value=self.metadata()), \
                    patch.object(archive_workflow.subprocess, 'run', side_effect=error):
                summary = archive_workflow.archive(options)
            self.assertFalse(summary['artifact_download_complete'])
            self.verify(options.output)
            receipt = json.loads(gzip.decompress((options.output / 'download.json.gz').read_bytes()))
            self.assertIsNone(receipt['exit_code'])
            self.assertEqual(receipt['stdout'], 'partial output')
            self.assertIn('300 seconds', receipt['error'])

    def test_cli_partial_download_exits_unsuccessfully_after_retaining_metadata(self):
        with tempfile.TemporaryDirectory() as directory:
            options = self.options(Path(directory))
            arguments = ['archive_workflow.py', '--run', str(options.run), '--source', options.source,
                         '--output', str(options.output)]
            result = subprocess.CompletedProcess(['gh'], 1, stdout='', stderr='artifact not available')
            with patch('sys.argv', arguments), patch('sys.stdout', new_callable=io.StringIO) as stdout, \
                    patch.object(archive_workflow.subprocess, 'check_output', return_value=self.metadata()), \
                    patch.object(archive_workflow.subprocess, 'run', return_value=result):
                with self.assertRaises(SystemExit):
                    archive_workflow.main()
            self.assertFalse(json.loads(stdout.getvalue())['artifact_download_complete'])
            self.verify(options.output)
            self.assertTrue((options.output / 'workflow.json.gz').is_file())


if __name__ == '__main__':
    unittest.main()
