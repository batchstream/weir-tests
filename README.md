# Weir system tests

Independent blackbox integration tests and reproducible database throughput
comparisons for [Weir](https://github.com/batchstream/weir). The first benchmark
compares native MongoDB/Elasticsearch clients with the published Weir Go SDK
under the same finite logical workload.

The test module depends on **SDK v0.2.0** and **protocol v0.1.0**. It imports no
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
Default tests verify deterministic schedules, accounting and cancellation,
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
- Completion-gated mixed execution, finite Scan continuation across owners,
  failed-page checkpoint suppression and Native response evidence.
- Cancellation, graceful owner withdrawal/restart, directory convergence,
  persistent clients and reads after losing the discovery node.

All services bind to loopback, use random ports and own exclusive data namespaces.
Cleanup targets exact process handles/container IDs after ownership verification.
Startup failures also run cleanup. Logs, a manifest and `cleanup.json` are
preserved in the printed fixture directory.

## Throughput comparison

Run a complete local comparison with owned services:

```sh
go run ./cmd/weir-lab \
  -weir .tools/weir \
  -backend all \
  -operations 10000 -warmup 1000 \
  -records 1024 -payload-bytes 1024 \
  -concurrency 8 -rounds 3 -write-percent 10 -batch-collect 0ms \
  -output results/local/mixed
```

`WEIR_TEST_MONGODB_BINARY` is honored by the lab; `-mongod .tools/mongod` is an
equivalent explicit option. Choose a new output directory for each run. Use
`-write-percent 0`, `10` or `100` for read, 90/10 mixed and write-only workloads.

The local profile runs one Weir owner, matches Store concurrency to client
concurrency, and records batch size 32. The example explicitly disables collection
wait; use `-batch-collect 5ms` for the separate batching-wait comparison. The CLI
default is 5 ms and every report records the chosen interval. These are benchmark
parameters, not a claim about every production deployment.

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
