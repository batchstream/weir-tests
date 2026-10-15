# Measured batching defaults and capacity boundaries

The selected defaults are **128 operations / 4 MiB / 20 ms / 16 concurrent adapter
invocations per Store**. Independent fixtures omit the entire batching section;
their effective configuration and all four runtime gauges agree with these
values. Explicit zero wait and execution limits remain zero.

The practical benefit is aggregating fragmented calls from independent producers.
At 0.25 database CPU and 512 workers, three 60-second rounds complete **6.74–8.34
times as many Search writes at nearly the same actual database CPU**. The
query-visible crawler completes approximately the same 66 products/s while
using **8.1–42.3% less actual database CPU**. These are observed workload results,
not a universal optimum or a proved production capacity.

## What was measured

All primary native calls contain one record. Four independent OS processes own
their driver/SDK connections and issue the same changes using disjoint keys.
There is no client batching or Sink baseline. Search has one primary per index,
zero replicas, request translog durability and a one-second refresh interval.
Fixtures explicitly allow 4096 transport pending records; the four omitted batching
controls are the defaults under selection, rather than an entirely default server
configuration. Queue and memory defaults are not selected by this study.
Acknowledgement and query visibility match within each comparison. Every update
changes a revision; independent full-field persisted oracles check the result.
Business repositories, SDK, protocol and deployment charts were not modified.

The frozen [plan](PLAN.md), [joint byte confirmation](CONFIRMATION-ADDENDUM.md),
[lower rate bracket](LOWER-BRACKET-SUPPLEMENT.md) and
[candidate-only supplement](CANDIDATE-CAPACITY-SUPPLEMENT.md) retain their original
rules. Selection measured 20 count/wait/execution combinations across pure
writes, partial updates, dependent crawler updates and Search/Mongo read guards;
four further byte targets used 64 KiB bodies. [Every ranking and exclusion](SELECTION.md)
is retained. Eleven protective candidates qualified, seven entered the 5%
throughput frontier, and one remained in the CPU plateau. The byte rule chose
4 MiB, the smaller target in the joint 4/8 MiB plateau.

The original independent confirmation inventory has 14 workflows and 297 planned
stages, in addition to 288 selection stages. It covers two database quotas,
three-round pure-write concurrency ladders, two crawler rate ladders with lower
brackets and independent candidate-only cases, Search/Mongo reads, Mongo writes,
90/10 mixed traffic, Lua mutations and 64 KiB Search writes. The
[complete corrected analysis](analysis-confirmation-parser-fix/analysis.json)
contains 549 audit rows; rows are decisions, not a claim that every planned timed
stage ran successfully. All warmup failures, missing stages, rejected arrivals,
unknown writes, timeouts and cleanup errors remain visible.

Actual CPU is integrated from owned database container counters over the complete
measurement and drain interval. Equal quotas alone do not establish equal load.
CPU per completed unit, whole-stage completion counts, completion-time buckets,
host/client headroom, effective quotas, source hashes and runtime configuration
are audited. The accepted completion p99 upper bound is **5 seconds**. Extra Weir
CPU is reported as a resource observation; reducing total system CPU is not an
adoption requirement.

## Same database load, higher completed throughput

Search, 1 KiB payload padding, 8192 records, 0.25 database CPU, 512 workers across
four processes, 60-second warmup and 60-second timed stages:

| Round | Native writes/s | Weir writes/s | Ratio | Native / Weir actual DB quota CPU | Native / Weir p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1 | 643.322 | 5270.143 | 8.192× | 99.821% / 99.138% | 1572.863 / 327.679 |
| 2 | 664.674 | 4480.537 | 6.741× | 99.983% / 99.481% | 1310.719 / 327.679 |
| 3 | 655.657 | 5468.189 | 8.340× | 100.047% / 99.141% | 1310.719 / 294.911 |

Both paths pass source, persisted oracle, zero-error, SLO, CPU observation,
host/client headroom and half-window stability gates in all three rounds. Weir's
actual CPU is 99.1–99.5% of the paired native CPU. These points prove a measured
throughput improvement at comparable actual database load. They do not yet prove
a sustainable maximum: the next concurrency still gains more than 10% in two
rounds, and its third round fails half-window stability.

The database request measurements also show the aggregation mechanism: Weir's
three 512-worker rounds average 117.565/115.523/118.597 records per attempted bulk,
with 2692/2329/2768 bulk attempts completing 316485/269052/328276 records. Native
still submits one record per call. Reducing request overhead does not remove the
database's per-document indexing work.

## Comparable completed crawler throughput, lower database load

