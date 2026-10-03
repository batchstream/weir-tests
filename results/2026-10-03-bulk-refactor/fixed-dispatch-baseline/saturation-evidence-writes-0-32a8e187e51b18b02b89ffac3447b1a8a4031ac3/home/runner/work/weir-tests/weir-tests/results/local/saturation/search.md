# Database saturation comparison

Backend **search**. Started 2026-10-03T11:23:03.375025293Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 459136 | 0 | 459136 | 0 | 22954.1 | 61.439 | 99.7 | 95.4 | 95.4 | true |
| 32 | 8 | 1 | weir | 647456 | 0 | 647456 | 0 | 32363.3 | 12.287 | 73.8 | 0.0 | 98.8 | true |
| 32 | 8 | 2 | direct | 999680 | 0 | 999680 | 0 | 49978.8 | 24.575 | 99.6 | 98.6 | 98.6 | true |
| 32 | 8 | 2 | weir | 652992 | 0 | 652992 | 0 | 32641.9 | 12.287 | 71.7 | 0.0 | 98.7 | true |
| 32 | 8 | 3 | direct | 1005856 | 0 | 1005856 | 0 | 50285.7 | 22.527 | 99.7 | 99.3 | 99.3 | true |
| 32 | 8 | 3 | weir | 663488 | 0 | 663488 | 0 | 33167.7 | 12.287 | 72.6 | 0.0 | 98.8 | true |
| 32 | 32 | 1 | direct | 1043872 | 0 | 1043872 | 0 | 52178.1 | 53.247 | 99.9 | 95.8 | 95.8 | true |
| 32 | 32 | 1 | weir | 704832 | 0 | 704832 | 0 | 35208.5 | 36.863 | 75.9 | 0.0 | 99.0 | true |
| 32 | 32 | 2 | direct | 1054048 | 0 | 1054048 | 0 | 52681.8 | 53.247 | 99.8 | 95.4 | 95.4 | true |
| 32 | 32 | 2 | weir | 702464 | 0 | 702464 | 0 | 35086.4 | 36.863 | 76.5 | 0.0 | 98.9 | true |
| 32 | 32 | 3 | direct | 1051648 | 0 | 1051648 | 0 | 52558.5 | 53.247 | 100.0 | 95.7 | 95.7 | true |
| 32 | 32 | 3 | weir | 699776 | 0 | 699776 | 0 | 34945.0 | 36.863 | 76.0 | 0.0 | 98.9 | true |
| 32 | 64 | 1 | direct | 1060800 | 0 | 1060800 | 0 | 53002.6 | 90.111 | 99.8 | 96.5 | 96.5 | true |
| 32 | 64 | 1 | weir | 724256 | 0 | 724256 | 0 | 36121.9 | 65.535 | 77.6 | 0.0 | 98.9 | true |
| 32 | 64 | 2 | direct | 1059200 | 0 | 1059200 | 0 | 52924.6 | 90.111 | 99.9 | 98.1 | 98.1 | true |
| 32 | 64 | 2 | weir | 727424 | 0 | 727424 | 0 | 36287.7 | 65.535 | 77.5 | 0.0 | 98.9 | true |
| 32 | 64 | 3 | direct | 1061696 | 0 | 1061696 | 0 | 53047.9 | 90.111 | 99.9 | 96.8 | 96.8 | true |
| 32 | 64 | 3 | weir | 722496 | 0 | 722496 | 0 | 36042.8 | 65.535 | 77.6 | 0.0 | 99.2 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 20233 | 32.000 | 20233/0 | 0.721 | 3.406 | 0 | 32.000 |
| 32 | 8 | 2 | 20406 | 32.000 | 20406/0 | 0.691 | 3.366 | 0 | 32.000 |
| 32 | 8 | 3 | 20734 | 32.000 | 20734/0 | 0.678 | 3.323 | 0 | 32.000 |
| 32 | 32 | 1 | 22026 | 32.000 | 22026/0 | 20.527 | 3.531 | 0 | 32.000 |
| 32 | 32 | 2 | 21952 | 32.000 | 21952/0 | 20.597 | 3.541 | 0 | 32.000 |
| 32 | 32 | 3 | 21868 | 32.000 | 21868/0 | 20.719 | 3.561 | 0 | 32.000 |
| 32 | 64 | 1 | 22633 | 32.000 | 22633/0 | 48.013 | 3.431 | 0 | 32.000 |
| 32 | 64 | 2 | 22732 | 32.000 | 22732/0 | 47.819 | 3.419 | 0 | 32.000 |
| 32 | 64 | 3 | 22578 | 32.000 | 22578/0 | 48.224 | 3.439 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
