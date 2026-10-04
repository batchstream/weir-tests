# Weir system tests

Independent blackbox integration tests and database throughput comparisons for
[Weir](https://github.com/batchstream/weir). This module uses the published
Go SDK and public protocol. It imports no server packages.

The SDK, protocol, server revision, Go toolchain and database artifacts are
locked in [versions.json](versions.json). Server binaries are built separately
from the verified immutable source. Module replacements and floating revisions
are rejected by the dependency check.

## Preparation and offline tests

Requires Go 1.27.1, Python 3 and a local Docker daemon with a Unix socket.
Fixtures use ordinary endpoints and processes and work without Kubernetes.
Downloads happen during explicit preparation:

```sh
make prepare
make test
GOWORK=off GOPROXY=off GOSUMDB=off python3 scripts/check_dependencies.py
```

Default Go tests, including race tests, launch no databases or Weir processes.
They validate request accounting, mutation evidence without replay, scheduling,
resource sampling, capacity qualification, fixture ownership and dependency boundaries.

On macOS, an isolated native MongoDB fixture is also available:

```sh
python3 scripts/prepare_native_mongo.py
export WEIR_TEST_MONGODB_BINARY="$PWD/.tools/mongod"
```

The prepared binary is checksum-locked. Each test owns a new replica set, data
directory and loopback port. Direct and Weir paths use the same database mode.

## Integration tests

```sh
make integration
```

The opt-in `integration` build tag starts MongoDB, Elasticsearch, two Store
owners and a discovery node. Tagged tests require a prepared `WEIR_TEST_BINARY`.
Coverage includes discovery through every node, direct owner connections, typed
CRUD and Lua transformations, independent persistence checks, same-Store batches,
whole-request validation, finite Scan continuation, Native response evidence,
cancellation, owner restart, directory convergence and persistent client recovery.
Separate multi-process tests verify independent single-record callers.

Services bind to loopback and own exclusive namespaces. Cleanup verifies exact
process and container ownership. Fixture logs, manifests and cleanup receipts
are retained in the printed output directory, including startup failures.

## Primary throughput benchmark

The primary benchmark measures Weir's aggregation of independent single-record
business requests. Four client OS processes issue one record per database call
or SDK RPC. Every process owns its driver or SDK connection pool and a disjoint
key partition. There is no client batching.

Run on Linux with Docker so database CPU quotas can be enforced and inspected:

```sh
make benchmark

# Equivalent command; choose a fresh output directory for each run.
go run ./cmd/weir-lab -mode saturation \
  -weir .tools/weir -backend all -database-cpus 1 \
  -store-concurrency 32 -backend-batch-limit 32 -client-processes 4 \
  -concurrency-levels 8,32,128 -batch-sizes 1 \
  -records 2048 -payload-bytes 1024 -write-percent 0 \
  -rounds 3 -warmup-duration 10s -duration 20s \
  -output results/local/read
```

Use `-write-percent 0`, `10` or `100` for reads, mixed traffic or writes.
MongoDB uses one `FindOne` or `ReplaceOne` per ordinary native call. Search uses
one-ID `POST _mget` or single-document `PUT _doc`. Both paths use identical bodies,
read/write policies and a complete 10s request budget, without business retries.
Warmup, initialization and independent persisted-data verification are excluded
from timing. Pools and revision state continue from warmup into measurement.
AB/BA order alternates across rounds. Failed calls or uncertain acknowledgements
invalidate the comparison.

For Lua read-modify-write, add `-lua-mutations -write-percent 100`. Each native
MongoDB request uses a snapshot transaction with one read, revision computation,
replacement and majority commit. Each native Search request uses one real-time
read and a write conditioned on its observed sequence number and primary term.
The SDK sends one Lua `AtomicTransform` per request with the same computation.
Fixture documents are identical on both paths; Weir adds no document metadata.

Repeat with `-backend-batch-limit 1` to disable server aggregation. Keep other
settings identical and use a fresh output directory. The dispatch workflow can
run grouping limits `1,32` sequentially on the same runner. Adapter batch-size
distributions and the RPC/adapter ratio show whether aggregation occurred.

Reports retain per-round throughput, pooled latency histograms, client process
identities, start lag, CPU/RSS, database CPU/memory/I/O/network samples and raw
Weir metrics. Metrics report unary completions, terminal records, queue wait,
adapter time, rejections and operations per adapter invocation. MongoDB also
records physical command deltas. Adapter invocations can split by namespace,
action and byte limits; their counts are distinct from database wire commands.
Missing samples and reset counters are reported as unavailable.

A database-saturated comparison requires both paths to qualify independently:
at least five CPU intervals cover 80% of measurement time, mean CPU reaches 90%
of the inspected quota, that threshold holds during 80% of measurement time in
every round, and the next concurrency adds at most 10% throughput. The highest
concurrency alone cannot prove a plateau. A plateau with low database CPU leaves
maximum-capacity comparison unavailable. Reports still show observed throughput
and latency at each matched concurrency. No performance ratio is a CI threshold.

Weir working-memory and process-admission budgets scale with backend concurrency
and ingress sessions. Reports record declared budgets and observed usage.
`-store-working-memory-mib` allows explicit workspace experiments.

## Other workload modes

`make diagnostic` runs a finite, deterministic single-operation comparison.
Preparation, warmup, reset and postflight checks remain outside timing:

```sh
go run ./cmd/weir-lab -mode fixed -weir .tools/weir -backend all \
  -operations 10000 -warmup 1000 -records 1024 -payload-bytes 1024 \
  -concurrency 8 -rounds 3 -write-percent 10 -output results/local/mixed
```

For supplemental bulk-versus-bulk overhead measurements, use
`-mode bulk-saturation -concurrency-levels 8,32,64 -batch-sizes 32`.
This mode measures client batching overhead separately from the primary scenario.

For independently provisioned endpoints, see `cmd/weir-bench -help`. It requires
explicit endpoints and creates an exclusive namespace. Remote addresses require
an explicit flag; credential-bearing URLs are rejected.

## CI and output

CI runs offline unit/race/vet and dependency checks, real multi-node integration,
and verified ordinary and Lua comparison smoke tests on both databases.
`saturation.yml` runs read/mixed/write matrices on PRs and accepts CPU quota,
concurrency and grouping controls on dispatch. Workflow artifacts preserve raw
reports and owned-resource cleanup receipts.

Generated measurements belong in ignored output directories under `results/`.
See [results/README.md](results/README.md) for the evidence required when comparing
runs. The current checkout documents and tests the current contract; previous
source revisions and measurements remain available in Git history.
