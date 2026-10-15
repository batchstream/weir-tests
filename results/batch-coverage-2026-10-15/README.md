# Batching coverage, calibration and independent confirmation

The complete six-workflow experiment shows that explicit collection and bounded
execution can turn fragmented single-record calls into larger physical Search
requests. Pure indexing completes 3.30–4.00 times as many writes/s with
48.2–53.7% lower measured database CPU across all six paired points.
The two-step crawler confirms lower database CPU per completed product
at comparable throughput in three paired rounds, with higher tail latency.
Partial updates retain a rejected native round, which prevents a strict
three-round candidate/native claim. All calibration and formal outcomes are
retained; none establishes a universal setting or maximum sustainable capacity.

The prospective [plan](PLAN.md), [manifest](manifest.json) and
[dispatch receipts](dispatch-receipts.json) fix exactly six workflows. The
candidate, rates and qualification rules were declared before inspecting their
results. No negative point selects a replacement rate or another candidate.

## Scope and immutable sources

- Server W: [`b9dfbc207022e0e8ab76118c58cef59e8bac33f3`](https://github.com/batchstream/weir/commit/b9dfbc207022e0e8ab76118c58cef59e8bac33f3).
- Client, workflow and original crawler auditor C:
  [`3e180ebd886d44d5d936d9a5f5192f8295a3f2d1`](https://github.com/batchstream/weir-tests/commit/3e180ebd886d44d5d936d9a5f5192f8295a3f2d1),
  dispatched through the frozen `randy/batch-coverage-client` reference.
- Analysis was frozen in
  [`2dab140`](https://github.com/batchstream/weir-tests/commit/2dab140)
  before formal dispatch. Its thresholds are independent of measured outcomes.

Within a workflow both Weir paths use the same server source and binary. The
pure-write workflow builds W from its frozen Go input tree; crawler workflows
build W with its immutable revision identity. Their different build procedures
need not produce the same executable hash. Receipts attest each executable
before and after use, source cleanliness, commands and effective controls.

Production defaults remain **32 operations, 8 MiB, 0 s collection and unlimited
adapter executions**. The fixed candidate is **128 operations, 8 MiB, 5 ms and
16 executions**. It is a hypothesis for this experiment, not an automatically
selected or universally optimal configuration. No business repository, SDK,
protocol, deployment or Sink implementation was changed. Lua executes inside
the single Weir process.

| Workload | Logical completion unit | Native single-record baseline | Weir single-record caller | Completion boundary |
| --- | --- | --- | --- | --- |
| Pure write | One indexed document with a changed revision | One native index call | One SDK write | Application acknowledgement; 1 KiB padding |
| Partial update | One changed partial update | One native `_update` | One equivalent backend expression | Application acknowledgement; 16 KiB seeded-document padding |
| Two-step crawler | One completed business product | Four causal record operations, plus the scheduled shared-seller operation | Equivalent Lua transforms | Query visible; `refresh=wait_for`, no additional padding |

All callers send one record at a time; native clients do not receive prebuilt
client batches. Four independent OS clients provide 128 or 512 total workers
for pure writes and 512 for the crawler profiles. Pure writes are closed-loop:
the next request follows completion. Crawler arrivals follow an absolute-time
schedule, with one waiting page per worker and a 10-second end-to-end deadline.
Rejected arrivals, drain work, failures, unknown outcomes and timeouts are
retained. The different logical units and visibility contracts are not pooled.
Latency columns are reported histogram upper bounds, with resolution up to
12.5% + 1 µs; ratios between them compare reported bounds rather than exact
quantiles.

## What changed in batching

The existing Store `batching.max_bytes` now also limits the final aggregated
Search NDJSON body, including generated Lua output and framing. The retained
input estimate remains separate. Actual HTTP request body-byte histograms
observe `client.Do` attempts. An individually legal large operation may exceed
the aggregation target only as a singleton; this is a soft aggregation target,
not a newly imposed per-document size rejection. The separate backend exchange
budget and its pre-read/Scan responsibilities are preserved.

The Store selector stops constructing other compatibility-group candidates
once the oldest candidate is known to be ready. It still observes their session
keys, preserving resource ordering across groups. Existing index/visibility
grouping, caller cancellation, deadlines, drain behavior and execution-slot
ownership remain explicit.

Immediate dispatch with unlimited executions can move fragmented arrivals from
the waiting queue into many simultaneous backend calls before a useful batch
forms. A collection window and a finite execution budget let compatible work
accumulate while the database is busy. These controls already exist; this
experiment tests their interaction rather than adding another hidden worker
queue. A very small execution budget can also underutilize a database, especially
while query-visible requests wait for refresh, so finite does not imply optimal.

The [selector-only benchmark](scheduler/README.md) and its retained outputs
cover 1/8/64/1024 compatibility groups. They measure local scheduler work, exclude
queue construction and make no database-throughput claim. The repeated queue
scan/removal remains quadratic in the high-cardinality worst case.

## Complete short pure-write calibration

[Run 38757498973](https://github.com/batchstream/weir-tests/actions/runs/38757498973)
contains all six declared configurations at both concurrency levels. Each row
below is one alternating native/Weir pair: **2 seconds warmup, 5 seconds timed,
one round**. Every pair passed the source, executable, command, configuration,
quota, process, counter, latency and persisted-postflight checks. All 24 stages
have zero failed, unknown or timed-out operations. These are sensitivity checks;
they are too short to identify an optimum or a stable capacity plateau.

| Case | Max operations | Collection | Execution slots |
| --- | ---: | ---: | ---: |
| immediate | 32 | 0 ms | Unlimited |
| bounded32 | 32 | 5 ms | 16 |
| bounded128, the predeclared anchor | 128 | 5 ms | 16 |
| bounded256 | 256 | 5 ms | 16 |
| longer-wait | 128 | 20 ms | 16 |
| fewer-slots | 128 | 5 ms | 4 |

All cases use the same 8 MiB target, 8192 seeded records and 1 KiB padding.
`N / W` denotes native / Weir. QPS uses complete elapsed time including the
completion tail. CPU ms/write is **sampled, clipped database CPU** divided by
completed writes; uncovered time is not extrapolated. CPU % is the mean of
covered intervals relative to the observed one-CPU quota. Coverage is retained
per stage in [stages.csv](calibration-validation/stages.csv), and ranges from
80.31% upward in these calibration stages.

| Case | Workers | Native writes/s | Weir writes/s | DB CPU % N / W | DB CPU ms/write N / W | Weir records/bulk | Weir KiB/bulk | p99 ms N / W |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| immediate | 128 | 525.6 | 516.9 | 90.1 / 99.0 | 1.443 / 1.541 | 1.16 | 1.34 | 524.3 / 458.8 |
| immediate | 512 | 1672.0 | 1152.4 | 82.9 / 98.1 | 0.398 / 0.817 | 2.04 | 2.36 | 786.4 / 917.5 |
| bounded32 | 128 | 493.4 | 3327.1 | 88.4 / 78.0 | 1.495 / 0.199 | 28.36 | 32.77 | 655.4 / 262.1 |
| bounded32 | 512 | 884.7 | 5917.8 | 76.0 / 88.6 | 0.785 / 0.126 | 30.43 | 35.15 | 983.0 / 229.4 |
| bounded128 | 128 | 557.0 | 2520.9 | 92.0 / 78.4 | 1.639 / 0.259 | 39.25 | 45.34 | 458.8 / 294.9 |
| bounded128 | 512 | 1001.4 | 6445.7 | 77.1 / 94.1 | 0.765 / 0.124 | 67.43 | 77.90 | 983.0 / 213.0 |
| bounded256 | 128 | 556.8 | 3129.3 | 91.4 / 82.7 | 1.580 / 0.222 | 40.05 | 46.27 | 524.3 / 180.2 |
| bounded256 | 512 | 472.4 | 5112.1 | 44.5 / 87.4 | 0.848 / 0.140 | 71.79 | 82.94 | 2359.3 / 327.7 |
| longer-wait | 128 | 382.5 | 3612.1 | 87.8 / 69.5 | 2.279 / 0.162 | 128.00 | 147.88 | 589.8 / 213.0 |
| longer-wait | 512 | 493.8 | 4446.6 | 53.7 / 62.0 | 1.022 / 0.136 | 122.64 | 141.68 | 1966.1 / 491.5 |
| fewer-slots | 128 | 299.0 | 2502.0 | 75.4 / 69.0 | 2.334 / 0.224 | 41.34 | 47.75 | 1179.6 / 327.7 |
| fewer-slots | 512 | 638.6 | 5678.4 | 63.3 / 67.8 | 0.893 / 0.102 | 85.02 | 98.22 | 1310.7 / 294.9 |

The immediate path's small batches and negative paired results are retained.
Bounded configurations form larger batches, but more records per bulk do not
monotonically increase throughput: the longer-wait path forms larger batches
at 512 workers and completes fewer writes/s than the fixed anchor. Increasing
the count target to 256 also does not consistently improve the anchor. The
native control itself varies substantially between these short sequential
points, so these observations do not select a winning setting. At 512 workers
the anchor uses more measured database CPU than its paired native stage;
its higher QPS alone is not an equal-actual-load result. The 8 MiB target is not
approached by the mean bodies in this small-document calibration.

Complete generated evidence, including failed/missing formal placeholders, is
in [calibration-validation/analysis.json](calibration-validation/analysis.json)
and [the acceptance table](calibration-validation/acceptance.md).

## Crawler diagnostics, including negative arrivals

The observer-enabled
[partial-update run](https://github.com/batchstream/weir-tests/actions/runs/38757510460)
and [two-step run](https://github.com/batchstream/weir-tests/actions/runs/38757522402)
use 2-second warmup, 5-second arrival windows and one rotating round. All six
stages passed the original C Go auditor, including all 3073 seeded documents
and untouched fields. Their failures, unknown outcomes and timeouts are zero.
This correctness result includes persisted-state checks for rejected keys; it
does not imply all offered arrivals completed.

| Profile / path | Offered | Completed | Rejected | Completed units/s | DB CPU ms/unit | p99 ms | Drain s | Max generator lag ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Partial / native | 2560 | 1874 | 686 | 198.1 | 5.063 | 5767.2 | 4.459 | 3.717 |
| Partial / immediate | 2560 | 1841 | 719 | 191.5 | 5.242 | 5767.2 | 4.613 | 4.822 |
| Partial / candidate | 2560 | 2560 | 0 | 471.8 | 2.138 | 589.8 | 0.426 | 5.875 |
| Two-step / native | 340 | 340 | 0 | 49.0 | 18.497 | 4718.6 | 1.941 | 1.849 |
| Two-step / immediate | 340 | 340 | 0 | 46.4 | 17.874 | 3932.2 | 2.334 | 23.453 |
| Two-step / candidate | 340 | 340 | 0 | 48.0 | 13.627 | 4718.6 | 2.084 | 22.612 |

The partial native and immediate stages violate the prospective all-arrivals,
p99 and drain limits. Their rejection counts remain explicit; changing the
denominator to only accepted work would hide the offered-load failure. The
candidate diagnostic completes all arrivals, but this short single round is
not formal confirmation. All three two-step stages complete all arrivals within
the diagnostic bounds; their completion tails still lower completed QPS below
the nominal 68 products/s offered during the arrival window.

Actual Weir HTTP attempt counts and request-body bytes agree with the
independent observer for every active phase in both diagnostics. Native
application-call counts also match observed native update attempts. The observer
is disabled for formal measurements. The measured Weir request sizes are:

| Profile / path | Metadata attempts | `_mget` attempts | Bulk attempts | Records/bulk | Bulk body bytes, total | Lua evaluations | Explicit refresh attempts |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Partial / immediate | 1820 | Unavailable | 1820 | 1.012 | 453400 | Unavailable | Unavailable |
| Partial / candidate | 269 | Unavailable | 269 | 9.517 | 630784 | Unavailable | Unavailable |
| Two-step / immediate | 570 | 633 | 569 | 2.390 | 5381180 | 1364 | 4 |
| Two-step / candidate | 129 | 272 | 129 | 10.543 | 5381860 | 1364 | 4 |

Missing before/after metric series remain unavailable, rather than inferred
zeros. The partial profile executes no Lua or external `_mget` by design, and
its observer has no corresponding traffic. Request bodies and record counts
describe framed HTTP attempts, not independent proof of delivery or applied
updates. The different total partial bytes also reflect different numbers of
completed operations. Two-step's 1364 Lua evaluations include 1360 mutations
framed into bulk and four unchanged shared-seller visibility barriers. Larger
bulk requests do not remove those read/modify/write or visibility requirements.
Bulk HTTP duration includes `refresh=wait_for`; summing concurrent HTTP
durations does not measure database CPU or wall-clock elapsed time.

The source-level causes addressed here are the disconnect between retained
input bytes and actual generated NDJSON, and avoidable selector work across
many compatibility groups. Calibration separately exercises count, collection
and execution-slot sensitivity. The candidate changes several controls
together, so these diagnostics do not attribute its entire effect to one
parameter or prove all remaining database overhead is eliminated.

## Correctness coverage and remaining boundaries

The retained [real-backend receipt and logs](integration/receipt.json) cover
Elasticsearch 8.19.22 and OpenSearch 3.3, four gRPC connections, four execution
slots, 20 ms collection, eight indexes with 80% hot-index traffic, 1/256 KiB
sources and 10% shared-key increments. Applied outcomes are checked against
persisted counters, native versions and untouched fields. Confirmed hot-key
conflicts remain accounted `NOT_APPLIED`; oversized aggregate bodies must
contain one individually legal large item. A new scheduler test initially
used a short caller deadline that correctly triggered immediate dispatch;
the pre-dispatch test-only fix separates caller cancellation from its bounded
test wait. It did not change production deadline behavior or benchmark code.

These real-backend tests establish correctness of the added combinations, not
OpenSearch throughput. Quantitative workflows here use an owned Elasticsearch
container on one runner, with an observed one-CPU quota. Four client processes
exercise concurrency and independent pools but share loopback networking and
one host with Weir. There is no production network, multi-node cluster,
distributed-host failure, replica-load or MongoDB throughput claim. Eight-index
skew and hot-key contention are correctness coverage, not a measured performance
matrix for those axes. No arbitrary same-key operations are coalesced.

## Formal confirmation results and gates

Each formal stage has **60-second warmup and 60-second timing**, with three
paired or rotating rounds and the diagnostic observer disabled. Every row below
retains complete elapsed time including the completion tail. Source, executable,
configuration, quota, cleanup, full-field correctness and clipped CPU checks
are separate from the all-arrivals and performance comparisons. All 30 formal
stages pass measurement checks; 17 of the 18 crawler stages pass all-arrivals
checks. The complete
generated decisions are in [analysis/acceptance.md](analysis/acceptance.md),
[analysis/comparisons.csv](analysis/comparisons.csv) and
[analysis/analysis.json](analysis/analysis.json).

All paired stages need zero failed/unknown/timed-out work and at least 80%
clipped CPU coverage. Crawler all-arrivals qualification additionally requires
no rejection, offered=completed, p99 upper bound <=5 s, drain <=3 s and maximum
generator lag <=50 ms. Lower CPU at comparable crawler throughput requires
completed-QPS ratios in [0.95,1.05] and lower CPU ms/completed unit in every
qualified paired round. Pure-write faster throughput at greater actual CPU is
not an equal-load win. Capacity additionally requires sustained database CPU
pressure and a repeatable next-concurrency plateau; equal quotas are insufficient.

### Pure write

[Run 38759209569](https://github.com/batchstream/weir-tests/actions/runs/38759209569)
retains all twelve stages. Each completion is one changed indexed document;
four client processes send one record per call. `N / W` means native / candidate.

| Workers | Round | Native writes/s | Candidate writes/s | DB CPU % N / W | DB CPU ms/write N / W | p99 ms N / W | Candidate records/bulk | Candidate KiB/bulk | CPU coverage % N / W |
| ---: | ---: | ---: | ---: | --- | --- | --- | ---: | ---: | --- |
| 128 | 1 | 1690.57 | 6075.05 | 73.58 / 34.10 | 0.43 / 0.06 | 180.22 / 45.05 | 40.62 | 46.93 | 99.69 / 99.51 |
| 128 | 2 | 1832.83 | 6039.49 | 65.92 / 34.12 | 0.36 / 0.06 | 90.11 / 40.96 | 40.32 | 46.58 | 98.95 / 99.93 |
| 128 | 3 | 1763.67 | 6142.12 | 64.80 / 31.81 | 0.36 / 0.05 | 90.11 / 40.96 | 40.98 | 47.35 | 98.78 / 99.64 |
| 512 | 1 | 1835.00 | 7226.38 | 61.80 / 30.55 | 0.33 / 0.04 | 393.21 / 163.84 | 74.15 | 85.66 | 98.60 / 98.66 |
| 512 | 2 | 1819.03 | 7273.75 | 62.52 / 30.86 | 0.34 / 0.04 | 425.98 / 147.46 | 73.56 | 84.98 | 98.62 / 98.48 |
| 512 | 3 | 1868.73 | 7382.68 | 64.01 / 30.29 | 0.34 / 0.04 | 360.45 / 147.46 | 73.71 | 85.16 | 98.64 / 98.31 |

All twelve pure-write stages pass measurement and complete-state checks with
zero failed, unknown or timed-out writes. At 128 workers the candidate completes
3.30–3.59 times as many writes/s as native; at 512 it completes 3.94–4.00 times
as many. Its actual measured database CPU is lower in every pair: 31.81–34.12%
versus native 64.80–73.58% at 128 workers, and 30.29–30.86% versus 61.80–64.01%
at 512. The frozen higher-throughput/no-more-measured-CPU gate passes at both
concurrencies. None of these twelve stages meets sustained database CPU
saturation, so these fixed-concurrency observations establish no maximum
capacity or equal-load capacity curve. The completed-QPS ratios are far outside
[0.95,1.05], making the separate comparable-QPS lower-CPU gate inapplicable.

### Partial update

[Original workflow](https://github.com/batchstream/weir-tests/actions/runs/38758792930) retains all nine stages. A completion is one update.

| Round | Path | Completed / offered | Rejected | Completed updates/s | DB CPU % | DB CPU ms/update | p99 ms | Drain s | Max lag ms | All arrivals qualified |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | Native | 29696 / 30720 | 1024 | 494.67 | 80.11 | 1.62 | 2621.44 | 0.03 | 3.13 | No |
| 1 | Immediate | 30720 / 30720 | 0 | 511.75 | 88.40 | 1.73 | 1441.79 | 0.03 | 6.76 | Yes |
| 1 | Candidate | 30720 / 30720 | 0 | 511.71 | 60.24 | 1.18 | 262.14 | 0.03 | 3.86 | Yes |
| 2 | Native | 30720 / 30720 | 0 | 511.78 | 62.45 | 1.22 | 294.91 | 0.03 | 3.84 | Yes |
| 2 | Immediate | 30720 / 30720 | 0 | 504.56 | 86.88 | 1.72 | 1572.86 | 0.89 | 4.86 | Yes |
| 2 | Candidate | 30720 / 30720 | 0 | 511.76 | 56.82 | 1.11 | 65.53 | 0.03 | 3.08 | Yes |
| 3 | Native | 30720 / 30720 | 0 | 511.36 | 63.32 | 1.24 | 122.88 | 0.08 | 2.00 | Yes |
| 3 | Immediate | 30720 / 30720 | 0 | 511.72 | 83.19 | 1.63 | 851.97 | 0.03 | 5.02 | Yes |
| 3 | Candidate | 30720 / 30720 | 0 | 511.76 | 55.97 | 1.09 | 114.69 | 0.03 | 3.28 | Yes |

### Two-step crawler

[Original workflow](https://github.com/batchstream/weir-tests/actions/runs/38758812622) retains all nine stages. A completion is one product.

| Round | Path | Completed / offered | Rejected | Completed products/s | DB CPU % | DB CPU ms/product | p99 ms | Drain s | Max lag ms | All arrivals qualified |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | Native | 4080 / 4080 | 0 | 65.85 | 36.09 | 5.48 | 3145.73 | 1.96 | 8.82 | Yes |
| 1 | Immediate | 4080 / 4080 | 0 | 66.27 | 35.21 | 5.31 | 3407.87 | 1.56 | 15.30 | Yes |
| 1 | Candidate | 4080 / 4080 | 0 | 65.39 | 20.92 | 3.20 | 4194.30 | 2.39 | 23.24 | Yes |
| 2 | Native | 4080 / 4080 | 0 | 66.24 | 22.66 | 3.42 | 3145.73 | 1.60 | 8.90 | Yes |
| 2 | Immediate | 4080 / 4080 | 0 | 66.27 | 29.03 | 4.38 | 3407.87 | 1.57 | 19.34 | Yes |
| 2 | Candidate | 4080 / 4080 | 0 | 65.43 | 19.09 | 2.92 | 4194.30 | 2.36 | 22.30 | Yes |
| 3 | Native | 4080 / 4080 | 0 | 66.27 | 20.77 | 3.13 | 3145.73 | 1.57 | 13.13 | Yes |
| 3 | Immediate | 4080 / 4080 | 0 | 66.32 | 30.12 | 4.54 | 3407.87 | 1.52 | 22.18 | Yes |
| 3 | Candidate | 4080 / 4080 | 0 | 65.46 | 17.70 | 2.70 | 4194.30 | 2.33 | 13.60 | Yes |

### Paired decisions and tradeoffs

Changes below are candidate relative to the indicated baseline, in rounds 1 / 2 / 3.
The three-round result requires every pair to qualify; a favorable numerical
ratio cannot override rejected arrivals. “Lower CPU” uses CPU ms/completed unit.

| Profile | Candidate / baseline | QPS changes R1 / R2 / R3 | DB CPU/unit changes R1 / R2 / R3 | Actual DB CPU changes R1 / R2 / R3 | Comparable-QPS lower-CPU gate | Higher write QPS at no more CPU gate | Capacity gate |
| --- | --- | --- | --- | --- | --- | --- | --- |
| write 128 workers | Candidate / Native | +259.35% / +229.52% / +248.26% | -87.13% / -84.14% / -85.78% | -53.65% / -48.25% / -50.91% | False | True | False |
| write 512 workers | Candidate / Native | +293.81% / +299.87% / +295.06% | -87.44% / -87.67% / -88.06% | -50.57% / -50.63% / -52.67% | False | True | False |
| partial | Immediate / Native | +3.45% / -1.41% / +0.07% | +6.67% / +41.11% / +31.29% | +10.35% / +39.12% / +31.38% | False | False | False |
| partial | Candidate / Native | +3.44% / -0.00% / +0.08% | -27.31% / -9.02% / -11.67% | -24.81% / -9.02% / -11.61% | False | False | False |
| partial | Candidate / Immediate | -0.01% / +1.43% / +0.01% | -31.85% / -35.53% / -32.73% | -31.86% / -34.60% / -32.72% | True | False | False |
| crawler | Immediate / Native | +0.64% / +0.04% / +0.08% | -3.06% / +28.09% / +44.90% | -2.44% / +28.14% / +45.01% | False | False | False |
| crawler | Candidate / Native | -0.70% / -1.22% / -1.22% | -41.63% / -14.70% / -13.73% | -42.04% / -15.74% / -14.78% | True | False | False |
| crawler | Candidate / Immediate | -1.32% / -1.26% / -1.29% | -39.79% / -33.41% / -40.46% | -40.58% / -34.25% / -41.23% | True | False | False |

For the two-step crawler, all nine stages complete all 4080 arrivals, with zero
failures, unknowns, timeouts or rejections. The candidate reduces database CPU
per product against native in all three rounds at comparable completed throughput.
The reported p99 upper bounds increase from native 3145.727 ms in each round
to candidate 4194.303 ms, about 33.3% higher as reported bounds. Candidate drains
are about
2.33–2.39 s, compared with native 1.57–1.96 s. Both stay within the declared
limits; those limits do not make the latency cost disappear. Immediate Weir does
not establish a stable CPU saving against native.

For partial updates, native round 1 rejects 1024 of 30720 arrivals. Therefore
candidate/native does **not** pass the strict three-round comparison, even though
its observed CPU/unit is lower. The candidate and immediate paths each complete
all 30720 arrivals in every round, and their comparison passes the lower-CPU gate.
Immediate Weir uses more database CPU per update than native in all three pairs.

### Actual batching and within-stage drift

Each range below spans all three formal rounds. These are actual framed HTTP
attempt histograms, not configured batch sizes. Missing metric series remain
unavailable. The partial candidate forms roughly three records per bulk at this
rate after long warmup; the much larger short-diagnostic batch was not a promise
of sustained batch size. The two-step candidate still pays external pre-reads,
Lua evaluation, full-source replacements and query-visible waits.

| Profile | Path | Records/bulk | KiB/bulk | CPU coverage % |
| --- | --- | ---: | ---: | ---: |
| write 128 workers | Candidate | 40.32–40.98 | 46.58–47.35 | 99.51–99.93 |
| write 512 workers | Candidate | 73.56–74.15 | 84.98–85.66 | 98.31–98.66 |
| partial | Immediate | 1.00–1.00 | 0.24–0.24 | 99.99–99.99 |
| partial | Candidate | 3.09–3.15 | 0.75–0.76 | 99.99–99.99 |
| crawler | Immediate | 2.19–2.26 | 8.71–8.97 | 99.99–99.99 |
| crawler | Candidate | 11.99–12.31 | 47.66–48.93 | 99.99–100.00 |

Longer warmup does not establish steady state. The table retains the measured
first-half and second-half CPU means for every formal stage. For example, the
first native partial stage drops from about 89.75% to 70.47% within one stage;
the first native crawler stage drops from 44.01% to 28.17%. All interval samples
are in the generated analysis. No result below proves maximum sustainable
capacity or a production-network benefit.

| Profile | Workers | Round | Path | First-half DB CPU % | Second-half DB CPU % |
| --- | ---: | ---: | --- | ---: | ---: |
| crawler | 512 | 1 | Immediate | 38.34 | 32.08 |
| crawler | 512 | 1 | Native | 44.01 | 28.17 |
| crawler | 512 | 1 | Candidate | 23.57 | 18.27 |
| crawler | 512 | 2 | Immediate | 29.46 | 28.61 |
| crawler | 512 | 2 | Native | 22.55 | 22.76 |
| crawler | 512 | 2 | Candidate | 18.20 | 19.98 |
| crawler | 512 | 3 | Immediate | 30.66 | 29.58 |
| crawler | 512 | 3 | Native | 21.13 | 20.41 |
| crawler | 512 | 3 | Candidate | 16.71 | 18.70 |
| partial | 512 | 1 | Immediate | 91.20 | 85.60 |
| partial | 512 | 1 | Native | 89.75 | 70.47 |
| partial | 512 | 1 | Candidate | 56.74 | 63.73 |
| partial | 512 | 2 | Immediate | 84.62 | 89.15 |
| partial | 512 | 2 | Native | 65.96 | 58.95 |
| partial | 512 | 2 | Candidate | 56.11 | 57.52 |
| partial | 512 | 3 | Immediate | 82.87 | 83.50 |
| partial | 512 | 3 | Native | 59.02 | 67.61 |
| partial | 512 | 3 | Candidate | 54.30 | 57.63 |
| write | 128 | 1 | Native | 75.74 | 71.40 |
| write | 128 | 1 | Candidate | 31.44 | 36.79 |
| write | 128 | 2 | Native | 64.61 | 67.26 |
| write | 128 | 2 | Candidate | 31.24 | 37.00 |
| write | 128 | 3 | Native | 64.71 | 64.89 |
| write | 128 | 3 | Candidate | 28.92 | 34.72 |
| write | 512 | 1 | Native | 62.10 | 61.50 |
| write | 512 | 1 | Candidate | 27.78 | 33.40 |
| write | 512 | 2 | Native | 63.16 | 61.86 |
| write | 512 | 2 | Candidate | 28.30 | 33.51 |
| write | 512 | 3 | Native | 64.15 | 63.87 |
| write | 512 | 3 | Candidate | 28.14 | 32.52 |

## Reproduce and inspect

`raw/<label>/files.json` lists every retained compressed artifact using safe
relative paths. The auditor checks compressed SHA-256, decompressed SHA-256 and
byte length, then immutable workflow/dispatch identity, original receipts,
effective routes, live metric boundaries and owned cleanup. Raw artifact
directory structure is preserved, including the six separate calibration
fixtures. Unknown metrics remain JSON `null`; they are not zero-filled.

The crawler auditor is rebuilt from the original C archive. Its exact native
build receipt is [auditor-build.json](auditor-build.json); the current receipt
is Go 1.27.1 on Darwin arm64. Executable hashes are platform-specific. The full
analysis command intentionally checks that receipt and hash, so another
platform's original-source auditor may independently validate reports but does
not silently substitute a different binary for the recorded analysis.

From this repository, with the matching original auditor already built:

```sh
python3 -B -m unittest discover -s results/batch-coverage-2026-10-15 -p test_analysis.py
python3 -B -O -m unittest discover -s results/batch-coverage-2026-10-15 -p test_analysis.py
python3 -B results/batch-coverage-2026-10-15/analyze.py \
  --auditor .tools/weir-crawler-coverage-audit \
  --output /tmp/weir-batch-coverage-reproduced
```

During collection, `--partial` retains every planned workflow and explicitly
marks unavailable evidence. After all workflows are archived, omit `--partial`
to require the complete inventory. Outputs are `analysis.json`, `runs.csv`,
`stages.csv`, `comparisons.csv` and `acceptance.md`. No command above contacts
a database or reruns a benchmark. The [diagnostic audit](diagnostic-validation/analysis.json)
and [calibration audit](calibration-validation/analysis.json) are preserved as
the pre-confirmation snapshots rather than overwritten by later results.
