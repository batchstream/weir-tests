# Search metadata-only experiment frozen before measurement

This experiment tests whether limiting Search metadata response fields lowers measured database CPU per completed product at comparable completed throughput. Native single-record updates remain a control. The implementation under test changes only the fields returned by the fresh metadata inspection request; it does not change Search batch keys, cross-index grouping, document reads, mutation execution, or visibility semantics. A smaller metadata response alone does not establish lower database CPU or higher completed throughput.

The previously tested cross-index batching candidate `b13318ffcfde662d5f77209ef88d689bdcd2433d` is a different implementation. It regressed completed throughput at the higher tested rates and was rejected. Its complete results remain in [the separate batching experiment](../search-batching-2026-10-15/PLAN.md). This new experiment neither replaces nor reclassifies those negative results.

## Immutable sources and scope

| Component | Source / policy |
| --- | --- |
| Client and workflow | `8bdfe233f4baad325c66718a4cdec1c8c436a41a` in `batchstream/weir-tests` |
| Baseline Weir | `0f6fac3edb462e088decff98b67560dae2c23c9a` in `batchstream/weir` |
| Metadata-only Weir | `745501319ae59a16351294c2eaed58019288c3ad` in `batchstream/weir` |
| Both Weir execution limits | `max_in_flight=0` |
| Native control | One document per cached atomic Painless `_update` |
| Weir input | One document per SDK `Mutate` call |

The client source is the existing clean main commit. Its `cmd`, `internal`, `fixture`, `scripts`, and crawler workflow trees have no diff from the previously measured `ad1879156c14028285f27f18794f1580e979af63`. Receipt identities must nevertheless match the actual checkout `8bdfe2...`. The two Weir source and executable hashes must differ, and their before/after executable hashes must remain unchanged. Analysis rejects a pending candidate SHA, a different baseline/client source, or a different rate matrix. Lua stays in the one Weir process. Business repositories remain read-only, legacy Sink is excluded, and no Lua API is added.

## Workload and controls

Run three separate workflows, one for each offered rate **16, 48, and 96 products/s**. Each workflow has **three rotating rounds**, with native, baseline Weir, and metadata-only Weir each measured once per round. Each stage uses **10 seconds of warmup and 20 seconds of timed arrivals**, followed by full completion drain. These rates and gates will not be adjusted after seeing this experiment's results. Do not pool absolute CPU or throughput across separate workflow runners.

Every path uses the existing `two-step` profile, `single` input shape, page size 1, four OS client processes, 128 total goroutine workers, one queued arrival per worker, a 10 second planned-business deadline, and four maximum Search attempts. Each product performs reconcile, then product update, then separately dispatched parallel seller updates; the existing occasional shared placeholder rule remains unchanged. Client processes and pools persist from warmup through timing. Each successful business product requires four logical mutations plus its existing possible placeholder mutation. Application-call histograms must establish exactly one record per invocation.

Keep the existing fixture controls: one owned database container with a 1 CPU quota, 1536 MiB memory limit, 512 MiB heap, two indices with one primary and zero replicas, 1 second refresh interval, request durability, batch size 32, and 4096 pending records. HTTP observation is explicitly disabled for all paths; unavailable backend HTTP counts and byte measurements remain null. The request path and visibility/durability semantics remain unchanged.

All paths use the same finite seeded working set and final-field oracle as the previous single-record experiment. This is a controlled same-host client model with independent processes, not a measurement of distributed network effects or production key frequencies.

## Exact dispatch

The metadata-only candidate SHA `745501319ae59a16351294c2eaed58019288c3ad` is frozen in this plan and the manifest. Commit the plan and analysis sources before dispatching any measurement. At this preparation stage, no metadata-only run has been dispatched and all three workflow URLs remain null. Record each resulting URL before inspecting its results. If dispatch or any subsequent run fails, retain the attempt in the experiment ledger and preserve every available raw artifact.

Dispatch the existing `crawler.yml` workflow using `--ref main` only after verifying that main still resolves to `8bdfe233f4baad325c66718a4cdec1c8c436a41a`; verify each returned run's `headSha` matches it. GitHub rejects a raw commit SHA as a `workflow_dispatch` ref. For each `RATE` in `16`, `48`, `96`, pass:

