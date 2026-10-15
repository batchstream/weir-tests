# Formal settings selected from native calibration

Frozen after retaining both primary calibration attempts and before any formal dispatch. Sources, workload, completion semantics, resource controls and selection rule remain those in [PLAN.md](PLAN.md).

The selected worker level is **512 total workers across four OS clients**. Its native stage at **96 offered products/s** completed all **480/480** products without failure, unknown outcome, timeout or rejection. The source/command/binary/ownership checks passed, every process and single-record request matched the declared shape, every seeded document was verified (**3073**), and the original auditor built from the exact client commit `488b4575932265030b81312319b707e45649a028` passed the **entire nine-stage report**.

That native stage meets the unchanged sustained database CPU gate:

| Observation | Value |
| --- | ---: |
| Valid cumulative-counter intervals | 8 |
| CPU sampling coverage | 99.9259638% |
| Sampled mean database CPU / verified 1 CPU quota | 98.5481865% |
| Entire measurement time in intervals at >=90% CPU | 87.3941239% |
| Maximum generator lag | 2.314296 ms |
| Completed products/s, including the complete drain | 57.69827286153495 |

It is the only selection-eligible native stage at the lowest eligible worker level. Apply the already frozen formula exactly: `Q = floor(57.69827286153495) = 57`; rates are `floor(0.8*57)`, `57`, and `floor(1.2*57)`, giving **45, 57 and 68 offered products/s**. No relative Weir result participates in this selection.

The 2048-worker attempt is fully retained as **failed/unqualified**. It contains all nine declared stages, including two Weir stages that failed during warmup and never reached timing. At 384 products/s, timed baseline Weir had 860 failed products (694 unknown outcomes), and the candidate had 604 (474 unknown); every failure was timed out. Native at 768 had 2520 failed/timed-out/unknown products. Final-field verification failed on those timed failed-business stages, and the original whole-report Go audit failed. Successful partial native stages are shown as observations, without relabelling the overall audit as passing. No auxiliary partial-stage oracle or fabricated replacement report was needed to select the already qualified lower worker level.

These settings are **formal measurement points, not an estimated sustainable capacity**. Calibration uses only 2 seconds of warmup and 5 seconds of arrivals, and the QPS denominator includes 3.319 seconds of native completion drain at the selected point. The selected native p99 upper bound is 5.243 seconds, so this short stage does not pass the later formal all-arrivals latency/drain gates. Later native stages in the same 512-worker workflow show *lower* CPU percentages despite higher offered rates: 64.93% at 384 and 46.31% at 768. Cold execution, JIT, persistent database history and path order may contribute; the calibration does not isolate those causes. Do not turn its first-stage CPU or Q into a capacity claim.

Run one independent workflow per selected rate, with **512 workers, 10 seconds warmup, 20 seconds arrivals and three rotating rounds**. The client remains frozen to `488b4575932265030b81312319b707e45649a028` on `randy/crawler-pressure-client`; both servers, all other controls and all numeric qualification gates stay unchanged. Each formal round must establish its actual database CPU pressure independently. If the native path fails sustained CPU in any round, the resulting report cannot claim a benefit under a demonstrated database CPU bottleneck, regardless of the calibration.
