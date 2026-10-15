# Crawler experiment plan

Planned on 2026-10-15 before the first Linux calibration result, with scope clarified before any successful calibration dispatch. The measured client is frozen at e391cb59a7c7c80775d4c987f6bd598183a68cc4, current server at 4fe287e5915766ec3f48c8f42abde6b884d89bdd, optimized server at c3101c6628872345d6fb09b977ce853f25936b79. Sink is excluded. The task is testing Weir only; business repositories are read-only references.

First calibrate the existing two-step rules/max_in_flight=0 with 32 and 128 workers, four client processes, 20 products/page, one rotating round, 2 s warmup and 5 s arrivals. This short experiment selects three offered rates below, near and above the observed service capacity; it provides no formal performance claim. Record the chosen rates before dispatching the formal run.

The primary formal experiment uses the same existing two-step profile, unlimited adapter executions, 32/128 workers, three rotating rounds, 10 s warmup and 20 s arrivals, one inspected database CPU, two indices and query-visible completion. No round or unfavorable path is discarded. Every document is checked against an independent domain oracle.

An error-free formal measurement requires the complete matrix and cleanup, matching immutable receipts, persisted-state and offline audits, no failed/timed-out/unknown business operation, and at least 80% sampled database CPU coverage. Open-loop arrival qualification additionally requires every arrival completed with no rejection, planned-to-visible p99 <=5 s, drain <=3 s, and generator lag <=50 ms in all three rounds. These are experiment acceptance limits, not production SLOs. Closed-loop saturation reports observed completion rates including every drain; it does not establish maximum production capacity or qualify the open-loop SLO.

No business-call refactor ablation or business migration is included. A small Weir execution-cap experiment is optional and must retain any slowdown, rejection or failure; a cap is not assumed to increase throughput. Different runners are compared through their own paired controls, never through absolute cross-host rates.

Retain the complete raw reports, source/build receipts, workflow identities, actual HTTP work and bytes, cgroup CPU and memory samples, native index/get/refresh/merge counters, reset settling observations, ownership and cleanup receipts. Analyze sampled CPU without extrapolating into missing intervals. The metric is database work and completed business throughput, not total infrastructure cost.
