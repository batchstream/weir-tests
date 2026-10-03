# Weir system tests

Independent blackbox integration tests and reproducible database throughput
comparisons for [Weir](https://github.com/batchstream/weir). The first benchmark
compares native MongoDB/Elasticsearch clients with the published Weir Go SDK
under independent single-record requests from multiple OS client processes. The primary benchmark tests server aggregation across RPCs; bulk-versus-bulk runs are supplemental overhead measurements.

The test module depends on **SDK v0.4.2** and **protocol v0.2.1**. It imports no
Weir server packages. The server executable is prepared separately from an
immutable source revision and checksum in [versions.json](versions.json);
there is no floating `main` dependency or local module replacement.

## Prepare

Requires Go **1.27.1**, Python 3, and a local Docker daemon with a Unix socket.
The fixture supports Linux and macOS, with ordinary IP endpoints and processes;
Kubernetes is not required. Public module and image downloads happen only during
explicit preparation. Images use immutable digests.

```sh
make prepare
```

MongoDB 8.0.32 refuses startup on affected Docker VM kernels. See the
[upstream compatibility matrix](https://jira.mongodb.org/browse/SERVER-125742).
On macOS, the checksum-locked native fixture provides another deployment mode:

```sh
python3 scripts/prepare_native_mongo.py
export WEIR_TEST_MONGODB_BINARY="$PWD/.tools/mongod"
```

This runs a new isolated MongoDB replica set with its own directory and random
loopback port. It preserves the MongoDB guard and the user's Docker settings.
Both paths of each comparison use the same database deployment mode.

## Offline default tests

```sh
make test
GOWORK=off GOPROXY=off GOSUMDB=off python3 scripts/check_dependencies.py
```

`go test ./...` and its race variant do not launch databases or Weir processes.
Default tests verify whole-batch lost acknowledgements without replay, retained
APPLIED-with-failure evidence, diagnostic monitoring failures, deterministic
schedules, accounting and cancellation,
latency histogram bounds, fair pairing, failed-result suppression, endpoint
validation, source locks and fixture ownership. CI also checks the complete
module/package graphs to prevent a dependency on server internals.

## Integration tests

```sh
make integration
```

The explicit `integration` build tag starts two Store owners, a discovery-only
node, MongoDB and Elasticsearch. A missing `WEIR_TEST_BINARY` fails a tagged run
with a preparation instruction rather than silently skipping coverage.

The suite covers:

- Resolve and SDK initialization through every node; missing Store responses.
- Direct business execution through discovered owners; rejection at a nonowner.
- Typed CRUD, duplicate/precondition outcomes, atomic transforms and independently
  checked persistence on both databases.
- Unary same-Store batches of 48 mutations/reads across multiple targets, input-order
  results with missing/precondition outcomes, same-URI mutation chains (including
  failures), and 513 repeated/missing reads within the encoded byte limit. SDK
  preflight and raw server preflight reject late-invalid batches without side
  effects on either real database. Public node metrics prove an entire 32-item
  read reaches exactly one owner in one unary RPC. An additional 513-distinct-record
  Put/Read uses a physical batch limit of 513 and verifies one actual adapter
  invocation per unary call, with all values independently persisted.
- Typed Read/Mutate sequences, finite Scan continuation across owners,
  failed-page checkpoint suppression and Native response evidence.
- Cancellation, graceful owner withdrawal/restart, directory convergence,
  persistent clients and reads after losing the discovery node.
- A separate opt-in multi-process workload starts two real client processes per
  path, sends only single-record calls, and verifies process ownership, request
  accounting, pooled latency and persisted data on both databases.

All services bind to loopback, use random ports and own exclusive data namespaces.
Cleanup targets exact process handles/container IDs after ownership verification.
Startup failures also run cleanup. Logs, a manifest and `cleanup.json` are
preserved in the printed fixture directory.

## Database-full-load comparison

The primary benchmark sweeps independent single-request client concurrency and
checks whether the database limits throughput. Run on Linux with Docker so each
owned database has an enforced, inspected CPU quota and the client/Weir retain
CPU capacity:

```sh
make benchmark

# Equivalent explicit command with a fresh output directory:
go run ./cmd/weir-lab -mode saturation \
  -weir .tools/weir -backend all -database-cpus 1 -store-concurrency 32 -backend-batch-limit 32 \
  -client-processes 4 -concurrency-levels 8,32,128 -batch-sizes 1 \
  -records 2048 -payload-bytes 1024 -write-percent 0 \
  -rounds 3 -warmup-duration 10s -duration 20s \
  -output results/local/saturation-read
```

Every timed worker makes one native database call or one SDK `ReadOne`/`Put`
RPC containing exactly one record. The coordinator launches four real client OS
processes. At primary concurrency 8/32/128, each has 2/8/32 concurrent
sequential workers, its own driver or SDK pool, and a disjoint ID partition.
MongoDB uses `FindOne`/`ReplaceOne`, never `BulkWrite` for a timed single write.
Search uses one-ID `POST _mget` and single-document `PUT _doc`; the read POST keeps
connection reuse without an implicit HTTP GET retry. There is no client batching.
Native pool limits equal each process's worker count; Weir's configured backend
concurrency is independently fixed at 32. Actual database connection gauges are
sampled, so changing the database request/connection ratio remains visible.

Both paths use identical bodies, read/write policies and deterministic request
generators. Each business call has a complete 10s client budget; the fixture
explicitly sets every Weir Store `backend_timeout` to 10s. Weir also honors the
active caller deadline, so queueing cannot grant an extra 10s after dispatch.
Native Search retains its 10s response-header limit within that request context.
The Search write query `timeout=1s` is identical on both paths and governs server
prerequisites, not the complete HTTP acknowledgement budget. JSON parameters,
provenance, the measurement receipt and generated routes retain the timeout
policy. Timeout and UNKNOWN acknowledgements invalidate the stage without retry.
The production default remains 2s when `backend_timeout` is omitted.

The same clients, connection pools and worker-owned revision state continue from warmup into measurement. Actual read/write counts are reported,
since timed runs complete different amounts of work. Every acknowledged mutation
changes its revision. The coordinator merges each child's final revision ranges
and independently verifies all documents and the record count. AB/BA order
alternates across stages and rounds. Child PIDs, pool sizes, common-barrier start
lag and successful process exits are retained. p50/p95/p99 are pooled single-call
histograms, not averages of client percentiles.

JSON reports retain database CPU, memory, cumulative I/O/network bytes and
each business-client PID CPU/RSS and Weir CPU/RSS samples. The coordinator is
excluded from business-client CPU. CPU is normalized by the **inspected Docker quota**;
native Mongo uses all host cores and shares them with the load generator and
Weir. At least five valid intervals must cover at least 80% of the measurement,
mean CPU must reach 90% of the budget, and at least 80% of the complete measured
time must reach that threshold in every round. A neighboring higher concurrency
must add no more than 10% throughput. Both paths must satisfy these conditions
independently before the report emits a database-saturated throughput ratio.

Every measured Weir stage also saves the before/after raw public metrics after
warmup and before independent postflight. Timed counter deltas report actual adapter
invocation counts, average operations per invocation, unary Read/Mutate completions,
terminal records, queue wait, adapter time and rejections. The configured backend
concurrency limit is reported separately as a stable before/after gauge. Dispatch
uses that fixed limit and the bounded working-memory budget. Missing scrapes,
absent metrics and counter resets are explicitly unavailable. These measurements
exclude fixture startup. Adapter invocations can split by
namespace, action or byte bounds, so their counts are not a universal claim about
physical database wire commands. The primary run has exactly one record per RPC;
an average adapter batch greater than one, its full histogram and the RPC/adapter
invocation ratio show cross-RPC aggregation. MongoDB also records physical
find/update/bulkWrite/getMore/killCursors command deltas from owned `serverStatus`
at the timed boundaries; missing fields remain unavailable. Elasticsearch has no
equivalent reliable physical HTTP-command counter. CPU and these
counters are collected without enabling a profiler during the timed comparison.
Use a separate diagnostic run to collect a CPU profile; never mix profiled data
into the capacity report.

`python3 scripts/profile_weir.py --offline --output results/local/cpu-diagnostic`
runs the integration-only CPU and mutex profiler from the locked Weir source.
Its receipt marks the helper as instrumented and excludes its workload from
capacity evidence. It owns and verifies cleanup of its temporary services.

A plateau while database CPU remains low produces **comparison unavailable**:
client/Weir limits or storage/network bottlenecks require more evidence. Byte
counters alone do not establish disk/link saturation. Increase the concurrency
ladder (up to 512 workers in the owned single-owner fixture), record count (at least
max-workers), or duration when a run lacks sufficient evidence. The report also
compares measured steady business QPS and p50/p95/p99 at each matched concurrency.
These observations can show a throughput benefit or regression even when database
maximum capacity remains unavailable; neither outcome is assumed.
No noisy performance ratio is used as a CI pass/fail threshold; workload errors
and failed persistence checks do fail the job. `saturation.yml` runs read,
mixed (10% writes) and write matrices on PRs, with a one-CPU database quota.
The CLI, Makefile and PRs use the primary 8/32/128 ladder. Dispatch can select
that ladder or 8/32/128/512, a workload, database CPU quota (0.5, 1 or 2), and backend grouping
limit (32 or 1). Each run keeps
the quota identical for direct and Weir paths and records the inspected denominator.
The primary ladder describes observed performance through 128 workers; it does
not assume that 128 workers reach maximum capacity. The optional 512-worker stage
is a separate stress extension: each process has 128 workers. The archived 512-worker direct-path warmup failures remain failed
stress evidence; they do not produce a paired capacity comparison. Earlier
results with Weir's implicit 2s backend deadline and a 10s client deadline remain
separate from this matched-budget recipe.
Reports from different quotas are separate experiments; a lower-quota saturation
result cannot establish the capacity of a one-CPU database.

The lab sizes `working_memory` for the selected backend concurrency: at least
41MiB per MongoDB batch and 96MiB per Search batch for its declared 2MiB read limit.
It also sizes process admission memory for ingress, both Store workspaces and
framing and the configured ingress sessions. The 128-worker two-backend sweep
declares 18GiB; the full 512-worker sweep declares 54GiB. These are
admission budgets, not allocated memory or OS reservations; resource samples show
actual consumption. `-store-working-memory-mib 384` reproduces the previous
smaller backend workspace, allowing only 9 MongoDB or 4 Search read batches at
once despite configured concurrency 32. The process envelope still adapts to
the ingress sessions. Each report records both budgets.

For supplemental bulk-versus-bulk overhead measurements, explicitly use
`-mode bulk-saturation -concurrency-levels 8,32,64 -batch-sizes 32`. Historical
bulk reports remain archived; they do not measure the value of aggregating
independent single-record business requests.

For an independent aggregation control, repeat the single-request command with
`-backend-batch-limit 1` and a fresh output directory, then compare with the default
`-backend-batch-limit 32`. This changes only the server adapter grouping limit;
client calls remain one record, backend concurrency stays 32 and the same
conservative workspace budget applies. The control is manual, not another path
in every CI matrix. Dispatch with `backend-batch-limit=1,32` runs both settings
sequentially on the same runner, with a fresh report directory and complete
receipt for each. Batch distribution and RPC/adapter ratio verify whether
aggregation actually occurred. Keep the quota, ladder and other workload
settings identical when evaluating this control.

## Finite single-operation comparison

Run a complete local comparison with owned services:

```sh
go run ./cmd/weir-lab -mode fixed \
  -weir .tools/weir \
  -backend all \
  -operations 10000 -warmup 1000 \
  -records 1024 -payload-bytes 1024 \
  -concurrency 8 -rounds 3 -write-percent 10 \
  -output results/local/mixed
```

`WEIR_TEST_MONGODB_BINARY` is honored by the lab; `-mongod .tools/mongod` is an
equivalent explicit option. Choose a new output directory for each run. Use
`-write-percent 0`, `10` or `100` for read, 90/10 mixed and write-only workloads.
`make diagnostic` explicitly runs this finite single-operation profile. The lab
CLI and `make benchmark` default to the independent single-request saturation benchmark.

The local profile runs one Weir owner, matches Store concurrency to client
concurrency, and configures an adapter grouping limit of 32. The server may merge
compatible queued requests into one adapter invocation; the recorded server
version determines that behavior. These are benchmark parameters, not a claim
about every production deployment.

Each paired round executes the exact same deterministic IDs, payloads and
read/write schedule through both paths, alternating direct/Weir and Weir/direct.
Workers own separate key partitions. Data preparation, preflight, warmup, reset
and independent postflight validation are excluded from timing. Timing includes
payload validation, all attempted calls and worker joins. Failed or indeterminate
operations invalidate the aggregate throughput comparison.

Every replacement flips between two precomputed document revisions, so repeated
writes to the same ID change its stored contents. Reads verify the latest revision
from the identical plan. The workload measures replacements in a fixed working
set; it does not model continual insertion of new records.

The native baseline uses one database operation per client call. Weir's internal
batching follows its recorded configuration. MongoDB uses primary reads,
majority write acknowledgement and disabled read/write retries on both paths.
Search uses matching pipeline, refresh, active-shard and timeout policies. Its
direct reads use single-ID `POST /INDEX/_mget?realtime=true`, matching Weir's
read operation while preserving pooled connections without implicit GET replay.

Reports contain JSON evidence and a Markdown table: successful operations/s,
errors, indeterminate writes, approximate p50/p95/p99 upper bounds, raw paired
results and the **Weir/direct throughput ratio**. Ratios below 1 mean lower Weir
throughput for that recorded profile. Document byte counts are logical payload
bytes, excluding framing and metadata; they are not measured network traffic.
Latency includes the extra SDK/gRPC completion and validation work.

For independently provisioned endpoints, use `cmd/weir-bench -help`. That command
requires explicit endpoints and generates a fresh exclusive database/index
namespace. Remote IPs require an explicit flag; credential-bearing URLs are
rejected. Do not use a production database for a benchmark.

## CI and recorded evidence

PR CI runs offline unit/race/vet/boundary tests, a real Linux multi-node
integration suite and a small verified comparison on both databases. The CI
comparison checks execution and report validity; it has no noisy performance
threshold. Workflow artifacts preserve the reports and fixture cleanup receipts.

[Recorded measurements](results/README.md) describe the local hardware, topology,
commands, immutable sources and limits of the initial throughput measurements.
Measurements characterize that environment and workload; they do not establish
a universal Weir throughput penalty or production capacity.
