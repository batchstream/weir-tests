# Distributed single-record crawler experiment plan

Recorded on 2026-10-15 before calibration. Scope is testing Weir only: no production Weir, SDK or business repository edits. The previous page-20 experiment remains an already-batched-client control; its result does not characterize independently arriving single-record writes. Sink and new Lua helper APIs are excluded.

## Request shape

Use the explicit single input shape with one product per arrival. Each process and goroutine submits its own record: native uses one atomic Elasticsearch `_update` request; Weir uses one Mutate RPC containing one record. No client-side aggregation window, shared queue across processes or prebuilt page batch is added. Preserve the existing two-step rules: reconcile one product, update that product, then submit its two sellers individually in parallel and wait for all acknowledgements. The existing synthetic placeholder adds a third individual seller request when `(worker + page) % 100 == 0`; report actual counts rather than calling it a production frequency.

Four OS clients each own 32 goroutine workers and retained pools (128 total workers). Native transport permits up to three connections per worker, matching the maximum possible seller fan-out including the placeholder. Actual application attempts and records per request are counted per worker. A successful single-shape stage must prove all requests contain one record, and the offline oracle must reproduce all fields from acknowledged business arrivals. Server Prometheus batch-operation sum/count independently shows whether Weir aggregates separate incoming calls; it is not a database HTTP-request counter.

## Fixed controls

Both Weir paths use c3101c6628872345d6fb09b977ce853f25936b79 and identical binary hashes. The default path uses unlimited adapter executions (cap 0); the other alone uses cap 16. Native atomic Painless is a strong single-record control with the same generated values and rules. All paths use the same owned database within one run, 1 CPU quota, 512 MiB JVM heap, 1536 MiB container memory, two one-primary/zero-replica indices, request durability, one-second refresh, four total attempts, and query-visible acknowledgements. Preserve the 10 s planned-arrival business deadline and one queued arrival per worker. HTTP observer is disabled for every path; absent HTTP measurements stay unavailable. Capture bounded owned backend logs and cleanup receipts.

Arrivals are open-loop, individually staggered from absolute times across processes. Latency includes queueing and all completion/visibility waits. Completed throughput includes the entire subsequent drain. Keys are worker-owned apart from the explicit shared placeholder. page1 reduces the seeded working set relative to the old page20 test: never subtract across those runs and call the difference a pure aggregation gain.

## Calibration and frozen selection rule

Run exactly 4, 16 and 48 offered business products/s, 128 workers, 5 s arrivals, 2 s warmup and one rotating round: native/cap0/cap16, nine stages. Calibration is diagnostic only. Use native and default cap0 to select formal rates, in this order:

- If 4 is overloaded, select 1/2/4.
- Otherwise, if 16 is overloaded, select 2/4/16.
- Otherwise, if 48 is overloaded, retain 4/16/48.
- If all three are stable, select 16/48/96.

For selection, stable means complete offered work without rejection, failure, unknown or timeout, p99 upper bound <=5 s, completion drain <=3 s, and generator lag <=50 ms. cap16 alone failing does not reduce the main rates. An infrastructure, submission-shape or correctness failure requires diagnosing the test and retaining the failed run, not lowering rates to conceal it. Record the chosen rates and immutable client SHA before formal dispatch; do not retune merely to obtain a positive result.

## Formal measurement and interpretation

Use three selected rates, 10 s warmup, 20 s arrivals and three rotating rounds: 27 stages. Retain every round, failure and overload. Require clean immutable source/build/command receipts, complete matrix and cleanup, persisted-state plus offline audit, no failed/unknown/timed-out dispatched work, and >=80% sampled database CPU coverage for formal measurements. The stricter all-arrivals gate additionally requires no rejection, all arrivals complete, p99 <=5 s, drain <=3 s and generator lag <=50 ms in all three rounds. These are experimental limits, not production SLOs.

Compare paired paths within the same run and round. Report CPU sample coverage, CPU ms/completed product, actual completed products/s, p99, drain, failures/rejections, and adapter batch fill. To describe lower database CPU at comparable completed throughput, both paths must meet all-arrivals gates in every round, completed QPS must be within +/-5% in each paired round, and the CPU-per-product direction must be consistent across rounds. Otherwise describe the measured tradeoff without a savings multiplier. Three rounds do not establish a production confidence interval or capacity.

Equal database CPU quota means equal budget, not equal observed CPU load. This experiment can show actual CPU-versus-completion curves; it does not automatically prove higher throughput at equal observed database load. Native Painless runs merge logic in the database while Weir runs Lua outside it; total infrastructure CPU is deliberately outside the user's requested metric. Host/JIT/background variation, persistent segment history and bounded run duration remain limitations.
