# Measured batching defaults and capacity

This prospective study continues the explicit-default experiment without changing
its retained results. Only Weir and weir-tests change. Native clients submit one
record per call; Sink and business repositories are excluded. Lua stays inside
one Weir process. Additional Weir CPU is not an adoption gate, but host CPU and
client headroom must be observed to attribute a database capacity limit.

The goal is a measured default within the declared workload/configuration set,
and repeatable capacity bounds for the declared topology and SLO. This is not a
claim that one constant is optimal for every database, document or cluster.

## Frozen phases

1. Selection: exactly six workflows, one case-set each in the committed plan.
   Sources and dispatch receipts must be committed before inspecting results.
   No selection observation is reused as independent confirmation.
2. Choose controls using the algorithm below; commit the complete ranking,
   rejected points and selected controls. Apply them as draft production defaults.
3. Independent confirmation: native single-record calls versus actual omitted
   batching defaults. Freeze the new server/client/auditor identities and every
   confirmation case before dispatch. No retuning from a negative confirmation.
4. A five-minute sustained check at each independently qualified capacity lower
   bound; the derivation and maximum number of checks are fixed below.
5. New independent agents review implementation and evidence after PR creation.
   Merge only after required CI, correctness and evidence checks pass.

## Selection controls and inventory

The anchor is 128 operations / 8 MiB / 5 ms / 16 adapter invocations.
The twenty candidate/control configurations are fixed before measurement:

- Count 32, 64, 128, 256, 512 at anchor wait/slots.
- Wait 0, 1, 2, 5, 10, 20 ms at anchor count/slots.
- Slots 1, 4, 8, 16, 32, 64 at anchor count/wait.
- Four interactions: count/wait-ms/slots = 64/1/4, 256/1/32,
  64/10/32 and 256/10/4.
- Legacy control 32 / 8 MiB / 0 ms / unlimited.

Deduplicating the anchor leaves exactly twenty configurations. Order is shuffled
once with seed 20261016, recorded in the plan and never changed from outcomes.
A new owned database is created for each configuration; paired path order is
retained by the existing engine. Selection stages are sensitivity measurements,
not steady-state or capacity proof.

| Case-set | Workload | Client workers | Arrival rate | Warmup / timed / rounds |
| --- | --- | ---: | ---: | --- |
| select-pure | 1 KiB indexed documents, application acknowledgement | 512 | Closed loop | 10s / 20s / 1 |
| select-partial | 16 KiB seeded documents, small partial update, application acknowledgement | 512 | 512 updates/s | 10s / 20s / 1 |
| select-crawler | Two-step product/seller Lua, query visible | 512 | 68 products/s | 10s / 20s / 1 |
| select-search-read | Search pure reads, 1 KiB documents | 4, 128 | Closed loop | 5s / 10s / 1 |
| select-mongo-read | Mongo pure reads, 1 KiB documents | 4, 128 | Closed loop | 5s / 10s / 1 |
| select-bytes | 64 KiB full-document writes, 2048 seeded records | 512 | Closed loop | 10s / 20s / 1 |

All use four independent OS clients and one observed database CPU. Small-document
sets retain 8192 seeded records; crawler retains its full seeded domain oracle.
The byte set holds anchor count/wait/slots and tests 256 KiB, 1 MiB, 4 MiB and
8 MiB. These targets coincide with retained HTTP histogram bounds; every legal
single entry is below the smallest target, so the corresponding bucket count
must equal the bulk attempt count. Actual request sizes and whether the target
binds remain explicit. This set chooses bytes, not count/wait/slots.

Pure Search indexes explicitly use one primary, zero replicas, refresh_interval
1s and translog.durability request. Actual settings are retained. Crawler keeps
matching durability and its declared visibility; no refresh/durability relaxation
is used to manufacture throughput. Selection is 288 timed stages: three write
sets * 20 * 2, two read sets * 20 * 2 levels * 2, plus 4 byte cases * 2.

## Qualification and deterministic selection

Do not erase any failed, rejected, timed-out or unexecuted point. A workflow
failure is preserved; the finite planned cases continue when safely possible.
Missing/reset counters are unavailable, never zero-filled.

A usable candidate point requires immutable clean source/binary/config receipts,
owned cleanup, every seeded-field check, zero failed/unknown/timed-out work,
positive completed work, CPU coverage >=80%, and no source/config mismatch.
The short selection coverage floor retains the prior calibration convention;
confirmation raises it to 95% and never extrapolates missing CPU intervals.
Crawler additionally requires no rejection, offered=completed, p99 histogram
upper bound <=5s, drain <=3s and maximum absolute generator lag <=50ms. Native
control failures remain visible. A failed/unknown/timed-out operation or incomplete
matrix fails the original whole-case audit: all paths in that case are ineligible,
including otherwise successful candidate paths. Their raw absolute measurements
remain retained. A fully audited rejection is a valid overload observation, but
does not qualify that stage for selection or an all-arrivals performance claim.

For count/wait/slot selection:

1. Require usable candidate measurements in all three write profiles. An unlimited
   execution budget is a legacy control, not an eligible protective default.
2. Require both Search and Mongo read points at both worker levels to be usable.
   At each level, normalize candidate/native QPS by the corresponding legacy/native
   QPS. Require normalized QPS >=0.90. Read p99 must be no greater than the larger
   of legacy p99 *1.25 and legacy p99 +2ms. Controls are measured, not assumed.
3. For each write profile, normalize candidate completed QPS by the maximum among
   eligible candidates in that profile. Maximize the minimum of those three
   normalized throughputs. This prevents a single profile choosing the default.
4. Retain candidates within 5% of that best minimum score. Among them minimize
   the geometric mean of three database CPU-ms/completed-unit values, each
   normalized to the lowest usable value in that profile.
