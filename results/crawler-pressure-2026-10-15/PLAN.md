# Crawler database pressure experiment

Recorded on 2026-10-15 before dispatch. This is a separate experiment from the 128-worker batching and metadata-only measurements. It tests whether a larger number of independently arriving single-record clients can demonstrate sustained database CPU pressure without changing the workload, durability, visibility or production configuration. High worker counts, a CPU quota, rejected arrivals and a brief CPU peak are not database bottleneck evidence. All unsuccessful and unsaturated attempts remain in the experiment ledger.

## Sources and scope

| Component | Immutable source / policy |
| --- | --- |
| Client, workflow and offline auditor | `488b4575932265030b81312319b707e45649a028` in `batchstream/weir-tests` |
| Baseline Weir | `0f6fac3edb462e088decff98b67560dae2c23c9a` in `batchstream/weir` |
| Metadata-only candidate Weir | `745501319ae59a16351294c2eaed58019288c3ad` in `batchstream/weir` |
| Native control | One document per cached atomic Painless `_update` |
| Weir input and execution limit | One document per SDK `Mutate`, `max_in_flight=0` on both servers |

The client commit changes only benchmark concurrency guards, their tests, and CLI/workflow documentation. It raises the total bound from 256 to 2048 and the child-process bound from 128 to 512, and rejects parent parameters that would exceed the child bound. Default concurrency stays 128. Native connection pools already scale to three connections per worker and are not reduced. No business, Weir, SDK, protocol or deployment repository is modified for this experiment. The existing pinned SDK remains `weir-go v0.15.0`; its calls open independent Execute streams through each retained process connection with no automatic write replay. Lua remains inside the single Weir process. Sink and new Lua helpers are excluded.

Root creates the immutable workflow branch `randy/crawler-pressure-client` at the exact client commit above, without moving it after dispatch. This plan and the manifest are a subsequent commit, so neither attempts to contain its own commit hash. Dispatch must verify that the branch still resolves to the frozen client SHA. Each returned GitHub run `headSha`, command receipt `source`, server receipt source and binary hash, and unchanged before/after binary hashes must match the manifest. A pending source, a dirty source, a moved branch, a receipt mismatch or an unexpected parameter invalidates that attempt; keep the failed attempt rather than substituting an unrecorded rerun. Historical 128-worker runs remain frozen at their original client source.

## Resource and workload controls

Use `single`, `two-step`, page size 1, observer disabled, fixed-rate arrivals, four OS clients, one queued arrival per worker, a 10-second planned-arrival business deadline, and four total Search attempts. Preserve reconcile acknowledgement, then product acknowledgement, then the separately dispatched parallel sellers and join. Each successful business product has four logical mutations plus the existing occasional shared unchanged seller. The request histogram must contain exactly one record per native update or SDK call; the application ledger is not a database HTTP-request counter. Disabled HTTP counters remain unavailable.

Every stage uses one exclusively owned Elasticsearch database with a verified 1 CPU quota, 1536 MiB container memory, 512 MiB JVM heap, two one-primary/zero-replica indices, request translog durability and a 1-second refresh interval. Weir batch size remains 32, Store queue remains 1024 records / 32 MiB, and transport pending records remain 4096. No memory, queue, connection, retry, deadline, visibility or overload safeguard is raised for the experiment. Record all server overloads, native connection failures, circuit breakers, timeouts, failed settling, OOMs, generator lag, unknown outcomes and final-field failures, with owned logs and cleanup receipts.

At 512 total workers each OS process has 128 workers and a native pool ceiling of 384. At 2048 total workers each has 512 workers and a ceiling of 1536. These are ceilings rather than measured active connections; no allocation guarantee is inferred from them. Each worker still has at most one active business product and one queued product. The seeded data set is `workers * 6 + 1`: 3073 documents at 512 workers and 12289 at 2048. Reset requests remain bounded to 8 MiB including action metadata, and final query verification uses batches of at most 100 IDs. Every seeded key, including untouched or rejected keys, is checked with the full-field oracle. Worker IDs and process offsets must cover the declared worker count exactly. Source-level and offline tests verify the new bounds, child readiness/pool receipts, and allocation of all 2048 distinct worker results without external services.

The larger worker count also enlarges the finite seeded working set. Do not treat a difference between the two worker runs as a pure concurrency gain. Both are same-host models with independent processes, rather than a measurement of distributed network effects or a production key distribution. Client and database resource samples are retained, including host, process memory and database CPU. A native frontend or worker bottleneck remains possible even at the larger bound.

## Short calibration fixed before results

Dispatch exactly two separate workflows, one with **512 total workers** and one with **2048 total workers**. Each runs **96, 384 and 768 offered products/s**, **2 seconds warmup**, **5 seconds timed arrivals**, **one rotating round**, and the same three native/baseline/candidate paths. This is 18 planned timed stages in total. Fixed-rate mode uses only the largest concurrency entry, so passing `512,2048` to a single fixed-rate workflow would not run both levels and is forbidden for this matrix.

For each worker level pass these inputs to `crawler.yml` using `--ref randy/crawler-pressure-client`:

```text
current-source=0f6fac3edb462e088decff98b67560dae2c23c9a
optimized-source=745501319ae59a16351294c2eaed58019288c3ad
after-max-in-flight=0
profiles=two-step
input-shape=single
http-observer=disabled
modes=fixed-rate
rates=96,384,768
concurrency-levels=<512 or 2048, one value per workflow>
page-size=1
duration=5s
warmup=2s
rounds=1
```

