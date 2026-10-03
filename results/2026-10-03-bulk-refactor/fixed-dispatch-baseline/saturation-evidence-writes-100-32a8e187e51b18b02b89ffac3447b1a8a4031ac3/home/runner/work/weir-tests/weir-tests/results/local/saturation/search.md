# Database saturation comparison

Backend **search**. Started 2026-10-03T11:23:15.138618501Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 0 | 171648 | 171648 | 0 | 8577.9 | 81.919 | 99.9 | 99.5 | 99.5 | true |
| 32 | 8 | 1 | weir | 0 | 192800 | 192800 | 0 | 9636.7 | 81.919 | 99.9 | 95.1 | 95.1 | true |
| 32 | 8 | 2 | direct | 0 | 528288 | 528288 | 0 | 26405.7 | 12.287 | 89.3 | 46.2 | 97.8 | true |
| 32 | 8 | 2 | weir | 0 | 346624 | 346624 | 0 | 17323.9 | 53.247 | 92.5 | 62.6 | 99.2 | true |
| 32 | 8 | 3 | direct | 0 | 514656 | 514656 | 0 | 25723.7 | 13.311 | 87.2 | 25.7 | 97.8 | true |
| 32 | 8 | 3 | weir | 0 | 360992 | 360992 | 0 | 18042.0 | 36.863 | 92.4 | 57.2 | 98.9 | true |
| 32 | 32 | 1 | direct | 0 | 489408 | 489408 | 0 | 24432.4 | 90.111 | 95.2 | 72.3 | 97.9 | true |
| 32 | 32 | 1 | weir | 0 | 449280 | 449280 | 0 | 22424.5 | 81.919 | 86.6 | 20.8 | 98.3 | true |
| 32 | 32 | 2 | direct | 0 | 559296 | 559296 | 0 | 27929.0 | 73.727 | 90.2 | 25.6 | 97.7 | true |
| 32 | 32 | 2 | weir | 0 | 422464 | 422464 | 0 | 21090.8 | 98.303 | 90.9 | 41.2 | 98.0 | true |
| 32 | 32 | 3 | direct | 0 | 543616 | 543616 | 0 | 27141.4 | 81.919 | 90.6 | 30.8 | 97.5 | true |
| 32 | 32 | 3 | weir | 0 | 454304 | 454304 | 0 | 22677.6 | 73.727 | 86.8 | 25.9 | 98.3 | true |
| 32 | 64 | 1 | direct | 0 | 537632 | 537632 | 0 | 26805.4 | 131.071 | 93.6 | 61.9 | 97.9 | true |
| 32 | 64 | 1 | weir | 0 | 453600 | 453600 | 0 | 22601.0 | 131.071 | 85.5 | 20.6 | 98.0 | true |
| 32 | 64 | 2 | direct | 0 | 580448 | 580448 | 0 | 28891.0 | 114.687 | 89.4 | 45.9 | 97.1 | true |
| 32 | 64 | 2 | weir | 0 | 422464 | 422464 | 0 | 21048.1 | 163.839 | 89.1 | 36.0 | 98.1 | true |
| 32 | 64 | 3 | direct | 0 | 562688 | 562688 | 0 | 28045.8 | 122.879 | 89.3 | 30.8 | 97.3 | true |
| 32 | 64 | 3 | weir | 0 | 460992 | 460992 | 0 | 22951.2 | 122.879 | 83.9 | 15.5 | 98.2 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 6025 | 32.000 | 0/6025 | 11.115 | 13.234 | 0 | 32.000 |
| 32 | 8 | 2 | 10832 | 32.000 | 0/10832 | 5.492 | 7.348 | 0 | 32.000 |
| 32 | 8 | 3 | 11281 | 32.000 | 0/11281 | 5.211 | 7.053 | 0 | 32.000 |
| 32 | 32 | 1 | 14040 | 32.000 | 0/14040 | 38.104 | 5.670 | 0 | 32.000 |
| 32 | 32 | 2 | 13202 | 32.000 | 0/13202 | 40.502 | 6.026 | 0 | 32.000 |
| 32 | 32 | 3 | 14197 | 32.000 | 0/14197 | 37.691 | 5.605 | 0 | 32.000 |
| 32 | 64 | 1 | 14175 | 32.000 | 0/14175 | 82.907 | 5.622 | 0 | 32.000 |
| 32 | 64 | 2 | 13202 | 32.000 | 0/13202 | 89.141 | 6.039 | 0 | 32.000 |
| 32 | 64 | 3 | 14406 | 32.000 | 0/14406 | 81.579 | 5.538 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