```text
current-source=0f6fac3edb462e088decff98b67560dae2c23c9a
optimized-source=745501319ae59a16351294c2eaed58019288c3ad
after-max-in-flight=0
profiles=two-step
input-shape=single
http-observer=disabled
modes=fixed-rate
rates=<RATE>
concurrency-levels=128
page-size=1
duration=20s
warmup=10s
rounds=3
```

Expected timed matrix: 3 workflows × 3 paths × 3 rounds = 27 stages. An aborted or overloaded run retains its original workflow URL and available raw artifact; it is not silently replaced. Any operational retry must have a separately identified ledger entry and cannot erase the original attempt. The formal manifest contains the three planned primary runs; separately document and archive any additional attempt instead of substituting it without disclosure.

## Audits and decision rules

Preserve receipts, fixture manifests, owned process/container cleanup receipts, database logs and their ownership/length receipts, full stage JSON, and compressed/uncompressed SHA-256 plus uncompressed byte length for every raw file. Check immutable sources and executables, live zero execution limits for both Weir paths, the complete rotating stage matrix, process/worker identities and warmup reuse, measured application calls, arrival/completion/failure arithmetic, merged latency histograms, settling samples, full-field verification, and the independent Go offline audit. Rebuild the auditor from the exact clean client source and record its hash. Failed, dropped, rejected, incomplete, and indeterminate results remain visible.

The frozen measurement gate requires the planned source/matrix/ownership/cleanup audits, successful final-field and Go audits, three rounds of 20 seconds after 10 seconds of warmup, zero failed/unknown/timed-out dispatched business operations, and at least 80% clipped cumulative-counter CPU coverage. The fixed-rate all-arrivals gate additionally requires no rejection, offered equals completed, planned-to-visible p99 upper bound ≤ 5 seconds, drain ≤ 3 seconds, and maximum generator lag ≤ 50 ms in **each** round. Histogram quantile upper bounds have the existing ≤ 12.5% plus 1 microsecond approximation. These are experimental gates, not production SLOs.

A lower-database-CPU claim at comparable completed throughput requires both compared paths to pass every all-arrivals gate in all three paired rounds, every completed-throughput ratio in `[0.95, 1.05]`, and every sampled database CPU ms/completed-product ratio below 1. Report all paired ratios and negative results, alongside per-round and completed/time-weighted totals within each workflow. No confidence interval is implied by three rounds.

Configured CPU quota is not actual database load. This fixed-rate experiment may support lower observed database CPU at comparable completed throughput; it does not establish maximum capacity or greater throughput at equal actual database load. The latter claim needs a separate, prospectively specified actual-load comparison. Database CPU includes completion drain and is weighted only over sampled coverage, with no extrapolation to unsampled time. Client invocation counts and adapter batch fill are not backend HTTP command counts. Preserve database get/index/refresh counters as observations without inventing missing HTTP measurements. Lower metadata response byte counts in separate protocol tests are not a substitute for this performance measurement.

## Frozen analysis

`analyze.py`, `summarize.py`, and `plot.py` are independent copies adapted from the prior batching experiment before collecting this candidate's data. Historical result files remain unchanged. Candidate SHA injection is confined to `manifest.json.server_sources.optimized`; analysis accepts no source override and refuses to run until a distinct immutable SHA is frozen. The three planned workflow URLs and raw paths are recorded before their results are inspected. Regenerate into a new output directory with:

```sh
python3 results/search-metadata-2026-10-15/analyze.py \
  --manifest results/search-metadata-2026-10-15/manifest.json \
  --auditor .tools/weir-crawler-search-metadata-audit \
  --output /tmp/weir-search-metadata-analysis
```

Generated analysis records all three frozen analysis-source hashes and the auditor executable hash. Plots label native, baseline Weir cap0, and metadata-filtered Weir cap0 separately and retain every round, including failed gates. No measurements or performance claims are present when this plan is first committed.
