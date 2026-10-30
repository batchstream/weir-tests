# Weir system tests

Independent blackbox integration tests and database throughput comparisons for
[Weir](https://github.com/batchstream/weir). This module uses the published
Go SDK and public protocol. It imports no server packages.

| Tool | Purpose |
| --- | --- |
| `tests/integration` | Real multi-node integration tests against owned MongoDB and Elasticsearch fixtures |
| `cmd/weir-lab` | Paired native-versus-Weir throughput comparisons on owned fixtures |
| `scripts/run_performance_matrix.py`, `scripts/summarize_performance.py` | Run a read/mixed/write/Lua matrix and compare two matrix runs |
| `cmd/weir-crawler` | [Crawler mutation benchmark](docs/crawler-benchmark.md) on an owned Search fixture |
| `cmd/weir-bench` | Fixed and saturation comparisons against independently deployed endpoints |
| `cmd/weir-soak`, `scripts/soak-faults.sh` | Long-running load through deployed Weir replicas, with every outcome checked against the databases, under scheduled faults |

The Weir server commit and the database images are pinned in
[versions.json](versions.json) by full commit; `go.mod` pins released SDK and
protocol versions without replacements. The tests depend on no server module or
package.

## Preparation and tests

Requires Go, Python 3, `jq` and a local Docker daemon with a Unix socket.
Fixtures use ordinary endpoints and processes and work without Kubernetes.

```sh
make prepare
make test
```

`make prepare` runs `go install` for `github.com/batchstream/weir/cmd/weir` at
the pinned commit into `.tools/weir` and stamps that commit as its `revision`.
Default Go tests, including race tests, launch no databases or Weir processes.
They validate request accounting, mutation outcomes without replay, scheduling,
resource sampling, saturation checks and fixture ownership.

### Native MongoDB fixture

On macOS (arm64/amd64) or Ubuntu 24.04 amd64, an explicitly selected native
MongoDB fixture is also available:

```sh
mongod=$(scripts/mongod.sh) && export WEIR_TEST_MONGODB_BINARY="$mongod"
```

`scripts/mongod.sh` downloads the MongoDB 8.0.32 archive for the host (the Linux
build targets Ubuntu 24.04), checks
its pinned SHA-256 and prints the installed `mongod` path. Each test owns a new
replica set, data directory and loopback port. Direct and Weir paths use the
same database mode. The blackbox CI job exports `WEIR_TEST_MONGODB_BINARY` for
integration and both comparison smokes; Search uses its Docker fixture. Without
that explicit selection the Mongo fixture is Docker; there is no runtime
fallback. The native fixture has no database cgroup limits.

## Integration tests

```sh
make integration
```

The opt-in `integration` build tag starts MongoDB, Elasticsearch and two
independent Weir replicas hosting the same Stores. Tagged tests require a
prepared `WEIR_TEST_BINARY`. Coverage includes SDK reads through every replica,
NotFound for an unhosted Store, typed CRUD and Lua transformations, independent
persistence checks, same-Store calls containing many records sent on one stream,
slice preflight and per-record validation, Native MongoDB
find/getMore and Search scroll pagination through alternating replicas,
backend-native filters and include/exclude projections, Native response metadata,
cancellation, and a replica stop and restart that the other replica and the
original client survive.
Lua source returns one function receiving the current and incoming documents as
ordinary tables. Tests cover object replacements, explicit keep/delete/reject
actions, first creation, and concurrent read-modify-write through both replicas.
Nil, missing or multiple callback results return INVALID_ARGUMENT, including a
callback that returns a missing current document. Missing records and missing
backend targets have distinct typed outcomes. Native keeps backend semantics:
MongoDB find on a missing collection succeeds with an empty cursor; Search returns
a complete HTTP 404 with index/record absence in its body. Pagination uses one
record per page, at most eight pages, ten Native calls, 1 MiB per response and
10 MiB total per paginator (including cleanup/probes). Callers own backend cursor
IDs, track rotating scroll IDs, and explicitly reclaim live cursors on early stop
or errors using a separate five-second cleanup deadline. Exhausted Search scrolls
are also cleared. Closed continuations retain complete Native transport metadata
alongside MongoDB CursorNotFound or Search HTTP 404 business errors. There is no
Weir continuation/checkpoint or projection-token binding contract. Search scroll
requests use the API common to Elasticsearch and OpenSearch; the fixture uses the
pinned Elasticsearch version. Separate multi-process tests verify independent
single-record callers.
One-RPC reads deliver 64 MiB and 256 MiB across 1024 and 4096 distinct documents
while checking that queued records stay within the stream window and sampling
replica RSS. Interrupted streams retain confirmed record outcomes; unconfirmed
mutations remain indeterminate.

Each Execute request carries one record and its consecutive index. The client
streams records as they are produced; server aggregation stays within the Store.
Later invalid records stop the stream while preserving earlier acknowledged writes.
Documents use explicit `ContentType` fields. Native carries an opaque request
Document and optional response metadata; adapters own the formats. HTTP fixtures
use standard messages through SDK helpers. Tests verify unknown content types
reach adapter classification. Projection syntax and business errors belong to the
backend, not a portable Weir projection/token layer. Native transfer completion
is checked separately from MongoDB `ok` and HTTP status/body, including complete
backend error responses.

Services bind to loopback and own exclusive namespaces. Cleanup verifies exact
process and container ownership. Fixture logs, manifests and `cleanup.json` are
kept in the printed output directory, including after startup failures.

## Saturation benchmark

The saturation benchmark measures Weir's aggregation of independent single-record
business requests. Client OS processes issue one record per database call or SDK
RPC. Concurrency levels count total workers across all processes; with four
processes, levels 8 and 32 mean 2 and 8 workers per process. Every process owns
its driver or SDK connection pool and a disjoint key partition. There is no
client batching.

Run on Linux with Docker so database CPU quotas can be enforced and inspected:

```sh
make benchmark

# Equivalent command; choose a fresh output directory for each run.
go run ./cmd/weir-lab -mode saturation \
  -weir .tools/weir -backend all -database-cpus 1 \
  -client-processes 4 \
  -concurrency-levels 4,8,32,128,256 -batch-sizes 1 \
  -records 8192 -payload-bytes 1024 -write-percent 0 \
  -rounds 3 -warmup-duration 10s -duration 20s \
  -output results/local/read
```

`weir-lab` requires the binary's revision to match `versions.json` and refuses
to overwrite an existing output directory.

Use `-write-percent 0`, `10` or `100` for reads, mixed traffic or writes.
MongoDB uses one `FindOne` or `ReplaceOne` per ordinary native call. Search uses
one-ID `POST _mget` or single-document `PUT _doc`. Both paths use identical bodies,
read/write policies and a complete 10s request budget, without business retries.
Warmup, initialization and independent persisted-data verification are excluded
from timing. Pools and revision state continue from warmup into measurement.
AB/BA order alternates across rounds. Failed calls or uncertain acknowledgements
invalidate the comparison.

For Lua read-modify-write, add `-lua-mutations -write-percent 100`. Each native
MongoDB request uses one atomic `UpdateOne` aggregation pipeline, with
`$set: {revision: {$subtract: [int32(1), "$revision"]}}` and majority acknowledgement.
Each native Search request uses one `POST _update` Painless script with
`ctx._source.revision = 1 - ctx._source.revision` and `retry_on_conflict=0`.
Neither baseline performs a client read or transaction. These are native controls
for this fixed transformation, rather than a claim about arbitrary Lua programs.
The SDK sends one Lua `AtomicTransform` per request with the same computation.
The function updates `current.revision = 1 - current.revision` and returns the
current document; independent postflight checks require the BSON revision to
retain its int32 type and verify every field and final revision on both paths.
Fixture documents are identical on both paths; Weir adds no document metadata.
The logical request/result document-byte counters exclude scripts and commands,
so they are zero for transform-only calls. Backend reads and replacements still
contribute to the measured network and CPU costs.

Repeat with `-backend-batch-limit 1` to disable server aggregation. Keep other
settings identical and use a fresh output directory. Adapter batch-size
distributions and the RPC/adapter ratio show whether aggregation occurred.

Reports retain per-round throughput, pooled latency histograms, client process
identities, start lag, CPU/RSS, database CPU/memory/I/O/network samples and raw
Weir metrics. Metrics report stream completions, terminal records, queue wait,
adapter time, rejections and operations per adapter invocation. MongoDB also
records selected business-command deltas at the timed boundaries for both
single-record and bulk calls. These counters exclude handshakes and monitoring
commands. Adapter invocations can split by namespace, action and byte limits;
their counts are distinct from database wire commands. Missing samples and reset
counters are reported as unavailable. `business_operation_timeouts` counts failed
logical operations whose request context expired before the call returned; an
expired bulk context counts every failed operation in that batch. This diagnostic
does not override confirmed mutation outcomes.

A database-saturated comparison requires both paths to independently reach
saturation: at least five CPU intervals cover 80% of measurement time, mean CPU
reaches 90% of the inspected quota, that threshold holds during 80% of
measurement time in every round, and the next concurrency adds at most 10%
throughput. The highest concurrency alone cannot prove a plateau. A plateau with
low database CPU leaves maximum-capacity comparison unavailable. Reports still
show observed throughput and latency at each matched concurrency. No performance
ratio is a CI threshold.

Without `-backend-batch-limit` the lab leaves Weir's batching at its defaults.
An explicit limit (`batching.max_operations`) writes all four batching controls:
`-backend-batch-bytes` 0 writes 8 MiB, `-backend-batch-wait` 0 executes
immediately and `-backend-max-in-flight` 0 selects Weir's default of 16
concurrent executions per Store. Weir's execution limit bounds matched-concurrency
comparisons, so raise it to the highest client level when the native path should
not out-run it. The report keeps the generated Weir configuration, and raw
metrics preserve the actual settings. The SDK and native callers both use the
complete 10s request budget.

These are closed-loop matched-concurrency measurements, not fixed offered-rate
experiments. Local macOS measurements describe the observed workload on that host
and do not show Linux database saturation or a production capacity limit.

### Saturation workflow

`saturation.yml` runs only on manual dispatch on a Linux amd64 runner. It installs
the pinned server and (unless the backend is `search`) the native MongoDB
fixture, then runs one `weir-lab` saturation matrix with the selected write
percentage, Search CPU quota, concurrency levels, backend batch limit and
execution limit, Lua mutations, backend, mode and records per call. The default
execution limit of 256 matches the highest default client level. The default curve is
4/8/32/128/256 total workers across four client processes, three alternating
rounds and 10s warmup plus 20s measurement per path. Native Mongo uses all host
CPUs; `database-cpus` applies only to Docker Search. Reports, the run log and
fixture logs are uploaded as a workflow artifact.

```sh
gh workflow run saturation.yml --ref main \
  -f backend=all -f write-percent=10 -f client-levels=8,32 \
  -f backend-batch-limit=32 -f backend-max-in-flight=256 -f mode=saturation -f batch-sizes=1
```

## Performance matrix

`scripts/run_performance_matrix.py` builds `weir-lab` once and runs read, mixed
(10%), write and Lua cases in sequence, at total concurrency 8/32 with three
alternating rounds. Stages use 10s warmup for MongoDB or 20s for Elasticsearch,
and 20s measurement. `--warmup-duration`, `--duration`, `--database-cpus`,
`--concurrency-levels`, `--client-processes`, `--records`, `--payload-bytes`,
`--rounds` and `--batch-limit` override these. Docker databases default to an
enforced 1 CPU quota and an 8192-document working set. `--bulk` appends read and
write cases using 32 records per call from one client process;
`--bulk-concurrency-levels` controls their concurrency, at most 64. Each case
writes its reports and a log into a new output directory, and the matrix stops
at the first failed case.

`scripts/summarize_performance.py` compares two matrix outputs, for example
before and after a `versions.json` update or with different batch limits:

```sh
python3 scripts/run_performance_matrix.py --backend mongo \
  --mongod "$(scripts/mongod.sh)" --output results/local/mongo-before --bulk
python3 scripts/run_performance_matrix.py --backend mongo \
  --mongod "$(scripts/mongod.sh)" --batch-limit 128 --output results/local/mongo-after --bulk
python3 scripts/summarize_performance.py --backend mongo \
  --before results/local/mongo-before --after results/local/mongo-after \
  --output results/local/mongo-comparison
```

The summary recomputes throughput from all successful records and measured time,
keeps each round's throughput and p95, and requires every compared case to have
the expected report kind (`single` for the core cases, `bulk` for the bulk cases),
matching workload parameters and paired round order. Missing core cases mark the
summary partial; a one-sided bulk case is listed as missing. Server CPU time and
Go allocation costs use Prometheus counter differences around each timed stage,
including small metrics collection overhead. RSS is a sampled maximum. Physical
database commands are reported only when counters were captured around the full
stage. Histograms without separate 31 and 32 boundaries cannot determine the
exact 32-record full-batch fraction.

## Other workload modes

`make diagnostic` runs a finite, deterministic single-operation comparison.
Preparation, warmup, reset and postflight checks remain outside timing:

```sh
go run ./cmd/weir-lab -mode fixed -weir .tools/weir -backend all \
  -operations 10000 -warmup 1000 -records 1024 -payload-bytes 1024 \
  -concurrency 8 -rounds 3 -write-percent 10 -output results/local/mixed
```

For bulk-versus-bulk overhead measurements, use
`-mode bulk-saturation -concurrency-levels 8,32,64 -batch-sizes 32`.
This mode measures client batching overhead separately from the saturation
benchmark.

For independently provisioned endpoints, see `cmd/weir-bench -help`. It requires
explicit endpoints and creates an exclusive namespace. Remote addresses require
an explicit flag; credential-bearing URLs are rejected.

### Saturation against deployed services

`weir-bench -mode saturation` runs the saturation benchmark above against a
database and Weir that you deploy, for example on separate Kubernetes nodes so
that clients, Weir and the database never share CPU. The command starts the
same independent client processes, each with its own pool, and drives the same
alternating direct/Weir ladder. Reports have the same format as `weir-lab`.

```sh
weir-bench -mode saturation -allow-remote -backend mongo \
  -mongo-uri 'mongodb://10.0.1.20:27017/?directConnection=true' \
  -weir-address 10.0.2.30:7447 -weir-diagnostics 10.0.2.30:7449 -store mongo \
  -database-cpus 2 -client-processes 16 -concurrency-levels 64,256,1024,2048 \
  -records 32768 -padding-bytes 1024 -write-percent 100 \
  -rounds 3 -warmup-duration 10s -duration 20s -output results/local/eks-mongo
```

Weir and Search endpoints are explicit IP:port values; resolve a Kubernetes
headless Service to its Pod IP first. The Weir Store must point at the same
database. Database CPU comes from the database itself: `serverStatus`
`extra_info` on Linux `mongod` and `_nodes/stats/process` on Elasticsearch,
divided by `-database-cpus`, which must equal the database's CPU limit.
Each sample uses the database's own read time, and Weir CPU and RSS come from
its diagnostics `/metrics`. The MongoDB URI must reach the `mongod` that takes
the writes, usually a single node or the primary; a `mongos` router is
rejected. Elasticsearch must be a single node. The run stops before the ladder
when the database reports no process CPU, such as `mongod` on macOS. Client CPU
uses `/proc` on Linux, so a minimal image without `ps` works.
The whole run is bounded by `-timeout`, derived from the ladder when unset.

Run the client on its own node with enough CPU for the highest level; give the
database fixed CPU requests and limits so its utilization has a stable
denominator. Results describe that database size and topology, not a
production capacity limit.

Mongo endpoints use a bounded (4096-byte), credential-free `mongodb://` profile
with an empty or root path and explicit numeric ports (1–65535). A direct profile
has one seed, no replica set, and omitted or `true` `directConnection`. Replica
discovery uses 1–7 unique seeds, an explicit `replicaSet` (1–64 ASCII letters,
digits, `_` or `-`), and omitted or `false` `directConnection`, for example
`mongodb://127.0.0.1:27017,[::1]:27018/?replicaSet=bench`.
Only those two query keys are accepted; authentication, TLS options, database
paths, SRV, load balancing and read-preference overrides are not supported here.
Every seed is validated, including canonical duplicate detection. Loopback IPs
and literal `localhost` are permitted by default; every other IP or syntactically
valid DNS name requires `--allow-remote`. Validation never resolves DNS. This is
seed authorization, **not network confinement**: discovery may dial members not
listed in the supplied seeds. Programmatic workload callers must set
`Config.AllowRemote` for the same opt-in.

The comparison path named `direct` means the native database client, **not**
`directConnection=true`. Its Mongo options preserve replica discovery, primary
reads, majority writes, disabled retries/compression and worker-sized pooling.
Discovery uses poll monitoring with a 1-second heartbeat, matching the server;
direct profiles retain the driver's default heartbeat. Reports record canonical
credential-free seeds, profile, replica-set name and polling policy for the
native connection. Parser/options tests are offline checks; live discovery and
primary movement need a separate replica-set fixture.

## Soak

`weir-soak` drives a steady write and read load through deployed Weir replicas
for hours, then checks every key against the databases directly. It creates a
fresh MongoDB database and Search index, drops them at the end unless
`-keep-data` is set, and never starts, stops or faults services.

Each worker alone writes its own keys, one operation at a time, with increasing
sequence numbers. The database may therefore hold only a key's latest APPLIED
sequence or one whose outcome was UNKNOWN, and a key with no APPLIED write may
be absent. Reads through Weir must return such a sequence. After NOT_STARTED or
NOT_APPLIED the worker reads the key directly to confirm the write took no
effect, then retries with a fresh sequence, so a refusal that took effect
stays visible. An SDK error without a result counts as UNKNOWN. An UNKNOWN
write is assumed to land within four operation timeouts of being issued; a
later APPLIED write retires it. `-verify-percent` of APPLIED writes are also
read back directly. Single-key MongoDB checks read linearizably from the
primary, so they see a write applied just before a refusal and are never stale
after a failover; the final check reads majority-committed data. Search checks
use realtime GET and `_mget`, and a document its shard cannot serve fails the
check instead of reading as absent.

```sh
weir-soak -weir-address weir.soak.svc:7447 -weir-diagnostics weir.soak.svc:7449 \
  -mongo-uri 'mongodb://mongo-0.mongo.soak.svc:27017,mongo-1.mongo.soak.svc:27017,mongo-2.mongo.soak.svc:27017/?replicaSet=soak' \
  -search-url http://elasticsearch.soak.svc:9200 -duration 12h -output /work/soak
```

The Weir addresses are names that resolve to every replica, such as a headless
Service; the diagnostics name is resolved at every sample to scrape each
replica's goroutines, live memory, RSS, open files and sessions. `<output>.jsonl`
holds one sample per `-sample-interval` with per-backend outcomes, latency
quantiles and database connections, and `<output>.json` the totals and final
check. The final check retries a backend that refuses reads for up to five
minutes. The run fails on any violation, a backend without APPLIED writes or an
incomplete final check.

`scripts/soak-faults.sh <namespace> <weir-release> <quiet> <window> <every>`
injects faults from a Pod in the same namespace, whose service account may get,
list, delete and exec into Pods, watch Deployments and ReplicaSets, and patch
Deployments there. After `quiet` seconds it rotates through a MongoDB primary
stepdown, a rolling restart of the weir-charts release, an Elasticsearch process
restart, a MongoDB primary Pod deletion and a Weir Pod deletion, one every
`every` seconds for `window` seconds, and waits for recovery after each. A
fault that does not recover in time ends the schedule with a non-zero exit. A
fault-free first half shows whether memory, goroutines, files, connections and
latency stay flat; the second half checks outcomes across failover and restarts.
End the window at least 20 minutes before the load so the last recovery
finishes first.

## CI and output

CI runs unit, race, vet and module-boundary checks, real multi-node integration,
and verified ordinary and Lua comparison smoke tests on both databases, and
uploads results and fixture logs as a workflow artifact.

All generated output belongs under the ignored `results/` directory, conventionally
`results/local/`. Nothing under `results/` is committed.
