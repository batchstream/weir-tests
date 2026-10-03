# Database saturation comparison

Backend **search**. Started 2026-10-03T12:24:22.124481957Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 442080 | 0 | 442080 | 0 | 22101.1 | 61.439 | 99.9 | 96.0 | 96.0 | true |
| 32 | 8 | 1 | weir | 655616 | 0 | 655616 | 0 | 32773.9 | 13.311 | 79.7 | 10.5 | 99.0 | true |
| 32 | 8 | 2 | direct | 988608 | 0 | 988608 | 0 | 49425.0 | 24.575 | 99.6 | 99.4 | 99.4 | true |
| 32 | 8 | 2 | weir | 689600 | 0 | 689600 | 0 | 34472.7 | 12.287 | 75.8 | 0.0 | 98.7 | true |
| 32 | 8 | 3 | direct | 986848 | 0 | 986848 | 0 | 49337.4 | 24.575 | 99.7 | 99.2 | 99.2 | true |
| 32 | 8 | 3 | weir | 693792 | 0 | 693792 | 0 | 34680.9 | 12.287 | 75.9 | 0.0 | 98.7 | true |
| 32 | 32 | 1 | direct | 1003968 | 0 | 1003968 | 0 | 50182.8 | 53.247 | 99.9 | 95.7 | 95.7 | true |
| 32 | 32 | 1 | weir | 785088 | 0 | 785088 | 0 | 39222.5 | 49.151 | 82.0 | 0.0 | 99.1 | true |
| 32 | 32 | 2 | direct | 1027680 | 0 | 1027680 | 0 | 51350.9 | 53.247 | 99.8 | 99.9 | 99.9 | true |
| 32 | 32 | 2 | weir | 790112 | 0 | 790112 | 0 | 39478.3 | 49.151 | 81.6 | 0.0 | 99.0 | true |
| 32 | 32 | 3 | direct | 1035296 | 0 | 1035296 | 0 | 51741.7 | 53.247 | 99.7 | 95.0 | 95.0 | true |
| 32 | 32 | 3 | weir | 788288 | 0 | 788288 | 0 | 39383.4 | 49.151 | 81.4 | 0.0 | 99.2 | true |
| 32 | 64 | 1 | direct | 1022400 | 0 | 1022400 | 0 | 51069.1 | 90.111 | 99.9 | 96.8 | 96.8 | true |
| 32 | 64 | 1 | weir | 797952 | 0 | 797952 | 0 | 39803.8 | 90.111 | 84.0 | 0.0 | 99.7 | true |
| 32 | 64 | 2 | direct | 1014080 | 0 | 1014080 | 0 | 50673.3 | 90.111 | 99.8 | 97.2 | 97.2 | true |
| 32 | 64 | 2 | weir | 783680 | 0 | 783680 | 0 | 39107.1 | 98.303 | 83.8 | 5.2 | 99.6 | true |
| 32 | 64 | 3 | direct | 1032512 | 0 | 1032512 | 0 | 51594.4 | 90.111 | 99.8 | 97.7 | 97.7 | true |
| 32 | 64 | 3 | weir | 800768 | 0 | 800768 | 0 | 39979.6 | 90.111 | 83.7 | 0.0 | 99.7 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 20488 | 32.000 | 20488/0 | 0.029 | 4.268 | 0 | 32.000 |
| 32 | 8 | 2 | 21550 | 32.000 | 21550/0 | 0.028 | 3.953 | 0 | 32.000 |
| 32 | 8 | 3 | 21681 | 32.000 | 21681/0 | 0.026 | 3.941 | 0 | 32.000 |
| 32 | 32 | 1 | 24534 | 32.000 | 24534/0 | 0.256 | 12.037 | 0 | 32.000 |
| 32 | 32 | 2 | 24691 | 32.000 | 24691/0 | 0.254 | 11.969 | 0 | 32.000 |
| 32 | 32 | 3 | 24634 | 32.000 | 24634/0 | 0.235 | 11.949 | 0 | 32.000 |
| 32 | 64 | 1 | 24936 | 32.000 | 24936/0 | 4.332 | 18.917 | 0 | 32.000 |
| 32 | 64 | 2 | 24490 | 32.000 | 24490/0 | 4.239 | 18.921 | 0 | 32.000 |
| 32 | 64 | 3 | 25024 | 32.000 | 25024/0 | 4.119 | 18.831 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
