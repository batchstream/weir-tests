# Database saturation comparison

Backend **mongo**. Started 2026-10-03T11:14:00.407447472Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 2073696 | 0 | 2073696 | 0 | 103679.1 | 4.607 | 99.9 | 99.7 | 99.7 | true |
| 32 | 8 | 1 | weir | 1035072 | 0 | 1035072 | 0 | 51743.8 | 8.191 | 52.6 | 0.0 | 98.6 | true |
| 32 | 8 | 2 | direct | 2116736 | 0 | 2116736 | 0 | 105826.9 | 4.607 | 100.0 | 99.6 | 99.6 | true |
| 32 | 8 | 2 | weir | 1049632 | 0 | 1049632 | 0 | 52470.6 | 7.679 | 53.5 | 0.0 | 98.5 | true |
| 32 | 8 | 3 | direct | 2079168 | 0 | 2079168 | 0 | 103953.0 | 4.095 | 99.9 | 99.8 | 99.8 | true |
| 32 | 8 | 3 | weir | 1048320 | 0 | 1048320 | 0 | 52410.2 | 8.191 | 53.7 | 0.0 | 98.6 | true |
| 32 | 32 | 1 | direct | 2050976 | 0 | 2050976 | 0 | 102526.5 | 40.959 | 99.9 | 95.9 | 95.9 | true |
| 32 | 32 | 1 | weir | 1149888 | 0 | 1149888 | 0 | 57453.9 | 30.719 | 59.0 | 0.0 | 99.3 | true |
| 32 | 32 | 2 | direct | 2047456 | 0 | 2047456 | 0 | 102348.9 | 40.959 | 99.9 | 96.7 | 96.7 | true |
| 32 | 32 | 2 | weir | 1142848 | 0 | 1142848 | 0 | 57119.2 | 30.719 | 58.8 | 0.0 | 99.0 | true |
| 32 | 32 | 3 | direct | 2076480 | 0 | 2076480 | 0 | 103799.4 | 40.959 | 100.1 | 94.8 | 94.8 | true |
| 32 | 32 | 3 | weir | 1120160 | 0 | 1120160 | 0 | 55965.5 | 30.719 | 59.0 | 0.0 | 99.3 | true |
| 32 | 64 | 1 | direct | 2054432 | 0 | 2054432 | 0 | 102671.0 | 57.343 | 99.9 | 98.3 | 98.3 | true |
| 32 | 64 | 1 | weir | 1185248 | 0 | 1185248 | 0 | 59197.1 | 49.151 | 60.4 | 0.0 | 99.2 | true |
| 32 | 64 | 2 | direct | 2087296 | 0 | 2087296 | 0 | 104307.3 | 57.343 | 100.0 | 97.9 | 97.9 | true |
| 32 | 64 | 2 | weir | 1183488 | 0 | 1183488 | 0 | 59107.8 | 49.151 | 60.3 | 0.0 | 99.3 | true |
| 32 | 64 | 3 | direct | 2085024 | 0 | 2085024 | 0 | 104221.9 | 57.343 | 99.9 | 96.9 | 96.9 | true |
| 32 | 64 | 3 | weir | 1178400 | 0 | 1178400 | 0 | 58844.4 | 49.151 | 60.0 | 0.0 | 99.1 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 32346 | 32.000 | 32346/0 | 0.044 | 1.618 | 0 | 32.000 |
| 32 | 8 | 2 | 32801 | 32.000 | 32801/0 | 0.043 | 1.603 | 0 | 32.000 |
| 32 | 8 | 3 | 32760 | 32.000 | 32760/0 | 0.041 | 1.598 | 0 | 32.000 |
| 32 | 32 | 1 | 35934 | 32.000 | 35934/0 | 2.842 | 3.675 | 0 | 32.000 |
| 32 | 32 | 2 | 35714 | 32.000 | 35714/0 | 2.844 | 3.735 | 0 | 32.000 |
| 32 | 32 | 3 | 35005 | 32.000 | 35005/0 | 2.988 | 3.810 | 0 | 32.000 |
| 32 | 64 | 1 | 37039 | 32.000 | 37039/0 | 17.019 | 3.891 | 0 | 32.000 |
| 32 | 64 | 2 | 36984 | 32.000 | 36984/0 | 17.181 | 3.934 | 0 | 32.000 |
| 32 | 64 | 3 | 36825 | 32.000 | 36825/0 | 17.399 | 3.919 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
