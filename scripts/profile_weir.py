#!/usr/bin/env python3
"""Run a short CPU diagnostic with the pinned integration-only Weir profiler.

These instrumented results are diagnostics, never saturated-capacity evidence.
The normal comparison continues to use the uninstrumented cmd/weir binary.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile

from prepare_weir import ROOT, validate_source


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--offline', action='store_true')
    parser.add_argument('--output', default='results/local/cpu-diagnostic')
    args = parser.parse_args()
    pins = json.loads((ROOT / 'versions.json').read_text())
    output = ROOT / args.output
    output.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, GOENV='off', GOWORK='off', GOTOOLCHAIN='local', GOFLAGS='-mod=readonly', CGO_ENABLED='0', WEIR_CAPACITY_INTEGRATION='1')
    env.pop('GOROOT', None)
    if args.offline:
        env.update(GOPROXY='off', GOSUMDB='off')
    toolchain = subprocess.check_output(['go', 'version'], env=env, text=True, timeout=10).strip()
    if not toolchain.startswith('go version go' + pins['go'] + ' '):
        raise ValueError('profiler requires the locked native Go toolchain')
    query = 'github.com/batchstream/weir@' + pins['weir_version']
    with tempfile.TemporaryDirectory(prefix='weir-profile-module-') as temporary:
        directory = Path(temporary)
        (directory / 'go.mod').write_text('module local.invalid/weir-profile\n\ngo ' + pins['go'] + '\n')
        raw = subprocess.check_output(['go', 'mod', 'download', '-json', query], cwd=directory, env=env, text=True, timeout=180)
    module = json.loads(raw)
    validate_source(pins, module)
    source = Path(module['Dir'])
    subprocess.run(['go', 'mod', 'verify'], cwd=source, env=env, check=True, timeout=180)
    helper = ROOT / '.tools' / 'weir-profile-node'
    subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-tags=integration', '-o', str(helper), './internal/testutil/testcapacity'], cwd=source, env=env, check=True, timeout=600)
    # This identity describes an integration helper built from the verified
    # source. It deliberately does not claim to be the production executable.
    identity = dict(product='weir-integration-profiler', revision=pins['weir_source'], instrumented=True)
    wrapper = output / 'weir-profile'
    wrapper.write_text(
        '#!/usr/bin/env python3\n'
        'import json, os, pathlib, sys\n'
        'identity = ' + repr(identity) + '\n'
        'if sys.argv[1:] == ["version"]:\n'
        '    print(json.dumps(identity)); raise SystemExit(0)\n'
        'if len(sys.argv) != 6 or sys.argv[1] != "serve" or sys.argv[2] != "--config" or sys.argv[4] != "--routes":\n'
        '    raise SystemExit("profiler only supports owned fixture serve/version")\n'
        'profile = pathlib.Path(sys.argv[3]).with_suffix(".cpu.pprof")\n'
        'helper = ' + repr(str(helper)) + '\n'
        'os.execv(helper, [helper, "-mode", "profile-node", "-config", sys.argv[3], "-routes", sys.argv[5], "-cpu-profile", str(profile)])\n'
    )
    wrapper.chmod(0o755)
    lab = ROOT / '.tools' / 'weir-profile-lab'
    subprocess.run(['go', 'build', '-o', str(lab), './cmd/weir-lab'], cwd=ROOT, env=env, check=True, timeout=180)
    command = [str(lab), '-mode', 'saturation', '-weir', str(wrapper), '-backend', 'mongo', '-database-cpus', '1', '-store-concurrency', '32', '-concurrency-levels', '8,32', '-batch-sizes', '32', '-records', '2048', '-payload-bytes', '1024', '-write-percent', '0', '-rounds', '1', '-warmup-duration', '3s', '-duration', '10s', '-output', str(output / 'workload')]
    receipt = dict(diagnostic_only=True, capacity_evidence=False, identity=identity, versions=pins, helper_sha256=hashlib.sha256(helper.read_bytes()).hexdigest(), wrapper_sha256=hashlib.sha256(wrapper.read_bytes()).hexdigest(), helper_build=subprocess.check_output(['go', 'version', '-m', str(helper)], env=env, text=True), source=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(), source_dirty=bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT)), command=command)
    (output / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    with (output / 'workload.log').open('w') as log:
        process = subprocess.Popen(command, cwd=ROOT, env=env, stdout=log, stderr=subprocess.STDOUT)
        try:
            result = process.wait(timeout=180)
        except subprocess.TimeoutExpired:
            process.send_signal(signal.SIGTERM)
            try:
                result = process.wait(timeout=60)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=10)
                receipt['failure'] = 'diagnostic and graceful cleanup exceeded their deadlines; inspect owned fixture receipts'
                (output / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
                raise RuntimeError(receipt['failure'])
            receipt['failure'] = 'diagnostic timed out; lab exited after SIGTERM; inspect owned cleanup receipts'
            (output / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
            raise RuntimeError(receipt['failure'])
        if result != 0:
            raise subprocess.CalledProcessError(result, command)
    report = json.loads((output / 'workload' / 'mongo.json').read_text())
    fixture = Path(report['provenance']['fixture_logs'])
    manifest = json.loads((fixture / 'manifest.json').read_text())
    cleanup = json.loads((fixture / 'cleanup.json').read_text())
    if cleanup.get('errors') or cleanup['owner'] != manifest['owner'] or any(not item['stopped'] for item in cleanup['processes']) or any(not item['removed'] for item in cleanup['containers']):
        raise RuntimeError('profile fixture did not cleanly release owned resources')
    profiles = sorted(fixture.glob('*.pprof'))
    if not any(profile.name.endswith('.cpu.pprof') for profile in profiles):
        raise RuntimeError('profile fixture produced no CPU profiles')
    for profile in profiles:
        target = output / profile.name
        shutil.copyfile(profile, target)
        top = subprocess.check_output(['go', 'tool', 'pprof', '-top', '-nodecount=35', str(target)], env=env, text=True, timeout=30)
        (output / (profile.stem + '.top.txt')).write_text(top)
        print(top)
    for name in ['manifest.json', 'cleanup.json']:
        shutil.copyfile(fixture / name, output / name)


if __name__ == '__main__':
    main()
