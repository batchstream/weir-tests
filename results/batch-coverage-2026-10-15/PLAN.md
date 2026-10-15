# Broader batching coverage and independent confirmation

This plan is prospective. Server and client immutable identities and dispatch IDs
must be committed here before inspecting any run results. Only Weir and its test
repository change. No business repository, SDK, protocol, deployment or Sink path
changes. Lua remains in the single Weir process.

## Candidate and measurement boundaries

Server source: `6c40957ec7352c867bc92b9b032db0d754956853` in `batchstream/weir`.
The client/workflow source identity will be frozen in a follow-up source ledger
commit before dispatch, because this file cannot contain its own commit hash.

The fixed candidate is 128 operations, 8 MiB aggregation, 5 ms collection and 16
simultaneous adapter executions. It is a test hypothesis, not a selected universal
default. The same server binary uses 32 operations, 8 MiB, zero wait and unlimited
executions on the immediate baseline path. Query visibility, durability, deadlines,
transport admission, retry budgets and seeded field oracles remain equal within
each comparison. Actual framed bulk bytes and record histograms are retained.

No source or setting may change after a run is dispatched. A correctness or
instrumentation failure is preserved and may require a new explicitly identified
attempt; a negative performance result is not rerun until positive. Diagnostics
are short controls, not formal proof. Extra Weir CPU is not an adoption gate.

## Finite experiment inventory

1. One pure-write calibration workflow, six explicit configurations, each with
   128 and 512 total workers in four OS clients. Each point alternates native
   single-record index calls and Weir single-record SDK calls, one round, 2 s
   warmup and 5 s timed. Configurations are immediate 32/0ms/unlimited; bounded
   32/5ms/16, 128/5ms/16 and 256/5ms/16; 128/20ms/16; 128/5ms/4. All use an 8 MiB
   byte target and 1 KiB payload. Count, wait and concurrency comparisons are
   against the 128/5ms/16 anchor. These short points expose sensitivity; they do
   not choose formal controls after seeing relative performance.
2. Two diagnostic crawler workflows, three paths each, one round, 2 s warmup and
   5 s timed, observer enabled. Partial-update uses 16 KiB seed padding,
   application acknowledgement and 512 offered operations/s. Two-step crawler
   uses query-visible completion, no extra padding and 68 products/s. Both use
   512 workers across four OS clients. The candidate and baseline are as above.
3. One pure-write formal workflow: the fixed candidate versus native single
   writes, two concurrency levels 128 and 512, three alternating paired rounds,
   60 s warmup and 60 s timed. Twelve measured stages. Application acknowledgement
   and 1 KiB payload remain unchanged. Completion latency and drains remain in
   measurement. There is no open-loop offered-arrival or equal-throughput claim
   from this closed-loop workload.
4. Two crawler formal workflows: diagnostic profiles/rates/controls unchanged,
   observer disabled, three rotating rounds, 60 s warmup and 60 s timed. Each has
   nine measured stages. Generation remains absolute-time, queue 1 per worker,
   10 s end-to-end deadline and 512 total workers. Data reconstruction occurs
   outside measurement; no qualification threshold is relaxed.

Exactly six workflows are planned. Formal runs require successful correctness and
usable metrics in diagnostics. Performance rejections/lag remain negative rows,
not reasons to search for another rate or change defaults. No capacity curve or
production-network simulation is part of this plan.

## Separate correctness and scheduler coverage

Owned real Elasticsearch 8.19.22 and OpenSearch 3.3 fixtures exercise four gRPC
connections, four execution slots and 20 ms collection with eight indexes (80%
hot-index traffic), 1/256 KiB sources, and 10% shared-key increments. Every
APPLIED outcome is checked against persisted counters, native versions and all
untouched fields. Known hot-key conflicts remain accounted NOT_APPLIED outcomes.
Observed over-target NDJSON bodies must contain exactly one legal large item.

Offline scheduler tests cover sparse and burst traces, byte/count/time dispatch,
cross-group session/resource ordering, slow-adapter execution slots, cancellation,
queue backpressure and no second collection wait after slot waiting. A same-host
selector microbenchmark covers 1/8/64/1024 compatibility groups with 1024 queued
operations. These tests do not establish database throughput improvements.

## Interpretation gates

Every formal stage needs clean immutable source/binary receipts, successful
owned cleanup, complete report/auditor, full seeded-field verification, zero
failed/unknown/timed-out work and at least 80% clipped database CPU coverage.
Crawler all-arrivals qualification additionally requires no rejection,
offered=completed, planned-to-visible p99 upper bound <=5 s, drain <=3 s and
maximum generator lag <=50 ms. Native and Weir must both qualify in every paired
round for a strict three-round claim.

Lower database CPU at comparable crawler throughput requires each completed-QPS
ratio in [0.95,1.05] and lower CPU ms/completed unit in every paired round. Pure
write throughput and CPU/unit are reported together; a faster path at greater
actual CPU is not an equal-load win. Capacity requires an independently
demonstrated database bottleneck and plateau: at least five valid CPU intervals,
80% coverage, mean >=90% quota and >=90% CPU during >=80% of full elapsed time,
plus no more than 10% throughput gain at the next concurrency in every round.
Equal quotas alone do not demonstrate equal actual load. No rate interpolation or
equal-load capacity claim will be manufactured when those conditions do not hold.

Inspect within-stage CPU samples for drift and show all three paired rounds;
60-second stages do not prove steady state. Preserve source, inputs, all raw
reports, command/config/metrics receipts, original auditor output, checksums and
every negative result. Do not combine absolute CPU or throughput between runners,
between pure writes and partial operations, or between those units and products.
