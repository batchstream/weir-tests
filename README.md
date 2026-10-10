# Weir system tests

Independent blackbox integration tests and database throughput comparisons for
[Weir](https://github.com/batchstream/weir). This module uses the published
Go SDK and public protocol. It imports no server packages.

The SDK, protocol, server revision, Go toolchain and database artifacts are
locked in [versions.json](versions.json). Server binaries are built separately
from verified immutable module source. Explicit local builds for before/after
experiments are frozen with a source snapshot and checksummed receipt; they are
reported as local source. Module replacements and floating revisions are rejected
by the dependency check.

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
CRUD and Lua transformations, independent persistence checks, same-Store calls
containing many records, slice preflight and per-record validation, typed Scan filters and include/exclude projections, cross-owner continuation, Native response evidence,
cancellation, owner restart, directory convergence and persistent client recovery.
Lua source returns one function receiving the current and incoming documents as
ordinary tables. Tests cover object replacements, explicit keep/delete/reject
actions, first creation, and concurrent read-modify-write through both owners.
Nil, missing or multiple callback results return INVALID_ARGUMENT, including a
callback that returns a missing current document. Missing records and missing
backend targets have distinct outcomes; empty scans succeed and complete HTTP
404 responses retain Native completion evidence. Separate multi-process tests
verify independent single-record callers.
One-RPC reads deliver 64 MiB and 256 MiB across 1024 and 4096 distinct documents while
checking bounded result credits and sampled owner RSS. Interrupted streams retain
confirmed record outcomes; unconfirmed mutations remain indeterminate.

Each Execute request carries one record and its consecutive index. The client
streams records as they are produced; server aggregation stays within the Store.
Later invalid records stop the stream while preserving earlier acknowledged writes.
Documents use explicit `ContentType` fields. Native carries an opaque request Document and optional response metadata; adapters own the formats. HTTP fixtures use standard messages through SDK helpers. Tests verify unknown content types reach adapter classification, while adapter-specific projection rules do not constrain other Stores.

Services bind to loopback and own exclusive namespaces. Cleanup verifies exact
process and container ownership. Fixture logs, manifests and cleanup receipts
are retained in the printed output directory, including startup failures.

## Primary throughput benchmark

The primary benchmark measures Weir's aggregation of independent single-record
business requests. Four client OS processes issue one record per database call
or SDK RPC. Concurrency levels count total workers across all processes; with
four processes, levels 8 and 32 mean 2 and 8 workers per process. Every process owns its driver or SDK connection pool and a disjoint
key partition. There is no client batching.

Run on Linux with Docker so database CPU quotas can be enforced and inspected:

```sh
make benchmark

# Equivalent command; choose a fresh output directory for each run.
go run ./cmd/weir-lab -mode saturation \
  -weir .tools/weir -backend all -database-cpus 1 \
  -client-processes 4 \
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
The function updates `current.revision = 1 - current.revision` and returns the
current document; existing BSON int32 revision fields retain their width.
Fixture documents are identical on both paths; Weir adds no document metadata.

Repeat with `-backend-batch-limit 1` to disable server aggregation. Keep other
settings identical and use a fresh output directory. The dispatch workflow can
run grouping limits `1,32` sequentially on the same runner. Adapter batch-size
distributions and the RPC/adapter ratio show whether aggregation occurred.

Reports retain per-round throughput, pooled latency histograms, client process
identities, start lag, CPU/RSS, database CPU/memory/I/O/network samples and raw
Weir metrics. Metrics report stream completions, terminal records, queue wait,
adapter time, rejections and operations per adapter invocation. MongoDB also
records physical command deltas. Adapter invocations can split by namespace,
action and byte limits; their counts are distinct from database wire commands.
Missing samples and reset counters are reported as unavailable.
`business_operation_timeouts` counts failed logical operations whose request context
expired before the call returned; an expired bulk context counts every failed operation
in that batch. This diagnostic does not override confirmed mutation evidence.

A database-saturated comparison requires both paths to qualify independently:
at least five CPU intervals cover 80% of measurement time, mean CPU reaches 90%
of the inspected quota, that threshold holds during 80% of measurement time in
every round, and the next concurrency adds at most 10% throughput. The highest
concurrency alone cannot prove a plateau. A plateau with low database CPU leaves
maximum-capacity comparison unavailable. Reports still show observed throughput
and latency at each matched concurrency. No performance ratio is a CI threshold.

The lab leaves server configuration at its defaults unless an experiment explicitly
sets `-backend-batch-limit` (`batching.max_operations`), `-pending-records`
(`transport.max_pending_records`) or `-exchange-bytes`
(`backend.max_exchange_bytes`). Zero means omitted; the generated fixture
configuration and raw metrics preserve the actual settings. There is no Store
concurrency, workspace memory or per-backend timeout override in the current
server contract. The SDK and native callers both use the complete 10s request
budget.

For an unpublished local server refactor, freeze its Go source and build a new
binary. Existing output artifacts are preserved:

```sh
python3 scripts/build_local_server.py --source ../weir \
  --output .tools/performance-after-server
go build -trimpath -buildvcs=false -o .tools/performance-lab ./cmd/weir-lab
.tools/performance-lab -mode saturation \
  -weir .tools/performance-after-server \
  -server-receipt .tools/performance-after-server.receipt.json \
  -backend mongo -mongod .tools/mongod -client-processes 4 \
  -concurrency-levels 8,32 -batch-sizes 1 -records 1024 \
  -payload-bytes 1024 -write-percent 10 -rounds 3 \
  -warmup-duration 2s -duration 5s -output results/local/after-mixed
```

The receipt records the binary SHA256, source HEAD, dirty state, Go source tree
SHA256 and every frozen input file. The retained source snapshot includes local
untracked Go files and a tracked Go diff, so the binary can be rebuilt independently
of later edits. Only Go build inputs are copied; secret files are excluded.
Local macOS measurements describe the observed workload on that host and do not
prove Linux database saturation or a production capacity limit.

`scripts/run_performance_matrix.py` serializes read, mixed (10%), write and Lua
cases at total concurrency 8/32, with three alternating native/Weir rounds,
2s warmup and 5s measurement for native MongoDB, or 20s warmup and 20s
measurement for Elasticsearch per stage. Longer Search stages gave better
resource sampling coverage and avoided a short-run regression that did not
reproduce in the adjacent longer comparison. These are measurement defaults;
the server batching and transport defaults remain 32 records.
`--warmup-duration` and `--duration` allow explicit overrides. It retains exact commands, client source
snapshots, binary hashes, JSON/Markdown reports and copied fixture cleanup/log
evidence. Use a fresh output root for each server or batching candidate:

```sh
python3 scripts/run_performance_matrix.py --backend mongo \
  --weir .tools/performance-before-server --mongod .tools/mongod \
  --output results/local/mongo-before --bulk
python3 scripts/run_performance_matrix.py --backend mongo \
  --weir .tools/performance-after-server \
  --server-receipt .tools/performance-after-server.receipt.json \
  --mongod .tools/mongod --output results/local/mongo-after --bulk
python3 scripts/summarize_performance.py --backend mongo \
  --before results/local/mongo-before --after results/local/mongo-after \
  --output results/local/mongo-comparison
```

`--bulk` appends read and write cases using 32 records per call. Bulk cases use
one client process with 8/32 workers; ordinary and Lua cases use four independent
client processes. Batching limits 128/256 need the bulk cases or higher single
request concurrency to exercise batches above 32 records.

The summary recomputes throughput from all successful records and measured time,
preserves each round's throughput and p95, and checks that every compared case
has matching client source/binary hashes, workload parameters and paired round
order. Missing core cases are marked partial; a one-sided bulk case is recorded
as missing. Server CPU time and Go allocation costs use Prometheus counter
differences around each timed stage, including small metrics collection overhead.
RSS is a sampled maximum. Physical database commands are reported only when the
harness captured counters around the full stage; an unavailable bulk count is
not inferred from adapter calls. Histograms without separate 31 and 32 boundaries
cannot determine the exact 32-record full-batch fraction.

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
The minimal `results/go.mod` isolates historical generated Go evidence from root
unit tests and dependency checks without moving or deleting old measurements.
See [results/README.md](results/README.md) for the evidence required when comparing
runs. The current checkout documents and tests the current contract; previous
source revisions and measurements remain available in Git history.
