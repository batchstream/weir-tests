# Sustained Weir value benchmark

This experiment asks whether Weir improves successful business throughput,
latency or resource demand over a strong direct backend client for the same
work. Fewer backend commands alone do not establish application value.

## Frozen implementation

- Server: `8955cde10e574dae55d890b0c9c824fd39219d1c`.
- Measured client: `99d7951bc935c811bc155a40e390ef7a200e3fa5`.
- Go `1.27.1`, SDK `v0.15.0`, protocol `v0.13.0`.
- MongoDB `8.0.32`, Elasticsearch `8.19.22`; immutable image digests and module
  checksums are retained in every execution receipt.
- Later Native integration-test fixes do not change the measured client code.

Each Linux GitHub Actions runner owns its databases, one Weir process and its
client processes. Docker database CPU quotas are inspected, not merely requested.
Both compared paths use the same owned database and durability, visibility and
request-timeout policies. CPU model, kernel, host memory, build identities,
commands and cleanup records are retained. No production endpoint is used.

## Planned matrix

| Role | Work | Database CPUs | Total workers | Adapter grouping limit |
| --- | --- | ---: | --- | --- |
| Primary | Read, 90/10 read/write, write | 1 | 4, 8, 32, 128, 256 | 32 |
| Native transform control | Fixed `revision = 1 - revision` | 1 | 4, 8, 32, 128, 256 | 32 |
| Aggregation ablation | Read, 90/10 read/write, write | 1 | 32, 128 | 1, then 32 on the same runner |
| Native bulk control | Read and write, 32 records per call | 1 | 4, 16, 64 | 32 |
| CPU sensitivity | Read, 90/10 read/write, write | 2 | 32, 128, 256 | 32 |

Every row runs against each backend. Each path and concurrency receives a
10-second warmup and 20-second timed closed-loop phase, repeated in three paired
rounds. AB/BA path order alternates with level and round. In-flight completion
and result validation are included in elapsed time. Initialization, warmup and
independent persisted-state verification are outside timing.

The dataset is a hot working set of 8192 records, each with 1024 bytes of padding.
Worker-owned disjoint keys avoid planned write conflicts. The single-record
experiments use four independent OS client processes and one record per call;
each worker waits for its result before sending another call. Bulk experiments
use one client process with native bulk operations or SDK calls containing 32
records. Bulk latency is call completion latency, assigned to its records.

The fixed transform uses one native MongoDB atomic update pipeline or one native
Elasticsearch update script. Weir performs the same state change through Lua.
The native control does not perform an unnecessary client read or transaction.
This comparison concerns the fixed computation, not arbitrary Lua programs.

## Qualification and interpretation

All scheduled cases and all rounds remain in the record. Any failed operation,
unknown mutation outcome, timeout, missing round, failed final verification or
failed owned-resource cleanup invalidates that comparison. Incomplete results
must be reported as failures, not silently removed or replaced by a favorable run.
The analysis recomputes rates and single-request histogram quantiles from raw
evidence and verifies source, build identities, parameters and paired rounds.

Report total successful records divided by total measured time, every round's
rate and its range. Percentiles are inclusive logarithmic-bucket upper bounds,
with up to 12.5% plus 1 microsecond approximation. Small percentile differences
do not establish an improvement. Bulk quantiles are retained per round because
the bulk report does not retain a raw histogram suitable for pooling.

A clear observed throughput improvement requires more than 10% at the same
concurrency in every paired round; this is a declared interpretation threshold,
not a statistical confidence interval or a production acceptance requirement.
Show the corresponding tail latency and failure counts even when throughput
improves. Smaller differences remain observations rather than a strong win.

Database-capacity claims require each path to independently meet the harness's
declared conditions: at least five valid CPU intervals, at least 80% timed
coverage, mean database CPU at least 90% of quota, at least 80% of the measurement
at that CPU threshold in every round, and no more than 10% throughput gain at
the next concurrency. Otherwise report observed throughput within this ladder.
The highest concurrency has no measured successor and cannot prove a plateau.

The aggregation ablation keeps limits 1 and 32 on the same runner and retains a
direct comparison at each limit. Check native baseline drift before attributing
a difference to aggregation. A drift over 10% makes small causal conclusions
unreliable. CPU sensitivity uses separate runners: compare each run's paired
native/Weir results and hardware identity, rather than treating differences
between runners as a controlled CPU-only intervention.

MongoDB command counts cover the selected `find`, `update`, `bulkWrite`,
`getMore`, `killCursors`, `commitTransaction` and `abortTransaction` counters,
sampled around timed work. They exclude handshakes and monitoring commands.
Missing/reset counters remain unavailable. Elasticsearch node statistics do not
provide equivalent physical client HTTP-command counts.

Weir CPU per successful record comes from its Prometheus process-CPU counter
window. Client CPU is a `ps` sampling estimate; Linux cumulative CPU has coarse
resolution, so it cannot substantiate small total-cost savings. Database CPU
uses cumulative Docker Engine counters and inspected quota. Memory measurements
are sampled process RSS or database container usage, not hard peak bounds.
Network and storage bytes do not establish utilization or monetary cost.

These measurements concern one hot namespace, loopback networking, one Store
owner and the recorded runner. They do not establish production cost, uncached
or disk-bound capacity, cross-host tail latency, conflict-heavy behavior or the
gain with fragmented namespaces and multiple owners. A negative result is a
valid outcome: a service needs measured benefits to justify its extra process
and request path.
