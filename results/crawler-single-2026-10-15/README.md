# Distributed single-record crawler benchmark, 2026-10-15

This experiment tests Weir with independently arriving single-record writes. It corrects the earlier [page-20 client control](../crawler-2026-10-15/README.md), which gave callers an already assembled batch and therefore did not represent the reference worker's individual product and seller calls. Only the test repository changes; business and production Weir code remain untouched. Sink and new Lua helper APIs are excluded. The metric is database load and completed business throughput; extra Weir CPU is outside the requested comparison.

## Submission and business completion

Four OS client processes own 32 goroutine workers each. One business product arrives at a time, scheduled from absolute timestamps independently of prior completion. Each worker reconciles one product, waits for query visibility, updates that product, waits again, then independently submits its two sellers concurrently and joins both acknowledgements. The synthetic shared placeholder adds a third separate seller when `(worker + page) % 100 == 0`. Every native HTTP `_update` and every Weir SDK `Mutate` call contains exactly one record. Native uses cached atomic Painless; Weir runs the frozen Lua translation outside the database. Neither caller creates a batch, adds an aggregation window or shares a client-side aggregator across processes.

The native HTTP pool allows 96 connections per process (three per worker), sufficient for the maximum seller fan-out. Each worker records actual application invocation attempts and their record-count histogram; child and stage totals are recomputed and audited. For successful work, the independently regenerated acknowledged arrivals must account for the same number of logical records. Empty workers and partial failures are retained. These counters prove input submission shape; they do not count every transport retry, wire message or database-facing Weir command. Native HTTP attempts and Weir SDK calls are distinguished in the analysis.

A completed business product requires every causal phase and seller to be durably acknowledged and query-visible. Business latency runs from the planned arrival to the final acknowledgement, including queueing. Throughput divides successful products by the full arrival window plus every completion drain. An arrival has four logical record mutations, plus a possible placeholder; reported shared counts are actual synthetic counts, not a production percentage. Rejected arrivals have no completed latency. A failed business product can have partially applied records, so an oracle mismatch after failure does not establish loss after a successful acknowledgement.

All 769 seeded documents, including untouched or rejected keys, are read by normal searchable state and compared against an independent field/type oracle. The offline Go audit reconstructs the canonical expected-document hash from acknowledged arrival ordinals. Input/output semantics cover the generated fixture, not arbitrary Lua or every production branch. Individual arrival shape also reduces the seeded working set relative to page20; do not subtract those runs and call the result a pure batching effect.

## Fixed controls and provenance

Client source is `ad1879156c14028285f27f18794f1580e979af63`. Both Weir paths use exactly `c3101c6628872345d6fb09b977ce853f25936b79` and matching binary hashes. The default path uses `max_in_flight=0`; the experimental path alone uses 16, attested by live before/after gauges. All three paths within each workflow share one owned Elasticsearch container with 1 CPU quota, 512 MiB JVM heap and 1536 MiB container memory. The two indices each have one primary, zero replicas, one-second refresh and request translog durability. Native and Weir each allow four total conflict attempts. Request deadlines remain 10 s and the client queue remains one business arrival per worker.

Every path bypasses the HTTP measurement proxy. Database-facing HTTP counts and bytes therefore remain unavailable, not zero. Database CPU comes from cumulative owned-container counters clipped to measured intervals without extrapolation; CPU coverage is reported. Native index/get/refresh/merge statistics and Weir adapter records/execution counters are retained. Adapter fill shows server aggregation of separate calls; it is not a complete database request count. Bounded owned database logs and process/container cleanup receipts are archived with raw reports.

Pools remain open across warmup, a deterministic reset and measurement. Both resets require at least two quiet seconds of index/merge/flush observations. Paths rotate within each workflow; persistent database history and JVM/JIT effects may still vary by order. Server/client source, clean state, executable hashes before/after, exact command, dependency versions, CPU/host/kernel, image identity and actual quota are verified from receipts. Each formal rate has its own workflow and host, with three paired paths and three rotating rounds. Comparisons stay within that workflow and round; absolute rates or CPU across runners are never joined into a capacity curve.

The [predeclared plan](PLAN.md) fixes calibration and rate selection before the results. Calibration uses 4/16/48 offered products/s, 2 s warmup, 5 s arrivals and one round. It is diagnostic only. Formal runs use 10 s warmup, 20 s arrivals and three rounds. Full measurement requires matching provenance, complete matrix, successful cleanup and field/offline audits, no failed/unknown/timed-out dispatched work, and >=80% CPU coverage. The all-arrivals gate additionally requires no rejection, all arrivals completed, business p99 <=5 s, drain <=3 s and generator lag <=50 ms in every round. Quantiles are inclusive histogram upper bounds with at most 12.5% plus 1 microsecond approximation. These gates are experiment limits, not production SLOs.

Lower CPU at comparable completed throughput additionally requires paired completed QPS within +/-5% in every round and a consistent CPU-per-product direction across all three rounds. Equal CPU quota means equal budget, not equal observed database load. A throughput improvement at equal actual load needs a matching observation; a single offered rate or a larger batch alone does not prove it. Three rounds do not establish production capacity or a precise confidence interval.

## Reproduce

Each raw directory includes deterministic gzip artifacts and a `files.json` inventory with compressed SHA-256, original SHA-256 and original byte count. The experiment's `summarize.py` calculation is frozen locally rather than importing the mutable repository summarizer. The generated analysis records the hashes of its scripts and the exact offline auditor binary. The manifest binds each report to its workflow and immutable client.

From the repository root, build the offline auditor with the declared Go version and rerun analysis without databases:

```sh
go build -trimpath -buildvcs=false -o .tools/weir-crawler-single-audit ./cmd/weir-crawler
python3 results/crawler-single-2026-10-15/analyze.py \
  --manifest results/crawler-single-2026-10-15/manifest.json \
  --auditor .tools/weir-crawler-single-audit \
  --output results/crawler-single-2026-10-15/analysis
```

Optional plots use `requirements-plot.txt`:

```sh
python3 results/crawler-single-2026-10-15/plot.py \
  --analysis results/crawler-single-2026-10-15/analysis/analysis.json \
  --output results/crawler-single-2026-10-15/plots
```

Every failed, rejected or unmeasured stage remains in the stage CSV. Failed Go audits remain failure evidence and never become qualified measurements. Cleanup receipts certify the final owned process and removed database container; earlier server replacements synchronously wait in fixture code but do not have separate historical OS termination receipts. Clients, Weir and the owned database run on the same host; independent machines, network RTT, production key distributions, replicas and multi-node database scaling are not modeled.
