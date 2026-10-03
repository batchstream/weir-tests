# Database saturation comparison

Backend **search**. Started 2026-10-03T12:47:02.293895536Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 0 | 141824 | 141824 | 0 | 7087.1 | 90.111 | 100.0 | 95.0 | 95.0 | true |
| 32 | 8 | 1 | weir | 0 | 164224 | 164224 | 0 | 8207.0 | 81.919 | 100.1 | 95.1 | 95.1 | true |
| 32 | 8 | 2 | direct | 0 | 478176 | 478176 | 0 | 23898.3 | 16.383 | 92.4 | 77.3 | 97.9 | true |
| 32 | 8 | 2 | weir | 0 | 298848 | 298848 | 0 | 14937.2 | 61.439 | 96.1 | 88.4 | 99.0 | true |
| 32 | 8 | 3 | direct | 0 | 500608 | 500608 | 0 | 25022.7 | 13.311 | 89.5 | 31.1 | 97.9 | true |
| 32 | 8 | 3 | weir | 0 | 345344 | 345344 | 0 | 17260.2 | 49.151 | 94.5 | 68.1 | 99.2 | true |
| 32 | 32 | 1 | direct | 0 | 486752 | 486752 | 0 | 24302.0 | 90.111 | 93.0 | 62.1 | 98.0 | true |
| 32 | 32 | 1 | weir | 0 | 425216 | 425216 | 0 | 21227.4 | 90.111 | 89.8 | 36.4 | 98.5 | true |
| 32 | 32 | 2 | direct | 0 | 543136 | 543136 | 0 | 27120.0 | 73.727 | 91.5 | 46.1 | 97.4 | true |
| 32 | 32 | 2 | weir | 0 | 352320 | 352320 | 0 | 17588.5 | 106.495 | 95.7 | 73.4 | 99.3 | true |
| 32 | 32 | 3 | direct | 0 | 457536 | 457536 | 0 | 22846.4 | 98.303 | 96.8 | 87.6 | 97.8 | true |
| 32 | 32 | 3 | weir | 0 | 436128 | 436128 | 0 | 21767.9 | 81.919 | 88.8 | 31.2 | 98.8 | true |
| 32 | 64 | 1 | direct | 0 | 554752 | 554752 | 0 | 27639.1 | 122.879 | 92.1 | 61.8 | 97.8 | true |
| 32 | 64 | 1 | weir | 0 | 375296 | 375296 | 0 | 18701.9 | 294.911 | 92.2 | 46.8 | 98.5 | true |
| 32 | 64 | 2 | direct | 0 | 522304 | 522304 | 0 | 26040.0 | 131.071 | 94.3 | 82.4 | 97.7 | true |
| 32 | 64 | 2 | weir | 0 | 433184 | 433184 | 0 | 21585.6 | 163.839 | 89.6 | 30.9 | 98.1 | true |
| 32 | 64 | 3 | direct | 0 | 507552 | 507552 | 0 | 25301.7 | 131.071 | 93.7 | 56.7 | 97.9 | true |
| 32 | 64 | 3 | weir | 0 | 462816 | 462816 | 0 | 23057.6 | 131.071 | 88.9 | 31.0 | 98.1 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 5132 | 32.000 | 0/5132 | 0.022 | 29.346 | 0 | 32.000 |
| 32 | 8 | 2 | 9339 | 32.000 | 0/9339 | 0.017 | 15.530 | 0 | 32.000 |
| 32 | 8 | 3 | 10792 | 32.000 | 0/10792 | 0.016 | 13.256 | 0 | 32.000 |
| 32 | 32 | 1 | 13288 | 32.000 | 0/13288 | 0.020 | 46.681 | 0 | 32.000 |
| 32 | 32 | 2 | 11010 | 32.000 | 0/11010 | 0.020 | 56.558 | 0 | 32.000 |
| 32 | 32 | 3 | 13629 | 32.000 | 0/13629 | 0.019 | 45.467 | 0 | 32.000 |
| 32 | 64 | 1 | 11728 | 32.000 | 0/11728 | 52.662 | 54.541 | 0 | 32.000 |
| 32 | 64 | 2 | 13537 | 32.000 | 0/13537 | 45.514 | 47.268 | 0 | 32.000 |
| 32 | 64 | 3 | 14463 | 32.000 | 0/14463 | 42.499 | 44.208 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
