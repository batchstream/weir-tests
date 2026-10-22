import hashlib
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest import mock

import prepare_native_mongo as native


class NativeMongoTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.payload = b'fixture binary content, never executed'
        self.member = 'mongodb-linux-x86_64-ubuntu2404-8.0.32/bin/mongod'
        self.archive = self.make_archive()
        self.profile = dict(url='https://fixture.invalid/mongo.tgz',
                            sha256=hashlib.sha256(self.archive).hexdigest(), size=len(self.archive),
                            member=self.member, member_size=len(self.payload),
                            member_sha256=hashlib.sha256(self.payload).hexdigest())
        profiles = {'linux/amd64': self.profile}
        for arch in ('arm64', 'amd64'):
            profiles['darwin/' + arch] = dict(self.profile, url='https://fixture.invalid/darwin-' + arch, sha256=arch)
        self.pins = dict(mongodb_version='8.0.32', mongodb_native=profiles)
        (self.root / 'versions.json').write_text(json.dumps(self.pins))
        for name, value in [('system', 'Linux'), ('machine', 'x86_64'),
                            ('freedesktop_os_release', {'ID': 'ubuntu', 'VERSION_ID': '24.04'})]:
            patch = mock.patch.object(native.platform, name, return_value=value)
            patch.start()
            self.addCleanup(patch.stop)
        self.download = mock.patch.object(native.urllib.request, 'urlopen',
                                          side_effect=lambda *args, **kwargs: io.BytesIO(self.archive)).start()
        self.addCleanup(mock.patch.stopall)
        self.execute = mock.patch.object(native.subprocess, 'check_output',
                                         return_value='Build Info: {"version":"8.0.32"}').start()

    def make_archive(self, kind=None, duplicate=False, name=None):
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode='w:gz') as tar:
            for _ in range(2 if duplicate else 1):
                member = tarfile.TarInfo(name or self.member)
                if kind is not None:
                    member.type = kind
                    member.linkname = '/outside'
                    tar.addfile(member)
                else:
                    member.size = len(self.payload)
                    tar.addfile(member, io.BytesIO(self.payload))
        return output.getvalue()

    def rewrite_pins(self):
        (self.root / 'versions.json').write_text(json.dumps(self.pins))

    def test_linux_verified_download_and_cache(self):
        output = native.prepare(self.root)
        self.assertEqual(output.read_bytes(), self.payload)
        self.assertEqual(native.prepare(self.root), output)
        self.download.assert_called_once_with(self.profile['url'], timeout=60)
        self.execute.assert_called_once()
        metadata = json.loads(output.with_suffix('.json').read_text())
        self.assertEqual(metadata['target'], 'linux/amd64')
        self.assertEqual(metadata['binary_sha256'], self.profile['member_sha256'])
        output.write_bytes(b'tampered')
        with self.assertRaises(ValueError):
            native.prepare(self.root)
        self.assertEqual(output.read_bytes(), b'tampered')

    def test_binary_without_metadata_is_preserved(self):
        output = native.prepare(self.root)
        output.with_suffix('.json').unlink()
        with self.assertRaisesRegex(ValueError, 'no metadata'):
            native.prepare(self.root)
        self.assertEqual(output.read_bytes(), self.payload)

    def test_unsupported_platforms_fail_closed(self):
        for system, machine, release in [('Linux', 'arm64', '24.04'),
                                         ('Linux', 'x86_64', '22.04'),
                                         ('Windows', 'x86_64', '24.04')]:
            with self.subTest(system=system, machine=machine, release=release):
                with mock.patch.object(native.platform, 'system', return_value=system), \
                     mock.patch.object(native.platform, 'machine', return_value=machine), \
                     mock.patch.object(native.platform, 'freedesktop_os_release',
                                       return_value={'ID': 'ubuntu', 'VERSION_ID': release}):
                    with self.assertRaises(ValueError):
                        native.prepare(self.root)
        self.download.assert_not_called()
        self.execute.assert_not_called()

    def test_darwin_profiles_are_selected_by_architecture(self):
        with mock.patch.object(native.platform, 'system', return_value='Darwin'):
            for machine, arch in [('arm64', 'arm64'), ('x86_64', 'amd64')]:
                with mock.patch.object(native.platform, 'machine', return_value=machine):
                    profile = native.select_profile(self.pins)
                    self.assertEqual(profile['target'], 'darwin/' + arch)
                    self.assertEqual(profile['sha256'], arch)
                    self.assertEqual(profile['version'], '8.0.32')

    def test_hash_size_member_and_version_rejections(self):
        for field, wrong in [('sha256', '0' * 64), ('size', len(self.archive) - 1),
                             ('member', 'other/bin/mongod'), ('member_size', len(self.payload) + 1),
                             ('member_sha256', '0' * 64)]:
            with self.subTest(field=field):
                original = self.profile[field]
                self.profile[field] = wrong
                self.rewrite_pins()
                with self.assertRaises(ValueError):
                    native.prepare(self.root)
                self.profile[field] = original
                self.rewrite_pins()
                self.assertFalse((self.root / '.tools' / 'mongod').exists())
                self.execute.assert_not_called()
        self.execute.return_value = 'Build Info: {"version":"wrong"}'
        with self.assertRaises(ValueError):
            native.prepare(self.root)
        self.assertFalse((self.root / '.tools' / 'mongod').exists())

    def test_links_duplicates_and_wrong_member_never_extracted(self):
        for raw in [self.make_archive(kind=tarfile.SYMTYPE),
                    self.make_archive(kind=tarfile.LNKTYPE),
                    self.make_archive(duplicate=True),
                    self.make_archive(name='unlocked/bin/mongod')]:
            archive = self.root / 'input.tgz'
            archive.write_bytes(raw)
            with self.assertRaises(ValueError):
                native.extract_binary(archive, self.root / 'output', self.profile)
        self.execute.assert_not_called()

    def test_bounded_copy(self):
        with self.assertRaises(ValueError):
            native.copy_bounded(io.BytesIO(b'12345'), io.BytesIO(), 4)

    def test_ci_exports_explicit_fixture_for_integration_and_smokes(self):
        root = Path(__file__).resolve().parent.parent
        workflow = (root / '.github/workflows/ci.yml').read_text()
        prepare = workflow.index('mongod=$(python3 scripts/prepare_native_mongo.py)')
        export = workflow.index('WEIR_TEST_MONGODB_BINARY=%s')
        self.assertLess(prepare, export)
        for step in ['- name: Real multi-node integration', '- name: Verified comparison smoke',
                     '- name: Verified single-record Lua RMW comparison smoke']:
            self.assertLess(export, workflow.index(step))
        self.assertIn('"$mongod" >> "$GITHUB_ENV"', workflow)
        pins = json.loads((root / 'versions.json').read_text())
        self.assertEqual(sorted(pins['mongodb_native']), ['darwin/amd64', 'darwin/arm64', 'linux/amd64'])
        for target, profile in pins['mongodb_native'].items():
            with self.subTest(target=target):
                self.assertEqual(sorted(profile), sorted(self.profile))
                self.assertIn(pins['mongodb_version'], profile['url'])


if __name__ == '__main__':
    unittest.main()
