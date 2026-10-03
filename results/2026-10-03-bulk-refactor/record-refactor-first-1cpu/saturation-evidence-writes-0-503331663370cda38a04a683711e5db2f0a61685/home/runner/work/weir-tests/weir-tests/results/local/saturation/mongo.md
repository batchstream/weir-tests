# Database saturation comparison

Backend **mongo**. Started 2026-10-03T12:15:19.060933729Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 2070016 | 0 | 2070016 | 0 | 103496.5 | 4.607 | 99.9 | 99.8 | 99.8 | true |
| 32 | 8 | 1 | weir | 1082912 | 0 | 1082912 | 0 | 54135.8 | 8.191 | 54.8 | 0.0 | 98.6 | true |
| 32 | 8 | 2 | direct | 2110944 | 0 | 2110944 | 0 | 105441.9 | 4.607 | 100.1 | 99.8 | 99.8 | true |
| 32 | 8 | 2 | weir | 1110848 | 0 | 1110848 | 0 | 55534.0 | 7.679 | 55.7 | 0.0 | 98.9 | true |
| 32 | 8 | 3 | direct | 2123424 | 0 | 2123424 | 0 | 106164.3 | 4.095 | 100.0 | 99.7 | 99.7 | true |
| 32 | 8 | 3 | weir | 1109760 | 0 | 1109760 | 0 | 55480.7 | 7.679 | 55.7 | 0.0 | 98.8 | true |
| 32 | 32 | 1 | direct | 2093696 | 0 | 2093696 | 0 | 104665.9 | 40.959 | 100.1 | 95.2 | 95.2 | true |
| 32 | 32 | 1 | weir | 1272704 | 0 | 1272704 | 0 | 63619.6 | 28.671 | 64.2 | 0.0 | 99.5 | true |
| 32 | 32 | 2 | direct | 2072000 | 0 | 2072000 | 0 | 103574.2 | 40.959 | 99.9 | 95.9 | 95.9 | true |
| 32 | 32 | 2 | weir | 1274112 | 0 | 1274112 | 0 | 63660.0 | 28.671 | 65.0 | 0.0 | 99.3 | true |
| 32 | 32 | 3 | direct | 2090720 | 0 | 2090720 | 0 | 104518.1 | 40.959 | 99.9 | 95.3 | 95.3 | true |
| 32 | 32 | 3 | weir | 1268544 | 0 | 1268544 | 0 | 63399.6 | 28.671 | 64.8 | 0.0 | 99.3 | true |
| 32 | 64 | 1 | direct | 2086080 | 0 | 2086080 | 0 | 104098.8 | 57.343 | 100.0 | 97.6 | 97.6 | true |
| 32 | 64 | 1 | weir | 1325824 | 0 | 1325824 | 0 | 66246.3 | 57.343 | 66.9 | 0.0 | 99.5 | true |
| 32 | 64 | 2 | direct | 2058496 | 0 | 2058496 | 0 | 102887.6 | 57.343 | 100.1 | 99.1 | 99.1 | true |
| 32 | 64 | 2 | weir | 1316928 | 0 | 1316928 | 0 | 65785.8 | 57.343 | 67.0 | 0.0 | 99.5 | true |
| 32 | 64 | 3 | direct | 2067136 | 0 | 2067136 | 0 | 103290.4 | 57.343 | 100.0 | 98.0 | 98.0 | true |
| 32 | 64 | 3 | weir | 1316768 | 0 | 1316768 | 0 | 65771.8 | 57.343 | 66.4 | 0.0 | 99.4 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 33841 | 32.000 | 33841/0 | 0.045 | 1.680 | 0 | 32.000 |
| 32 | 8 | 2 | 34714 | 32.000 | 34714/0 | 0.040 | 1.650 | 0 | 32.000 |
| 32 | 8 | 3 | 34680 | 32.000 | 34680/0 | 0.042 | 1.645 | 0 | 32.000 |
| 32 | 32 | 1 | 39772 | 32.000 | 39772/0 | 0.377 | 4.596 | 0 | 32.000 |
| 32 | 32 | 2 | 39816 | 32.000 | 39816/0 | 0.396 | 4.594 | 0 | 32.000 |
| 32 | 32 | 3 | 39642 | 32.000 | 39642/0 | 0.404 | 4.687 | 0 | 32.000 |
| 32 | 64 | 1 | 41432 | 32.000 | 41432/0 | 1.877 | 7.588 | 0 | 32.000 |
| 32 | 64 | 2 | 41154 | 32.000 | 41154/0 | 2.121 | 7.657 | 0 | 32.000 |
| 32 | 64 | 3 | 41149 | 32.000 | 41149/0 | 2.132 | 8.092 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