5. Treat CPU scores within 5% of the best as a plateau; choose shorter wait,
   then smaller count, then smaller positive execution budget, then stable ID.

For bytes: retain usable candidates within 5% of maximum completed QPS and within
5% of minimum CPU-ms/write. Choose the smallest target in this joint plateau.
If that intersection is empty, maximize the minimum of normalized throughput
and normalized CPU efficiency, with a 5% tie band preferring smaller bytes.
An unbound target is not evidence that a larger target helps.

Publish all intermediate eligibility/score calculations. If no new setting meets
these predeclared constraints, report the failed constraints; do not change the
thresholds or call a failing setting best practice. Diagnose a genuine source or
instrumentation failure transparently before any explicitly identified new study.

## Confirmation and capacity inventory

After selection, actual omitted defaults must match the selected four controls
in raw routes/effective configuration and runtime metrics. Explicit legacy zeros
must still mean zero wait/unlimited; the test fixture may not omit them.

The finite independent confirmation inventory is:

- Pure writes, selected defaults versus native: concurrency 128/512/1024,
  database quota 0.25 CPU and separately 1 CPU, three alternating rounds,
  60s warmup +60s timed. Eighteen timed stages per quota.
- Partial updates at 0.25 CPU: offered rates 128/256/512/1024/2048/4096 updates/s, 512 workers,
  three rotating paired rounds, 60s warmup +60s arrivals.
- Query-visible crawler at 0.25 CPU: offered rates 34/68/136/272/544 products/s,
  512 workers, three rotating paired rounds, 60s warmup +60s arrivals.
- Partial512 and crawler68 at 1 CPU: matching previous representative loads,
  three paired rounds, 60s warmup +60s arrivals.
- Search and Mongo pure reads: actual defaults and explicit legacy controls,
  native single-record comparison, workers 4/128, three alternating rounds,
  30s warmup +30s timed. The same normalized throughput/p99 read guard must pass
  independently after selection.
- Mongo ordinary write, 90/10 mixed and Lua mutation: actual defaults versus
  native and explicit legacy controls, 128 workers, three alternating rounds,
  30s warmup +30s timed; retain full-field oracle and actual resource load.

The 0.25 quota is declared before any new data because old evidence suggests
shared-host/service CPU may bind before a one-CPU database. It deliberately makes
a database bottleneck reachable while preserving host headroom; it is not a
claim about one-CPU or production capacity. The one-CPU confirmations are reported
separately, with actual CPU rather than quota used in load comparisons.

All confirmation measurement gates additionally require >=95% CPU coverage.
Crawler retains its all-arrivals/SLO gates. Report both halves and all CPU
intervals; 60 seconds alone does not establish steady state.

Database CPU capacity requires, in every one of three rounds: usable measurements,
mean >=90% of actual quota, >=90% quota CPU during >=80% of full elapsed time,
at least five valid intervals, and <=10% throughput gain at the next declared
concurrency. Both adjacent concurrency points must meet the CPU and headroom
gates. Host samples must cover >=90% of elapsed time, mean host busy <=85%,
mean steal <=5%, and each client process CPU must stay below 80% of the effective
host CPU capacity. A full host or Weir/client bottleneck must not be relabeled
as database saturation. Preserve node queues, rejections, throttling, GC,
indexing pressure, Store queue/active work and CPU/IO pressure for diagnosis.
Whole-host busy is accepted for this headroom claim only when runtime CPU count,
sampler affinity and /proc/stat listed CPU count agree. Every known owned client
PID must have >=90% CPU observation coverage; a missing process is unavailable.

Crawler sustainable-rate bounds use the highest rate where all three rounds
meet whole-stage all-arrivals gates, and the next predeclared failing rate as an
upper bound. An upper bound caused by generator lag/host pressure is not a
service overload bound. If no higher failing point exists, report a lower bound,
not an invented maximum. If no point qualifies, report that outcome.

Completed work and latency are retained in fixed ten-second completion-time
buckets; offered/rejected work is assigned by scheduled arrival time, with a
separate open drain bucket. No gate demands equal offered/completed counts in
each sub-window: normal in-flight work crosses those boundaries. For capacity,
second-half completion rate must be at least 90% of first-half rate. For crawler,
using cumulative offered-completed-failed-rejected across regular buckets, the
last regular bucket backlog must be no greater than the first-third backlog plus
one second of the offered rate. Whole-stage accounting and SLO remain strict.
The five-minute check applies the same rate-stability rule to every consecutive
minute after the first minute; per-minute p99 upper bounds must also be <=5s.
The final minute must also retain at least 90% of the first minute's completion
rate, so repeated small decreases cannot conceal sustained degradation.
Any failed/rejected/unknown work, invalid bucket offset or mismatched bucket sum
invalidates that capacity check. Finite sampling can miss brief queue spikes;
zero sampled queue is never interpreted as proof that no queue existed.

Five-minute checks evaluate selected defaults, 60s warmup +300s timed (the pure
write engine also retains its paired native control; crawler uses optimized only):
for pure writes, the smallest concurrency satisfying the database CPU/plateau
rule; for each crawler profile, the highest three-round qualified rate with a
valid service-overload upper bound. At most three checks, one per workload.
No check is dispatched without the specified qualifying lower bound. All work,
latency, CPU and queue observations from every minute remain visible. A failed
sustained check invalidates the corresponding capacity claim rather than being
rerun until favorable. Unknown writes are never automatically replayed.

## Limits

These are single-runner owned database fixtures over loopback, not a production
fleet or a universal optimization proof. CPU and byte defaults do not infer the
capacity of a remote database from Weir's local core count. Existing explicit
controls remain public and visible in effective configuration. No adaptive pool,
extra queue, external coordination or subprocess Lua design is introduced.
