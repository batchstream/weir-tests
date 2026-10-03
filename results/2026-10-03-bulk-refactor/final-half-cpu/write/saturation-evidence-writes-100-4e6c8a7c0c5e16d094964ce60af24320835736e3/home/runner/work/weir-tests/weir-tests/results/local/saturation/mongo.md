# Database saturation comparison

Backend **mongo**. Started 2026-10-03T12:38:58.101705279Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 0 | 78272 | 78272 | 0 | 3895.4 | 106.495 | 99.0 | 98.0 | 98.0 | true |
| 32 | 8 | 1 | weir | 0 | 63360 | 63360 | 0 | 3152.5 | 196.607 | 99.9 | 98.5 | 98.5 | true |
| 32 | 8 | 2 | direct | 0 | 61312 | 61312 | 0 | 3050.0 | 196.607 | 99.5 | 98.5 | 98.5 | true |
| 32 | 8 | 2 | weir | 0 | 62592 | 62592 | 0 | 3084.3 | 196.607 | 99.6 | 97.1 | 97.1 | true |
| 32 | 8 | 3 | direct | 0 | 65632 | 65632 | 0 | 3248.7 | 196.607 | 99.2 | 98.0 | 98.0 | true |
| 32 | 8 | 3 | weir | 0 | 57536 | 57536 | 0 | 2863.8 | 212.991 | 99.2 | 98.5 | 98.5 | true |
| 32 | 32 | 1 | direct | 0 | 61920 | 61920 | 0 | 3066.4 | 655.359 | 99.8 | 99.5 | 99.5 | true |
| 32 | 32 | 1 | weir | 0 | 49984 | 49984 | 0 | 2402.9 | 1179.647 | 99.6 | 97.1 | 97.1 | true |
| 32 | 32 | 2 | direct | 0 | 59040 | 59040 | 0 | 2907.7 | 851.967 | 99.7 | 96.5 | 96.5 | true |
| 32 | 32 | 2 | weir | 0 | 51968 | 51968 | 0 | 2561.4 | 917.503 | 99.8 | 98.5 | 98.5 | true |
| 32 | 32 | 3 | direct | 0 | 61120 | 61120 | 0 | 3026.6 | 720.895 | 99.9 | 94.6 | 94.6 | true |
| 32 | 32 | 3 | weir | 0 | 53536 | 53536 | 0 | 2625.6 | 1310.719 | 99.7 | 95.6 | 95.6 | true |
| 32 | 64 | 1 | direct | 0 | 59264 | 59264 | 0 | 2783.9 | 1703.935 | 99.8 | 99.1 | 99.1 | true |
| 32 | 64 | 1 | weir | 0 | 60608 | 60608 | 0 | 2942.3 | 917.503 | 100.1 | 94.6 | 94.6 | true |
| 32 | 64 | 2 | direct | 0 | 53888 | 53888 | 0 | 2438.3 | 2097.151 | 99.8 | 96.8 | 96.8 | true |
| 32 | 64 | 2 | weir | 0 | 56832 | 56832 | 0 | 2758.6 | 1703.935 | 100.1 | 95.1 | 95.1 | true |
| 32 | 64 | 3 | direct | 0 | 61312 | 61312 | 0 | 3018.1 | 1572.863 | 100.2 | 96.0 | 96.0 | true |
| 32 | 64 | 3 | weir | 0 | 53760 | 53760 | 0 | 2623.4 | 1572.863 | 99.8 | 99.5 | 99.5 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 1980 | 32.000 | 0/1980 | 0.032 | 78.594 | 0 | 32.000 |
| 32 | 8 | 2 | 1956 | 32.000 | 0/1956 | 0.027 | 79.959 | 0 | 32.000 |
| 32 | 8 | 3 | 1798 | 32.000 | 0/1798 | 0.036 | 86.869 | 0 | 32.000 |
| 32 | 32 | 1 | 1562 | 32.000 | 0/1562 | 0.035 | 410.331 | 0 | 32.000 |
| 32 | 32 | 2 | 1624 | 32.000 | 0/1624 | 0.040 | 393.974 | 0 | 32.000 |
| 32 | 32 | 3 | 1673 | 32.000 | 0/1673 | 0.048 | 384.544 | 0 | 32.000 |
| 32 | 64 | 1 | 1894 | 32.000 | 0/1894 | 337.379 | 346.313 | 0 | 32.000 |
| 32 | 64 | 2 | 1776 | 32.000 | 0/1776 | 360.721 | 369.348 | 0 | 32.000 |
| 32 | 64 | 3 | 1680 | 32.000 | 0/1680 | 379.055 | 388.035 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 3397.7 logical ops/s (8 workers), Weir 3033.7 logical ops/s (8 workers), Weir/direct 0.8929, change -10.71%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
