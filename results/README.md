# Initial throughput measurements

These paired measurements compare the same successful logical database load with
and without Weir. They characterize one local deployment at **8 concurrent
clients**, not maximum service capacity or a universal Weir overhead figure.

## Collection wait disabled

Each path executes 10,000 operations per round, with three alternating paired
rounds (30,000 measured operations per path). The working set has 1,024 records,
1,024 padding bytes per document and 1,000 untimed warmup operations. Every
replacement changes its stored revision, including repeated writes to one ID.
The deterministic mixed plan contains **9,029 reads and 971 replacements**.

| Backend | Workload | Direct ops/s | Weir ops/s | Weir/direct | Throughput change |
| --- | --- | ---: | ---: | ---: | ---: |
| MongoDB | [read](2026-10-03-local/read-0ms/mongo.md) | 56,889.5 | 7,698.5 | 0.135 | -86.5% |
| Elasticsearch | [read](2026-10-03-local/read-0ms/search.md) | 9,006.7 | 3,691.8 | 0.410 | -59.0% |
| MongoDB | [mixed](2026-10-03-local/mixed-0ms/mongo.md) | 9,175.5 | 5,129.0 | 0.559 | -44.1% |
| Elasticsearch | [mixed](2026-10-03-local/mixed-0ms/search.md) | 6,754.6 | 3,082.4 | 0.456 | -54.4% |
| MongoDB | [write](2026-10-03-local/write-0ms/mongo.md) | 950.4 | 784.7 | 0.826 | -17.4% |
| Elasticsearch | [write](2026-10-03-local/write-0ms/search.md) | 4,592.2 | 2,251.4 | 0.490 | -51.0% |

The aggregate is total successful operations divided by total measured wall time,
not the arithmetic average of round rates. All eight recorded comparisons have
three verified pairs, with zero errors, indeterminate writes, unattempted calls
or applied-with-error outcomes. Both paths passed independent persisted-data
verification after every measured round. Raw JSON accompanies every table.

## A separate 5 ms collection-wait profile

Only the collection interval changes. This profile uses the same mixed plan and
keeps its own direct baseline; ratios must be read within each paired comparison.

| Backend | Direct ops/s | Weir ops/s | Weir/direct | Throughput change |
| --- | ---: | ---: | ---: | ---: |
| [MongoDB](2026-10-03-local/mixed-5ms/mongo.md) | 9,065.3 | 1,000.4 | 0.110 | -89.0% |
| [Elasticsearch](2026-10-03-local/mixed-5ms/search.md) | 6,651.0 | 1,014.9 | 0.153 | -84.7% |

These measurements show that the configured collection wait can substantially
change throughput in a small closed-loop workload. They do not isolate every
component's cost or predict behavior at larger concurrency or over a network.

## Environment and immutable sources

- Apple M2, 8 cores, 24 GiB RAM, macOS 26.6.2 / arm64.
- Client and one Weir owner run natively on the same host. MongoDB **8.0.32** runs
  as an isolated native single-member replica set with a 0.25 GiB WiredTiger cache;
  both paths use primary reads and majority acknowledgement.
- Elasticsearch **8.19.22** runs in Docker with 2 CPUs, 1,536 MiB memory limit,
  512 MiB heap, one shard and zero replicas. Docker VM: 8 CPUs, approximately
  7.75 GiB RAM. Both paths use the same database instance within each comparison.
- Weir Store concurrency 8, max batch 32, ingress max sessions/connections 64.
  The configured 8 GiB Weir memory budget is an admission declaration, not an OS
  reservation or an observed resident-memory measurement.
- Native baselines issue one operation per call without client bulk or business
  retries. Weir's internal batching and SDK/gRPC work remain part of its path.
  Initialization, dataset setup/reset, warmup and verification are untimed.
- Test/benchmark source: [`0ff03786e637`](https://github.com/batchstream/weir-tests/commit/0ff03786e637d4eca1fba8148328c73b77973aa1).
  Weir source: `42749f6d1653682c76ecb6b84ccaf4be0f3f1aa5`, SDK **v0.2.0**,
  protocol **v0.1.0**, Go **1.27.1**, CGO disabled, client GOMAXPROCS 8.

[Hardware](2026-10-03-local/hardware.json),
[measurement commands and client binary digest](2026-10-03-local/measurement.receipt.json),
[version/checksum locks](2026-10-03-local/versions.json),
[Weir binary receipt](2026-10-03-local/weir.receipt.json) and
[MongoDB binary receipt](2026-10-03-local/mongod.receipt.json) preserve provenance.
Each profile contains its fixture manifest, actual Weir configuration and cleanup
receipt; all owned processes stopped and containers were removed. Loopback
addresses and temporary directories in those files describe retired fixtures.

This is a hot, fixed-working-set replacement/read workload, with no growing
insert dataset, URI affinity, cross-host network, Kubernetes, failure during
measurement or client-native bulk baseline. The host was not reserved exclusively
for benchmarks; three rounds cannot establish confidence intervals or remove all
scheduler/storage variation. Compare individual round results as well as the
pooled ratio. Preliminary measurements from earlier code are excluded.

## Reproduce

The commands below run from the repository root at the recorded source commit,
using the locked native Go toolchain. The native MongoDB option is needed on the
recorded macOS Docker kernel; it does not bypass MongoDB's compatibility guard.
Choose fresh output directories for every repetition.

```sh
make prepare
python3 scripts/prepare_native_mongo.py
GOWORK=off CGO_ENABLED=0 go build -trimpath -o .tools/weir-lab ./cmd/weir-lab
for percent in 0 10 100; do
  .tools/weir-lab -weir .tools/weir -mongod .tools/mongod -backend all \
    -operations 10000 -warmup 1000 -records 1024 -payload-bytes 1024 \
    -concurrency 8 -rounds 3 -write-percent "$percent" -batch-collect 0ms \
    -output "results/local/repeat-$percent-0ms"
done
.tools/weir-lab -weir .tools/weir -mongod .tools/mongod -backend all \
  -operations 10000 -warmup 1000 -records 1024 -payload-bytes 1024 \
  -concurrency 8 -rounds 3 -write-percent 10 -batch-collect 5ms \
  -output results/local/repeat-10-5ms
```

On a compatible Linux kernel, omit `-mongod` and native preparation to use the
locked MongoDB Docker image. That changes the deployment environment and must
be recorded with its own measurements.

## Integration evidence

[Linux CI](https://github.com/batchstream/weir-tests/actions/runs/37092032915)
passed offline race/vet/module-boundary checks, real multi-node race integration
and a verified comparison smoke on both Docker databases at the recorded source.
The [integration log](2026-10-03-local/linux-ci/integration.log) and
[verification record](2026-10-03-local/linux-ci/verification.json) preserve the
result. CI smoke validates the harness; its rates are not capacity measurements.
Node recovery coverage uses graceful withdrawal/restart and seed loss, and does
not claim hard-crash lease-expiry or network-partition coverage.
