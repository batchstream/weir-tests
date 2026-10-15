# Frozen formal-rate decision

Recorded on 2026-10-15 after calibration workflow [38727143882](https://github.com/batchstream/weir-tests/actions/runs/38727143882), before dispatching formal measurements. Client source remains ad1879156c14028285f27f18794f1580e979af63. Both servers remain c3101c6628872345d6fb09b977ce853f25936b79, with cap0/cap16 as declared in PLAN.md.

Native and default Weir completed every offered product without rejection, failure, unknown or timeout at all three calibration rates. The selection observations were:

| Offered products/s | Native p99 (s) | cap0 p99 (s) | Native drain (s) | cap0 drain (s) | Native max lag (ms) | cap0 max lag (ms) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4 | 2.359 | 2.621 | 1.913 | 1.384 | 1.060 | 0.937 |
| 16 | 2.359 | 2.884 | 1.567 | 1.516 | 3.891 | 4.035 |
| 48 | 3.408 | 3.670 | 1.784 | 2.798 | 7.307 | 18.032 |

All values meet the predeclared selection gates. Therefore use the plan's final branch: **16, 48 and 96 offered products/s**. cap16 at 48 had p99 5.767 s and drain 4.812 s; this is retained and does not change selection based on native/cap0. The short calibration provides no formal savings or capacity claim.

Run one independent workflow at each rate, with native/cap0/cap16 on the same runner/database and three rotating rounds per workflow. Every workflow uses single input, page1, 4 OS clients, 128 goroutine workers, 10 s warmup, 20 s arrivals, disabled observer, 1 database CPU, 512 MiB heap, 1536 MiB container memory, 10 s business deadline and queue1. No source, resource, deadline or acceptance limit changes. Compare paired paths only within each workflow and round, and retain every overload or failure. Do not construct absolute cross-runner capacity curves.
