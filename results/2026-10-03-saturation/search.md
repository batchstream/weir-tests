# Database saturation comparison

Backend **search**. Started 2026-10-03T04:47:23.145160036Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 476512 | 0 | 476512 | 0 | 23822.1 | 61.439 | 99.8 | 96.1 | 96.1 | true |
| 32 | 8 | 1 | weir | 114208 | 0 | 114208 | 0 | 5693.8 | 61.439 | 78.6 | 0.0 | 98.5 | true |
| 32 | 8 | 2 | direct | 1010496 | 0 | 1010496 | 0 | 50519.0 | 22.527 | 99.6 | 99.3 | 99.3 | true |
| 32 | 8 | 2 | weir | 114624 | 0 | 114624 | 0 | 5724.8 | 61.439 | 78.3 | 0.0 | 98.5 | true |
| 32 | 8 | 3 | direct | 998784 | 0 | 998784 | 0 | 49931.5 | 24.575 | 99.7 | 99.1 | 99.1 | true |
| 32 | 8 | 3 | weir | 116032 | 0 | 116032 | 0 | 5795.5 | 57.343 | 77.5 | 0.0 | 98.8 | true |
| 32 | 32 | 1 | direct | 1029600 | 0 | 1029600 | 0 | 51457.5 | 53.247 | 99.7 | 95.1 | 95.1 | true |
| 32 | 32 | 1 | weir | 117312 | 0 | 117312 | 0 | 5823.8 | 327.679 | 77.3 | 0.0 | 98.0 | true |
| 32 | 32 | 2 | direct | 1046912 | 0 | 1046912 | 0 | 52329.3 | 53.247 | 99.6 | 94.8 | 94.8 | true |
| 32 | 32 | 2 | weir | 116544 | 0 | 116544 | 0 | 5784.3 | 327.679 | 75.5 | 0.0 | 98.2 | true |
| 32 | 32 | 3 | direct | 1044128 | 0 | 1044128 | 0 | 52186.1 | 53.247 | 99.8 | 95.0 | 95.0 | true |
| 32 | 32 | 3 | weir | 117728 | 0 | 117728 | 0 | 5841.1 | 360.447 | 75.6 | 0.0 | 97.9 | true |
| 32 | 64 | 1 | direct | 1053600 | 0 | 1053600 | 0 | 52643.2 | 90.111 | 99.7 | 96.7 | 96.7 | true |
| 32 | 64 | 1 | weir | 120192 | 0 | 120192 | 0 | 5912.7 | 655.359 | 75.7 | 0.0 | 96.9 | true |
| 32 | 64 | 2 | direct | 1058240 | 0 | 1058240 | 0 | 52865.3 | 90.111 | 99.8 | 96.3 | 96.3 | true |
| 32 | 64 | 2 | weir | 119968 | 0 | 119968 | 0 | 5905.6 | 655.359 | 75.1 | 0.0 | 96.8 | true |
| 32 | 64 | 3 | direct | 1061824 | 0 | 1061824 | 0 | 53050.4 | 90.111 | 99.9 | 95.9 | 95.9 | true |
| 32 | 64 | 3 | weir | 119168 | 0 | 119168 | 0 | 5863.9 | 655.359 | 75.7 | 0.0 | 97.3 | true |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
