#!/usr/bin/env python3
"""Run the read, mixed, write and Lua weir-lab saturation cases, plus optional bulk cases, in sequence."""
import argparse
from pathlib import Path
import subprocess


CASES = [('read', 0, False, 'saturation'), ('mixed', 10, False, 'saturation'),
         ('write', 100, False, 'saturation'), ('lua', 100, True, 'saturation')]
BULK_CASES = [('bulk-read', 0, False, 'bulk-saturation'), ('bulk-write', 100, False, 'bulk-saturation')]


def case_command(client, args, case):
    name, writes, lua, mode = case
    bulk = mode == 'bulk-saturation'
    command = [str(client), '-mode', mode, '-weir', str(args.weir.resolve()), '-backend', args.backend,
               '-client-processes', str(args.client_processes),
               '-concurrency-levels', args.bulk_concurrency_levels if bulk else args.concurrency_levels,
               '-batch-sizes', '32' if bulk else '1',
               '-records', str(args.records), '-payload-bytes', str(args.payload_bytes), '-rounds', str(args.rounds),
               '-warmup-duration', args.warmup_duration or ('20s' if args.backend == 'search' else '10s'),
               '-duration', args.duration, '-database-cpus', str(args.database_cpus),
               '-write-percent', str(writes), '-backend-batch-limit', str(args.batch_limit),
               '-output', str(args.output.resolve() / name)]
    if args.mongod:
        command += ['-mongod', str(args.mongod.resolve())]
    if lua:
        command.append('-lua-mutations')
    return command


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--backend', choices=('mongo', 'search'), required=True)
    parser.add_argument('--weir', type=Path, default=Path('.tools/weir'))
    parser.add_argument('--mongod', type=Path)
    parser.add_argument('--output', required=True, type=Path, help='new directory for per-case reports and logs')
    parser.add_argument('--bulk', action='store_true', help='append read and write bulk32 comparisons')
    parser.add_argument('--batch-limit', type=int, default=0, help='0 keeps the server default')
    parser.add_argument('--database-cpus', type=float, default=1, help='enforced CPU quota for each owned Docker database')
    parser.add_argument('--concurrency-levels', default='8,32', help='ascending total client concurrency')
    parser.add_argument('--bulk-concurrency-levels', default='8,32', help='ascending bulk client concurrency, at most 64')
    parser.add_argument('--client-processes', type=int, default=4, help='independent client OS processes for single-record cases')
    parser.add_argument('--records', type=int, default=8192, help='working-set document count')
    parser.add_argument('--payload-bytes', type=int, default=1024, help='document payload size')
    parser.add_argument('--rounds', type=int, default=3, help='alternating paired rounds')
    parser.add_argument('--warmup-duration', help='default: Search 20s, Mongo 10s per path/stage')
    parser.add_argument('--duration', default='20s', help='measurement duration per path/stage')
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    # Build once so every case runs the same client binary.
    client = output / 'weir-lab'
    subprocess.run(['go', 'build', '-o', str(client), './cmd/weir-lab'], cwd=root, check=True, timeout=600)
    cases = CASES + (BULK_CASES if args.bulk else [])
    for case in cases:
        name = case[0]
        command = case_command(client, args, case)
        print('Starting ' + name, flush=True)
        with (output / (name + '.log')).open('w') as log:
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT)
        print('Finished ' + name + ': exit ' + str(result.returncode), flush=True)
        if result.returncode:
            raise SystemExit(result.returncode)


if __name__ == '__main__':
    main()
