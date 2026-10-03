# Database saturation comparison

Backend **search**. Started 2026-10-03T12:48:24.068399063Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 0 | 40512 | 40512 | 0 | 2007.2 | 212.991 | 99.7 | 98.0 | 98.0 | true |
| 32 | 8 | 1 | weir | 0 | 66880 | 66880 | 0 | 3329.0 | 122.879 | 99.6 | 98.5 | 98.5 | true |
| 32 | 8 | 2 | direct | 0 | 115072 | 115072 | 0 | 5727.1 | 98.303 | 99.8 | 97.5 | 97.5 | true |
| 32 | 8 | 2 | weir | 0 | 79584 | 79584 | 0 | 3962.9 | 106.495 | 99.6 | 98.5 | 98.5 | true |
| 32 | 8 | 3 | direct | 0 | 132544 | 132544 | 0 | 6624.6 | 98.303 | 99.8 | 96.9 | 96.9 | true |
| 32 | 8 | 3 | weir | 0 | 199552 | 199552 | 0 | 9973.6 | 90.111 | 99.5 | 95.4 | 95.4 | true |
| 32 | 32 | 1 | direct | 0 | 227392 | 227392 | 0 | 11331.5 | 212.991 | 100.1 | 95.6 | 95.6 | true |
| 32 | 32 | 1 | weir | 0 | 207776 | 207776 | 0 | 10353.9 | 212.991 | 99.9 | 95.1 | 95.1 | true |
| 32 | 32 | 2 | direct | 0 | 244384 | 244384 | 0 | 12201.4 | 212.991 | 99.7 | 95.7 | 95.7 | true |
| 32 | 32 | 2 | weir | 0 | 234432 | 234432 | 0 | 11679.3 | 196.607 | 99.8 | 95.0 | 95.0 | true |
| 32 | 32 | 3 | direct | 0 | 275168 | 275168 | 0 | 13736.0 | 196.607 | 100.0 | 99.8 | 99.8 | true |
| 32 | 32 | 3 | weir | 0 | 209792 | 209792 | 0 | 10456.0 | 212.991 | 99.6 | 95.5 | 95.5 | true |
| 32 | 64 | 1 | direct | 0 | 276384 | 276384 | 0 | 13778.1 | 294.911 | 99.9 | 99.2 | 99.2 | true |
| 32 | 64 | 1 | weir | 0 | 263232 | 263232 | 0 | 13095.4 | 294.911 | 99.8 | 99.2 | 99.2 | true |
| 32 | 64 | 2 | direct | 0 | 297632 | 297632 | 0 | 14784.5 | 294.911 | 100.0 | 98.6 | 98.6 | true |
| 32 | 64 | 2 | weir | 0 | 236672 | 236672 | 0 | 11772.3 | 327.679 | 99.8 | 99.4 | 99.4 | true |
| 32 | 64 | 3 | direct | 0 | 280896 | 280896 | 0 | 13962.3 | 294.911 | 100.2 | 99.0 | 99.0 | true |
| 32 | 64 | 3 | weir | 0 | 251168 | 251168 | 0 | 12491.2 | 294.911 | 99.9 | 99.3 | 99.3 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 2090 | 32.000 | 0/2090 | 0.021 | 75.242 | 0 | 32.000 |
| 32 | 8 | 2 | 2487 | 32.000 | 0/2487 | 0.025 | 62.906 | 0 | 32.000 |
| 32 | 8 | 3 | 6236 | 32.000 | 0/6236 | 0.014 | 24.346 | 0 | 32.000 |
| 32 | 32 | 1 | 6493 | 32.000 | 0/6493 | 0.018 | 97.268 | 0 | 32.000 |
| 32 | 32 | 2 | 7326 | 32.000 | 0/7326 | 0.017 | 86.059 | 0 | 32.000 |
| 32 | 32 | 3 | 6556 | 32.000 | 0/6556 | 0.018 | 96.385 | 0 | 32.000 |
| 32 | 64 | 1 | 8226 | 32.000 | 0/8226 | 76.343 | 77.965 | 0 | 32.000 |
| 32 | 64 | 2 | 7396 | 32.000 | 0/7396 | 85.125 | 86.795 | 0 | 32.000 |
| 32 | 64 | 3 | 7849 | 32.000 | 0/7849 | 80.161 | 81.730 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
