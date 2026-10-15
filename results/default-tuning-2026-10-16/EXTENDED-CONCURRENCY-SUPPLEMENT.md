# Prospective extended concurrency and fixed preheating study

Registered after the corrected one-eighth-CPU cohort completed. The 22-workflow
analysis and its zero-soak decision are retained in commit
`891d2c5`. Its 567 rows include all previous 549 rows unchanged and 18 new usable
stages. None demonstrates a Weir maximum: 512-to-1024 throughput gains exceed
10% in two rounds, and the third 1024-worker round fails half-window stability.
This study does not erase, retry or reclassify any of those failures.

At 1024 workers, sampled active Weir executions average 7.44–7.71 and reach at
most nine of the 16 slots. Sampled queued operations reach 185 of the Store's
1024-operation queue. Attempted batches average about 124–125 of 128 records,
only 147–151 KiB against the 4 MiB byte bound. This supports testing the missing
higher concurrency rather than changing the selected batching defaults. These
sampled gauges do not prove that no unsampled peak occurred. The first
256-worker round also has strong cold-state effects; later variability cannot
be retrospectively excused as startup.

One final finite case set, `confirm-pure-extended-q0125`, contains exactly one
case, `default-pure-extended-q0125`:

- Search, fixed observed quota 0.125 CPU, one primary, zero replicas,
  request translog durability and one-second refresh;
- paired native single-record calls and omitted Weir defaults; four independent
  OS clients and unchanged alternating AB/BA order;
- total concurrency **1024, 2048**, three rounds at each level;
- **300 seconds of fixed warmup and 180 seconds timed** per path, every round;
- 8192 records, 1024 payload bytes, 100% writes, no Lua mutations;
- unchanged Weir source `084572d79b7a53230801297f90ca2363fd864913`, with effective
  128 operations / 4194304 bytes / 20ms / 16 in flight.

This declares 12 timed stages and 96 minutes of warmup plus measurement,
excluding bounded preparation/postflight. The existing 100-minute plan limit
and 120-minute workflow limit remain. Native latency or timeouts at 2048 are
possible; they remain failures, with no timeout extension or error exemption.
No point is assembled from different workflows, quotas or client revisions.

The client source extension only admits a total of 2048 workers and five-minute
warmup in the existing single-request engine. Per-child threads remain bounded
at 1024; this case uses 256 or 512 threads per child. Dataset/CLI/client-plan
admission checks and their tests may change, but measurement loops, workload
scripts, native/Weir operations, timeouts, configuration, verification,
resource observation and cleanup must remain byte-identical to the preparation
source `2220c18eae4cbe5d079753155638f44cc7dc649b`. No path-selection feature,
queue increase or new production option is needed. The exact changed source
files and immutable Go tree must be independently reviewed before dispatch.

Every original numerical capacity gate remains: zero failures/unknowns/timeouts,
complete source and persisted oracle, p99 histogram upper bound at most five
seconds, CPU observation and saturation, host/client headroom, second-half
throughput at least 90% of first-half throughput, and at most 10% gain at the
next concurrency. Both adjacent points must qualify in all three rounds. Only
1024 has a following point; 2048 cannot itself be called a proved maximum.
All 180 timed seconds count. There is no adaptive preheating, trimmed burn-in,
favorable retry, new quota, extended ladder or relaxed threshold after dispatch.

If and only if 1024 qualifies, one paired pure check may be derived under the
original five-minute rules: same source, quota, workload, four clients and 1024
workers, one round, 60s warmup and 300s timed. Each minute must retain the
registered throughput, p99, CPU and integrity gates. The original one-pure,
three-total check budget and failed-soak no-retry rule remain unchanged; earlier
ineligible cases do not acquire eligibility. The parent must be selected from
audited rows, never copied qualification booleans. The plan, source and hashes
must be frozen and independently reviewed before measurement.

A passing adjacent pair plus sustained check supports a **scoped CPU-saturated
throughput plateau**, not a universal mathematical maximum or production fleet
limit. If this last finite study fails, publish the failed gates and report that
the tested environment did not establish a stable maximum; do not call an
observed peak maximum capacity.
