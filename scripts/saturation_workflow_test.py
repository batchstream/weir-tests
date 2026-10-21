import contextlib
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import textwrap
import unittest
from unittest import mock

import performance_tools_test as tools


ROOT = Path(__file__).resolve().parent.parent
WORKFLOW = ROOT / '.github/workflows/saturation.yml'


def python_blocks():
    text = WORKFLOW.read_text()
    blocks = re.findall(r"          python3 - <<'PY'\n(.*?)          PY", text, re.S)
    return [textwrap.dedent(block) for block in blocks]


class SaturationWorkflowTests(unittest.TestCase):
    def test_embedded_programs_and_shell_syntax(self):
        blocks = python_blocks()
        self.assertEqual(len(blocks), 3)
        for block in blocks:
            compile(block, str(WORKFLOW), 'exec')
        for run in re.findall(r'        run: \|\n((?:          .*\n|\n)+)', WORKFLOW.read_text()):
            subprocess.run(['bash', '-n'], input=textwrap.dedent(run), text=True, check=True)

    def test_candidate_graph_preparation_precedes_offline_build(self):
        workflow = WORKFLOW.read_text()
        prepare = workflow.split('      - name: Prepare immutable tools and owned database images\n', 1)[1]
        prepare = prepare.split('      - name: Independent single-request native and SDK clients\n', 1)[0]
        verify = "check_checkout(pathlib.Path('.tools/server-candidate'), os.environ['SERVER_CANDIDATE'], build_environment())"
        graph = 'python3 scripts/download_modules.py --server-source .tools/server-candidate'
        build = 'GOPROXY=off GOSUMDB=off python3 scripts/build_local_server.py --source .tools/server-candidate --output .tools/weir'
        self.assertLess(prepare.index('if [ -n "$SERVER_CANDIDATE" ]; then'), prepare.index(verify))
        self.assertLess(prepare.index(verify), prepare.index(graph))
        self.assertLess(prepare.index(graph), prepare.index(build))
        self.assertLess(prepare.index(build), prepare.index('          else\n'))

    def test_candidate_and_platform_fail_closed(self):
        block = python_blocks()[0]
        cases = [('', 'Linux', 'x86_64', True), ('a' * 40, 'Linux', 'x86_64', True),
                 ('main', 'Linux', 'x86_64', False), ('A' * 40, 'Linux', 'x86_64', False),
                 ('a' * 40, 'Darwin', 'arm64', False), ('', 'Linux', 'aarch64', False)]
        for candidate, system, machine, valid in cases:
            with self.subTest(candidate=candidate, system=system, machine=machine), \
                    mock.patch.dict(os.environ, SERVER_CANDIDATE=candidate), \
                    mock.patch('platform.system', return_value=system), \
                    mock.patch('platform.machine', return_value=machine):
                if valid:
                    exec(block, {})
                else:
                    with self.assertRaises(SystemExit):
                        exec(block, {})

    def measurement(self, options):
        with tempfile.TemporaryDirectory() as directory, contextlib.chdir(directory):
            root = Path(directory)
            (root / '.tools').mkdir()
            (root / 'results/local').mkdir(parents=True)
            versions = json.loads((ROOT / 'versions.json').read_text())
            pin = versions['mongodb_linux_amd64']
            pin['member_sha256'] = hashlib.sha256(b'native mongo').hexdigest()
            (root / 'versions.json').write_text(json.dumps(versions))
            receipt = tools.make_receipt('local', versions)
            server = receipt['server_receipt']
            server['sha256'] = hashlib.sha256(b'candidate').hexdigest()
            server['identity'].update(product='weir', version='dev', state='dev', dirty='unknown',
                                      go='go' + versions['go'], target='linux/amd64')
            if options.get('profile'):
                server['source_dirty'] = False
            (root / '.tools/weir').write_bytes(b'candidate')
            (root / '.tools/weir.receipt.json').write_text(json.dumps(server))
            mongo = root / '.tools/mongod'
            mongo.write_bytes(b'native mongo')
            mongo_receipt = dict(target='linux/amd64', version=versions['mongodb_version'],
                                 archive_sha256=pin['sha256'], binary_sha256=pin['member_sha256'])
            if options.get('wrong_mongo'):
                mongo_receipt['version'] = 'wrong'
            mongo.with_suffix('.receipt.json').write_text(json.dumps(mongo_receipt))
            environment = dict(BACKEND='all', BACKEND_BATCH_LIMITS='32', MODE='saturation',
                               DATABASE_CPUS='1', CLIENT_LEVELS='8,32', BATCH_SIZES='1',
                               WRITE_PERCENT='10', LUA_MUTATIONS='false',
                               WEIR_TEST_MONGODB_BINARY=str(mongo), GITHUB_REPOSITORY='owner/test', GITHUB_RUN_ID='1')
            if options.get('profile'):
                environment.update(BACKEND='mongo', CPU_PROFILE_CONCURRENCY='32', SERVER_CANDIDATE=server['source'])
            environment.update(options.get('environment', {}))
            commands = []

            def run(command, **kwargs):
                self.assertEqual(command[0], 'go')
                Path(command[command.index('-o') + 1]).write_bytes(b'client')

            def popen(command, **kwargs):
                commands.append(command)
                target = Path(command[command.index('-output') + 1])
                if options.get('profile'):
                    retained = Path(str(target) + '.weir-profile-binary')
                    self.assertEqual(retained.read_bytes(), b'candidate')
                    self.assertFalse(target.exists(), 'profile retention must not pre-create the runner output directory')
                target.mkdir()
                for backend in ('mongo', 'search'):
                    report = tools.make_report(receipt)
                    if options.get('wrong_report'):
                        report['provenance']['weir_binary_sha256'] = '0' * 64
                    (target / (backend + '.json')).write_text(json.dumps(report))
                process = mock.Mock(stdout=[])
                process.wait.return_value = options.get('exit_code', 0)
                return process

            read_text = Path.read_text

            def host_read(path, *args, **kwargs):
                if str(path) == '/proc/meminfo':
                    return 'MemTotal: 1048576 kB\n'
                if str(path) == '/proc/cpuinfo':
                    return 'model name: offline fixture\n'
                return read_text(path, *args, **kwargs)

            with mock.patch.dict(os.environ, environment), \
                    mock.patch.object(Path, 'read_text', autospec=True, side_effect=host_read), \
                    mock.patch('build_local_server.freeze_source', return_value=('d' * 64, [])), \
                    mock.patch('subprocess.run', side_effect=run), \
                    mock.patch('subprocess.Popen', side_effect=popen), \
                    mock.patch('platform.uname', return_value=mock.Mock(_asdict=lambda: {})), \
                    mock.patch('subprocess.check_output', side_effect=['f' * 40, b'', 'go version', 'go build', options.get('pprof_raw', 'Samples:\nsamples/count cpu/nanoseconds\n 1 10000000: 1 2\n')]):
                if options.get('profile_error'):
                    with self.assertRaisesRegex(SystemExit, options['profile_error']):
                        exec(python_blocks()[2], {})
                    if options.get('exit_code'):
                        self.assertEqual((root / 'results/local/saturation-batch-32.weir-profile-binary').read_bytes(), b'candidate')
                        saved = json.loads((root / 'results/local/measurement-batch-32.receipt.json').read_text())
                        self.assertEqual(saved['exit_code'], options['exit_code'])
                    return
                if options.get('invalid_profile'):
                    with self.assertRaisesRegex(SystemExit, 'CPU diagnostic|cpu-profile-concurrency'):
                        exec(python_blocks()[2], {})
                    self.assertEqual(commands, [])
                    return
                if options.get('wrong_mongo'):
                    with self.assertRaisesRegex(SystemExit, 'native Mongo receipt'):
                        exec(python_blocks()[2], {})
                    self.assertEqual(commands, [])
                    return
                if options.get('wrong_report'):
                    with self.assertRaisesRegex(ValueError, 'weir_binary_sha256'):
                        exec(python_blocks()[2], {})
                    saved = json.loads((root / 'results/local/measurement-batch-32.receipt.json').read_text())
                    self.assertIn('validation_error', saved)
                    return
                exec(python_blocks()[2], {})
            self.assertEqual(len(commands), 1)
            command = commands[0]
            self.assertEqual(command[command.index('-mongod') + 1], str(mongo))
            self.assertEqual(command[command.index('-server-receipt') + 1], '.tools/weir.receipt.json')
            saved = json.loads((root / 'results/local/measurement-batch-32.receipt.json').read_text())
            self.assertEqual(saved['server_receipt_kind'], 'local')
            self.assertIn('not matched fixed offered rate', saved['load_model'])
            self.assertEqual(saved['native_mongodb_receipt'], mongo_receipt)
            if options.get('profile'):
                self.assertEqual(command[command.index('-cpu-profile-concurrency') + 1], '32')
                self.assertEqual(command[command.index('-cpu-profile-duration') + 1], '60s')
                self.assertIn('non-baseline', saved['measurement_classification'])
                self.assertEqual((root / 'results/local/saturation-batch-32.weir-profile-binary').read_bytes(), b'candidate')
                self.assertIn('10000000', (root / 'results/local/saturation-batch-32/weir-cpu.raw.txt').read_text())
            else:
                self.assertNotIn('-cpu-profile-concurrency', command)
                self.assertNotIn('measurement_classification', saved)

    def test_opt_in_cpu_profile_reuses_existing_measurement(self):
        options = dict(profile=True)
        self.measurement(options)

    def test_cpu_binary_retained_before_launch_even_when_later_stage_fails(self):
        options = dict(profile=True, exit_code=7, profile_error='7')
        self.measurement(options)

    def test_cpu_capture_rejects_wrong_candidate_receipt(self):
        options = dict(profile=True, environment=dict(SERVER_CANDIDATE='0' * 40),
                       profile_error='exact candidate source receipt')
        self.measurement(options)

    def test_cpu_capture_requires_positive_cpu_samples(self):
        for raw in ('Samples:\nsamples/count cpu/nanoseconds\n',
                    'Samples:\nsamples/count cpu/nanoseconds\n 0 0: 1 2\n'):
            with self.subTest(raw=raw):
                options = dict(profile=True, pprof_raw=raw, profile_error='no positive CPU samples')
                self.measurement(options)

    def test_cpu_profile_scope_fails_before_build_or_measurement(self):
        cases = [dict(CPU_PROFILE_CONCURRENCY='bogus'),
                 dict(CPU_PROFILE_CONCURRENCY='32'),
                 dict(CPU_PROFILE_CONCURRENCY='16', BACKEND='mongo', SERVER_CANDIDATE='a' * 40),
                 dict(CPU_PROFILE_CONCURRENCY='32', BACKEND='mongo', SERVER_CANDIDATE='a' * 40, MODE='bulk-saturation'),
                 dict(CPU_PROFILE_CONCURRENCY='32', BACKEND='mongo', SERVER_CANDIDATE='a' * 40, BACKEND_BATCH_LIMITS='1,32')]
        for environment in cases:
            with self.subTest(environment=environment):
                options = dict(invalid_profile=True, environment=environment)
                self.measurement(options)

    def test_candidate_receipt_and_native_mongo_reach_existing_engine(self):
        self.measurement({})

    def test_wrong_native_fixture_is_rejected_before_measurement(self):
        options = dict(wrong_mongo=True)
        self.measurement(options)

    def test_wrong_report_provenance_retains_failure(self):
        options = dict(wrong_report=True)
        self.measurement(options)


if __name__ == '__main__':
    unittest.main()