Record each dispatch attempt, workflow URL, run ID, returned source SHA and dispatch time in the ledger before examining its result. Archive every available report, receipt, owned manifest, database log and cleanup file, with compressed and uncompressed SHA-256 plus uncompressed length. Keep incomplete matrices and failed attempts. Any operational retry is an additional identified attempt; it cannot erase an earlier run or replace the primary calibration silently.

## Actual-pressure gate and prospective formal selection

Use the existing `Resources.sustained_cpu_saturation` definition without lowering its threshold: at least **5 valid cumulative-counter intervals**, clipped CPU coverage **>=80%**, sampled mean database CPU **>=90% of its verified quota**, and time in intervals at **>=90% CPU covering >=80% of the entire measurement**, including completion drain. CPU is time-weighted over valid sampled intervals without extrapolating missing intervals. A maximum or an average alone does not satisfy this gate. Five-second calibration is deliberately short and may fail this strict gate even under a burst; it is diagnostic, never capacity evidence.

Select formal settings using **native stages only**, without consulting whether either Weir path looks better. A native selection-eligible stage must have:

- The frozen clean client/server sources, binaries, command, worker level, rate and resource budget, complete stage and client identities, retained warmup pools/PIDs, both successful settling waits, owned diagnostics and completed cleanup.
- Successful full-field verification of all `workers * 6 + 1` documents; independent reconstruction of the expected hash from acknowledged pages; valid arrival/worker/child/stage arithmetic and merged latency/call histograms, with exactly one record per call and the declared native pool. If another path or a later stage invalidates the whole workflow, flag the workflow as unqualified while retaining any independently complete earlier native stage for diagnostic selection only. A failing whole-report Go audit must stay visible; it cannot be described as passing.
- Zero native failed, unknown or timed-out dispatched business products, maximum generator lag **<=50 ms**, and the existing **sustained CPU gate true**. Rejected arrivals are retained and allowed for diagnostic selection; they prohibit a sustainable-all-arrivals claim.

Choose the **lowest worker level** with at least one eligible native stage: 512 first, otherwise 2048. At that level choose the eligible native stage with the **highest completed products/s including all drain**; ties choose the lower offered rate. Let `Q = floor(completed products/s)` for that stage. Freeze the formal offered rates as the ascending unique values of **`max(1, floor(0.8*Q))`, `max(1, Q)`, `max(1, floor(1.2*Q))`**. Record the selected native run/stage, measured completed QPS, integer Q, worker level and resulting rates before formal dispatch. If rounding deduplicates rates, retain the deduplicated matrix rather than inventing replacements. This deliberately conservative short-run Q only chooses formal points; it is not a sustainable rate estimate.

If neither worker level contains an eligible sustained-CPU native stage, stop this experiment at diagnostic evidence and explicitly report that it does not establish a database CPU bottleneck or Weir capacity benefit. Do not lower the CPU threshold, change completion semantics, expand resource limits, select from relative Weir gains or keep escalating offered rates to obtain a positive result. Infrastructure failures can be investigated separately, with their original attempts retained.

## Formal verification and claims

For the selected worker level and each selected rate, run a **separate workflow** with **10 seconds warmup, 20 seconds timed arrivals and three rotating rounds**; each workflow has native/baseline/candidate in the same database and runner. All other inputs, sources and limits remain frozen. Different formal rates can use different runners, so compare paths only within the same workflow and round; do not pool absolute CPU or throughput across runners into a capacity curve. Each formal matrix must pass the complete source/ownership/cleanup audit, full-field oracle and the original whole-report Go offline audit, and retain every negative row. A short calibration's successful selection does not waive formal correctness or pressure checks.

The formal measurement gate additionally requires zero failed/unknown/timed-out dispatched work, complete 20-second/three-round stages after 10-second warmup, and >=80% clipped CPU coverage. The all-arrivals gate additionally requires no rejected arrivals, offered equals completed, planned-to-visible p99 upper bound <=5 seconds, drain <=3 seconds and generator lag <=50 ms in every round. These are experimental limits, not production SLOs. Quantile upper bounds retain the existing <=12.5% plus 1 microsecond approximation.

A lower-database-CPU claim at comparable completed throughput requires both compared paths to pass all-arrivals gates in all three paired rounds, each completed-QPS ratio in `[0.95, 1.05]`, and database CPU ms/completed product lower in every paired round. To call it evidence specifically under demonstrated high database CPU pressure, the native path must also satisfy the unchanged sustained-CPU gate in **every** formal round. Otherwise describe the observed CPU/completion tradeoff and explicitly state that the database bottleneck was not established. Preserve all paired values, CPU coverage, latency, drain, rejections, failures, actual index/get/refresh work and adapter batch fill. A lower HTTP count or smaller metadata response is not itself a CPU result.

This matrix is designed to test lower actual database CPU at comparable completed throughput. A shared CPU quota is only a budget, and neither rejected arrivals nor completed QPS from unsaturated stages proves higher throughput at equal actual database load. No maximum-capacity, confidence-interval or production-throughput claim follows automatically from these runs. Additional actual-load comparisons would need a separate prospective design; no such extra matrix is authorized by this plan.
