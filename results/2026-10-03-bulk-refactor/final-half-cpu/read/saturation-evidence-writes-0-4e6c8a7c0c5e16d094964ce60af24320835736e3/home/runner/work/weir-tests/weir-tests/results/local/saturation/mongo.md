# Database saturation comparison

Backend **mongo**. Started 2026-10-03T12:38:51.668649449Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 1075328 | 0 | 1075328 | 0 | 53580.5 | 6.143 | 99.6 | 99.7 | 99.7 | true |
| 32 | 8 | 1 | weir | 911072 | 0 | 911072 | 0 | 45541.2 | 14.335 | 99.5 | 98.1 | 98.1 | true |
| 32 | 8 | 2 | direct | 1089088 | 0 | 1089088 | 0 | 54451.8 | 6.143 | 99.5 | 95.0 | 95.0 | true |
| 32 | 8 | 2 | weir | 935136 | 0 | 935136 | 0 | 46701.0 | 14.335 | 100.0 | 98.0 | 98.0 | true |
| 32 | 8 | 3 | direct | 1081920 | 0 | 1081920 | 0 | 54093.3 | 6.143 | 99.6 | 95.1 | 95.1 | true |
| 32 | 8 | 3 | weir | 947232 | 0 | 947232 | 0 | 47353.9 | 14.335 | 100.0 | 97.9 | 97.9 | true |
| 32 | 32 | 1 | direct | 1037696 | 0 | 1037696 | 0 | 51870.1 | 81.919 | 100.0 | 98.5 | 98.5 | true |
| 32 | 32 | 1 | weir | 931904 | 0 | 931904 | 0 | 46559.8 | 53.247 | 100.1 | 99.0 | 99.0 | true |
| 32 | 32 | 2 | direct | 1042016 | 0 | 1042016 | 0 | 52088.3 | 81.919 | 100.0 | 99.0 | 99.0 | true |
| 32 | 32 | 2 | weir | 942176 | 0 | 942176 | 0 | 47015.3 | 53.247 | 100.0 | 98.7 | 98.7 | true |
| 32 | 32 | 3 | direct | 1035360 | 0 | 1035360 | 0 | 51756.9 | 81.919 | 99.8 | 98.4 | 98.4 | true |
| 32 | 32 | 3 | weir | 936960 | 0 | 936960 | 0 | 46836.4 | 53.247 | 100.1 | 99.5 | 99.5 | true |
| 32 | 64 | 1 | direct | 1004768 | 0 | 1004768 | 0 | 50203.3 | 98.303 | 99.9 | 99.9 | 99.9 | true |
| 32 | 64 | 1 | weir | 906176 | 0 | 906176 | 0 | 45271.0 | 90.111 | 100.0 | 99.8 | 99.8 | true |
| 32 | 64 | 2 | direct | 1039872 | 0 | 1039872 | 0 | 51973.1 | 98.303 | 99.9 | 99.4 | 99.4 | true |
| 32 | 64 | 2 | weir | 914496 | 0 | 914496 | 0 | 45689.8 | 90.111 | 100.0 | 99.5 | 99.5 | true |
| 32 | 64 | 3 | direct | 1033472 | 0 | 1033472 | 0 | 51644.4 | 98.303 | 100.1 | 94.0 | 94.0 | true |
| 32 | 64 | 3 | weir | 971872 | 0 | 971872 | 0 | 48557.8 | 81.919 | 100.1 | 99.5 | 99.5 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 28471 | 32.000 | 28471/0 | 0.037 | 2.592 | 0 | 32.000 |
| 32 | 8 | 2 | 29223 | 32.000 | 29223/0 | 0.039 | 2.514 | 0 | 32.000 |
| 32 | 8 | 3 | 29601 | 32.000 | 29601/0 | 0.039 | 2.516 | 0 | 32.000 |
| 32 | 32 | 1 | 29122 | 32.000 | 29122/0 | 0.303 | 11.372 | 0 | 32.000 |
| 32 | 32 | 2 | 29443 | 32.000 | 29443/0 | 0.302 | 11.164 | 0 | 32.000 |
| 32 | 32 | 3 | 29280 | 32.000 | 29280/0 | 0.327 | 11.226 | 0 | 32.000 |
| 32 | 64 | 1 | 28318 | 32.000 | 28318/0 | 9.095 | 15.392 | 0 | 32.000 |
| 32 | 64 | 2 | 28578 | 32.000 | 28578/0 | 9.179 | 15.286 | 0 | 32.000 |
| 32 | 64 | 3 | 30371 | 32.000 | 30371/0 | 8.577 | 14.246 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 54041.3 logical ops/s (8 workers), Weir 46803.9 logical ops/s (32 workers), Weir/direct 0.8661, change -13.39%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
