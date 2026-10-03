# Database saturation comparison

Backend **mongo**. Started 2026-10-03T04:38:17.670232406Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 2154528 | 0 | 2154528 | 0 | 107721.6 | 4.095 | 99.9 | 99.5 | 99.5 | true |
| 32 | 8 | 1 | weir | 104288 | 0 | 104288 | 0 | 5209.4 | 73.727 | 63.0 | 0.0 | 99.2 | true |
| 32 | 8 | 2 | direct | 2169536 | 0 | 2169536 | 0 | 108470.3 | 4.095 | 99.9 | 99.4 | 99.4 | true |
| 32 | 8 | 2 | weir | 119328 | 0 | 119328 | 0 | 5960.1 | 57.343 | 73.1 | 0.0 | 99.0 | true |
| 32 | 8 | 3 | direct | 2173472 | 0 | 2173472 | 0 | 108669.5 | 4.095 | 99.9 | 99.3 | 99.3 | true |
| 32 | 8 | 3 | weir | 118784 | 0 | 118784 | 0 | 5933.7 | 57.343 | 72.1 | 0.0 | 98.7 | true |
| 32 | 32 | 1 | direct | 2179840 | 0 | 2179840 | 0 | 108968.5 | 36.863 | 100.0 | 99.9 | 99.9 | true |
| 32 | 32 | 1 | weir | 121536 | 0 | 121536 | 0 | 6028.2 | 393.215 | 73.3 | 0.0 | 97.9 | true |
| 32 | 32 | 2 | direct | 2179360 | 0 | 2179360 | 0 | 108948.5 | 36.863 | 100.0 | 95.1 | 95.1 | true |
| 32 | 32 | 2 | weir | 121248 | 0 | 121248 | 0 | 6018.9 | 393.215 | 73.5 | 0.0 | 98.0 | true |
| 32 | 32 | 3 | direct | 2208832 | 0 | 2208832 | 0 | 110419.1 | 36.863 | 99.9 | 95.2 | 95.2 | true |
| 32 | 32 | 3 | weir | 121216 | 0 | 121216 | 0 | 6018.6 | 393.215 | 73.5 | 0.0 | 98.1 | true |
| 32 | 64 | 1 | direct | 2153664 | 0 | 2153664 | 0 | 107634.7 | 53.247 | 100.0 | 98.5 | 98.5 | true |
| 32 | 64 | 1 | weir | 120960 | 0 | 120960 | 0 | 5950.6 | 851.967 | 72.8 | 0.0 | 97.4 | true |
| 32 | 64 | 2 | direct | 2104224 | 0 | 2104224 | 0 | 105182.1 | 57.343 | 100.0 | 98.8 | 98.8 | true |
| 32 | 64 | 2 | weir | 120960 | 0 | 120960 | 0 | 5956.5 | 851.967 | 73.1 | 0.0 | 97.4 | true |
| 32 | 64 | 3 | direct | 2158048 | 0 | 2158048 | 0 | 107877.0 | 53.247 | 99.9 | 97.9 | 97.9 | true |
| 32 | 64 | 3 | weir | 120768 | 0 | 120768 | 0 | 5946.8 | 851.967 | 72.7 | 0.0 | 97.4 | true |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