The original `two-step` profile preserves three ordered query-visible phases:
product reconciliation, the product update, then the two seller updates in
parallel. It touches one product and two seller documents. At 68 offered
products/s, 512 workers, one database CPU and three 60-second rounds:

| Round | Native products/s | Weir products/s | Native / Weir p99 ms | Native / Weir DB CPU ms/product |
| --- | ---: | ---: | ---: | ---: |
| 1 | 65.737 | 65.676 | 3407.871 / 3932.159 | 5.705 / 3.293 |
| 2 | 65.355 | 65.502 | 3145.727 / 4194.303 | 3.052 / 2.799 |
| 3 | 66.312 | 65.457 | 3145.727 / 3932.159 | 3.196 / 2.478 |

All arrivals complete and both paths pass the paired workload gates. Completed
throughput differs by −1.3% to +0.2%; actual database CPU falls by 8.1–42.3%.
Weir trades some tail latency for this saving while remaining below five seconds.
Neither path saturates the one-CPU database, so this establishes lower load at
comparable throughput rather than maximum capacity.

## Regression and broader workload checks

- **Reads:** all 12 independently repeated Search/Mongo guards pass. The
  native-normalized default/legacy throughput ratios are 0.934–1.065. Each p99
  stays below the declared `max(legacy × 1.25, legacy + 2 ms)` guard. See the
  [machine-readable guard report](read-guards.json). Reads do not wait to collect
  future arrivals.
- **Mongo:** all 36 write/mixed/Lua audit rows qualify. Normalizing each control
  by its own paired native stage, default/legacy throughput is 1.079–1.100 for
  ordinary writes, 0.989–1.022 for 90/10 mixed traffic and 1.251–1.331 for Lua.
  Corresponding CPU-per-completed-unit ratios are 0.586–0.644, 0.934–0.987 and
  0.363–0.454. These normalized ratios are not direct same-load capacity ratios.
- **64 KiB Search bodies:** the actual combined default completes
  1891.298/1978.225/2089.408 writes/s versus its native
  961.448/1399.400/1305.379. Its attempted bulk requests average
  47.73–48.82 records and 2.990–3.059 MiB, versus legacy's 1.35–1.72 records.
  CPU per completed write falls 23.3–41.3%, but actual database CPU increases
  4.6–16.0%; this is not a same-load throughput claim. All three default rounds
  meet the SLO, full-field and half-window stability checks.
- **Partial updates, 512 arrivals/s, one CPU:** Weir completes all 30,720 arrivals
  in each round; native rejects 4608/3072/2625. These native failures invalidate
  a same-throughput CPU-saving comparison and remain negative evidence.

The read guard compares the new defaults with explicit legacy Weir settings.
The absolute read-only comparison with native calls has these three-round ranges:

| Backend | Total workers | Default Weir/native completed QPS | Default Weir p99 ms |
| --- | ---: | ---: | ---: |
| Search | 4 | 0.326–0.394× | 4.607–5.119 |
| Search | 128 | 0.682–0.709× | 61.439–65.535 |
| Mongo | 4 | 0.418–0.447× | 2.303–2.815 |
| Mongo | 128 | 1.236–1.289× | 32.767 |

The extra service hop has a substantial cost in the low-concurrency read cases.
The measured adoption case is collecting concurrent writes and reducing database
CPU for the crawler; the table retains the read-only counterexamples.

## Capacity remains a separate qualification

The initial confirmation does not establish a Weir maximum. At 0.25 CPU, the
512-to-1024 pure-write gains are 11.1%, 40.6% and 9.0%; the final 1024-worker round
has a second/first-half completion ratio of 0.864. At one CPU, Weir's high
concurrency fills the shared host while the database uses only about one-third
of its quota. Host saturation is not database saturation.

Six paired crawler/partial cases fail whole-case lifecycle qualification after
native warmup timeouts and subsequent server startup failures; see
[all retained lifecycle failures](LIFECYCLE-FAILURES.md). Independent candidate-only
quarter-CPU crawler cases still have no rate with three qualified rounds. The
independent partial ladder has qualified lower rates 32 and 64, but its next rate
has incomplete evidence rather than a genuine service-overload upper bound.
Unknown or unexecuted work does not establish an upper bound.

The [original soak derivation](soak-derivation-no-qualified-points.json) consequently
selects **zero checks**, and none was dispatched from those results. Its
[receipt](soak-derivation-no-qualified-points-receipt.json) and all failed gates
remain immutable. A separately preregistered
[one-eighth-CPU pure-write study](PURE-CAPACITY-QUOTA-SUPPLEMENT.md) failed during
fixture preparation: the Elasticsearch JVM was still initializing at the nested
180-second readiness deadline. No Weir/client process, warmup or timed stage
started. All 399 retained files and the successful owned-container cleanup remain
in `raw/confirm-pure-q0125/`. The
[21-workflow audit](analysis-pure-quota-extension/analysis.json) preserves all 549
previous rows unchanged, adds this preparation failure with zero performance
rows, and [still derives zero soak checks](soak-derivation-after-preparation-failure.json).
The startup failure cannot establish a capacity upper bound.

