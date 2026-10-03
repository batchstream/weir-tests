# Database saturation comparison

Backend **search**. Started 2026-10-03T12:24:25.342202904Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 0 | 94976 | 94976 | 0 | 4719.0 | 229.375 | 70.5 | 20.5 | 96.4 | true |
| 32 | 8 | 1 | weir | 0 | 175104 | 175104 | 0 | 8751.7 | 122.879 | 58.7 | 5.1 | 96.7 | true |
| 32 | 8 | 2 | direct | 0 | 185248 | 185248 | 0 | 9258.2 | 163.839 | 35.7 | 0.0 | 96.6 | true |
| 32 | 8 | 2 | weir | 0 | 197120 | 197120 | 0 | 9839.2 | 114.687 | 44.8 | 0.0 | 96.5 | true |
| 32 | 8 | 3 | direct | 0 | 168256 | 168256 | 0 | 8350.0 | 147.455 | 27.4 | 0.0 | 95.7 | true |
| 32 | 8 | 3 | weir | 0 | 128416 | 128416 | 0 | 6404.5 | 163.839 | 26.3 | 0.0 | 96.6 | true |
| 32 | 32 | 1 | direct | 0 | 194560 | 194560 | 0 | 9543.8 | 393.215 | 28.3 | 0.0 | 99.6 | true |
| 32 | 32 | 1 | weir | 0 | 146176 | 146176 | 0 | 7254.7 | 425.983 | 24.1 | 0.0 | 95.7 | true |
| 32 | 32 | 2 | direct | 0 | 157824 | 157824 | 0 | 7878.8 | 458.751 | 24.7 | 0.0 | 96.2 | true |
| 32 | 32 | 2 | weir | 0 | 102272 | 102272 | 0 | 5065.8 | 589.823 | 21.1 | 0.0 | 95.6 | true |
| 32 | 32 | 3 | direct | 0 | 337376 | 337376 | 0 | 16452.1 | 229.375 | 44.8 | 0.0 | 99.3 | true |
| 32 | 32 | 3 | weir | 0 | 154016 | 154016 | 0 | 7653.1 | 425.983 | 26.3 | 0.0 | 95.8 | true |
| 32 | 64 | 1 | direct | 0 | 320960 | 320960 | 0 | 15944.2 | 360.447 | 42.9 | 0.0 | 97.1 | true |
| 32 | 64 | 1 | weir | 0 | 218336 | 218336 | 0 | 10866.9 | 491.519 | 35.3 | 0.0 | 96.1 | true |
| 32 | 64 | 2 | direct | 0 | 160128 | 160128 | 0 | 7915.6 | 720.895 | 23.4 | 0.0 | 98.6 | true |
| 32 | 64 | 2 | weir | 0 | 223680 | 223680 | 0 | 10997.3 | 589.823 | 35.9 | 0.0 | 99.9 | true |
| 32 | 64 | 3 | direct | 0 | 199776 | 199776 | 0 | 9915.1 | 589.823 | 26.4 | 0.0 | 98.2 | true |
| 32 | 64 | 3 | weir | 0 | 143744 | 143744 | 0 | 7099.7 | 851.967 | 24.5 | 0.0 | 95.7 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 5472 | 32.000 | 0/5472 | 0.013 | 28.233 | 0 | 32.000 |
| 32 | 8 | 2 | 6160 | 32.000 | 0/6160 | 0.011 | 25.091 | 0 | 32.000 |
| 32 | 8 | 3 | 4013 | 32.000 | 0/4013 | 0.012 | 39.042 | 0 | 32.000 |
| 32 | 32 | 1 | 4568 | 32.000 | 0/4568 | 0.015 | 139.841 | 0 | 32.000 |
| 32 | 32 | 2 | 3196 | 32.000 | 0/3196 | 0.027 | 200.804 | 0 | 32.000 |
| 32 | 32 | 3 | 4813 | 32.000 | 0/4813 | 0.015 | 132.345 | 0 | 32.000 |
| 32 | 64 | 1 | 6823 | 32.000 | 0/6823 | 92.738 | 93.897 | 0 | 32.000 |
| 32 | 64 | 2 | 6990 | 32.000 | 0/6990 | 91.327 | 92.639 | 0 | 32.000 |
| 32 | 64 | 3 | 4492 | 32.000 | 0/4492 | 141.991 | 143.948 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