A separately registered [preparation correction](STARTUP-PREPARATION-CORRECTION.md)
removes that duplicate timeout and inherits the existing eight-minute whole-fixture
deadline. Its production source change is confined to the fixture's `readySearch`
function; a virtual-time regression fails at three minutes on the old code and
passes at the parent's eight-minute deadline after the fix. Weir, database quota,
per-request deadlines, measurement code, embedded workloads and numerical gates
remain unchanged. This named correction cohort and any eligible conditional
five-minute pure check were audited in the
[22-workflow analysis](analysis-startup-corrected-confirmation/analysis.json).
All 18 new stages are usable; the [equivalence receipt](startup-corrected-analysis-equivalence.json)
preserves the previous 549 rows and both selectors unchanged. At 512 workers:

| Round | Native writes/s | Weir writes/s | Ratio | Native / Weir actual DB quota CPU | Weir p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1 | 677.454 | 5197.600 | 7.672× | 103.409% / 103.640% | 393.215 |
| 2 | 695.808 | 5379.045 | 7.731× | 103.944% / 103.603% | 425.983 |
| 3 | 695.287 | 5794.242 | 8.334× | 103.967% / 103.947% | 327.679 |

Actual container CPU is approximately 0.129–0.130 cores; the quota label alone
would understate the observation. The paired actual CPU ratios are
0.99672–1.00223. These three points pass the original quality and stability gates,
but the 512-to-1024 gains are 11.220%, 12.062% and 3.510%; the third 1024-worker
round's second/first-half ratio is 0.879494. No Weir capacity point qualifies,
and [the unchanged rules still derive zero soak checks](soak-derivation-after-startup-correction.json).
The first 256-worker round has strong cold-state effects and its larger ratio
is not used as the principal benefit claim. The original failed workflow is retained;
there is no favorable performance retry or adaptive startup extension.

## Reproduction and analysis corrections

The [manifest](manifest.json) binds every workflow to its committed plan, plan
hash, immutable client/server revisions and workflow URL. Each `raw/<label>`
inventory preserves every artifact using deterministic gzip with compressed and
uncompressed SHA256 hashes and lengths. Source snapshots, binaries, embedded Lua
and the original independent Go auditor are verified before analysis. The server
Go source is `084572d79b7a53230801297f90ca2363fd864913`; the public-module lock and
[module receipt](selected-default-public-module.json) resolve to that exact source.

Two analysis defects were corrected transparently. The first incorrectly
iterated unavailable Search metrics for Mongo. The second reused the strict
single-unit plan parser for raw Go durations such as `1m0s`, initially excluding
four cases. Original outputs remain in `analysis-selection/` and
`analysis-confirmation/`. The narrowly corrected confirmation parser is frozen at
`175272e0e8479cea6f2a03c215e285910e466376`; [the equivalence receipt](confirmation-parser-equivalence.json)
confirms all original 501 rows and both selectors are unchanged, with 48 recovered
rows. No measured data, selection threshold, capacity gate or original auditor
was changed by either correction.

The preparation-cohort source audit is separately frozen at
`b934c20ab3d09d6d2aa5bcdd0e6fac7ecfac26b5`. It verifies the exact fixture-only
correction, every client's embedded workload bytes and the original C0 auditor.
Conditional sustained eligibility is recomputed from audited parent rows using
the original capacity and derivation rules. The following command audits the
21 retained workflows before the new correction cohort's dispatch:

```sh
python3 results/default-tuning-2026-10-16/analyze.py \
  --manifest results/default-tuning-2026-10-16/manifest-before-startup-corrected-dispatch.json \
  --auditor .tools/default-selection-auditor --output /tmp/weir-confirmation-audit
python3 -m unittest discover -s results/default-tuning-2026-10-16 -p '*_test.py'
```

The auditor build receipt identifies its original source and binary SHA256, built
with Go 1.27.1 for `darwin/arm64` and `CGO_ENABLED=1`. The exact binary gate uses
that recorded platform/build identity; a different-platform rebuild has a
different identity and is not silently substituted into this evidence.
Output directories must be new. Ordinary Go tests remain offline and do not
start services; real database integration is opt-in. These are owned Linux
loopback fixtures, not a distributed production fleet. The finite candidate grid,
database quotas and document distributions bound every claim above.
