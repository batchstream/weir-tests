# Database saturation comparison

Backend **mongo**. Started 2026-10-03T12:37:52.740586995Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 2112640 | 0 | 2112640 | 0 | 105626.0 | 4.095 | 99.9 | 99.7 | 99.7 | true |
| 32 | 8 | 1 | weir | 1149280 | 0 | 1149280 | 0 | 57456.5 | 7.167 | 56.7 | 0.0 | 98.6 | true |
| 32 | 8 | 2 | direct | 2117472 | 0 | 2117472 | 0 | 105866.8 | 4.095 | 99.9 | 99.7 | 99.7 | true |
| 32 | 8 | 2 | weir | 1137728 | 0 | 1137728 | 0 | 56874.1 | 7.679 | 56.2 | 0.0 | 98.7 | true |
| 32 | 8 | 3 | direct | 2126336 | 0 | 2126336 | 0 | 106313.2 | 4.095 | 99.9 | 99.4 | 99.4 | true |
| 32 | 8 | 3 | weir | 1155392 | 0 | 1155392 | 0 | 57759.4 | 7.167 | 57.5 | 0.0 | 98.9 | true |
| 32 | 32 | 1 | direct | 2121440 | 0 | 2121440 | 0 | 106054.2 | 40.959 | 99.9 | 95.8 | 95.8 | true |
| 32 | 32 | 1 | weir | 1299424 | 0 | 1299424 | 0 | 64938.3 | 28.671 | 64.8 | 0.0 | 99.3 | true |
| 32 | 32 | 2 | direct | 2129408 | 0 | 2129408 | 0 | 106443.7 | 36.863 | 100.1 | 95.8 | 95.8 | true |
| 32 | 32 | 2 | weir | 1309984 | 0 | 1309984 | 0 | 65473.6 | 28.671 | 64.7 | 0.0 | 98.9 | true |
| 32 | 32 | 3 | direct | 2159200 | 0 | 2159200 | 0 | 107943.7 | 36.863 | 99.9 | 95.0 | 95.0 | true |
| 32 | 32 | 3 | weir | 1306112 | 0 | 1306112 | 0 | 65272.2 | 28.671 | 64.6 | 0.0 | 99.0 | true |
| 32 | 64 | 1 | direct | 2131104 | 0 | 2131104 | 0 | 106525.5 | 57.343 | 100.0 | 98.5 | 98.5 | true |
| 32 | 64 | 1 | weir | 1354528 | 0 | 1354528 | 0 | 67650.0 | 53.247 | 66.9 | 0.0 | 99.3 | true |
| 32 | 64 | 2 | direct | 2144000 | 0 | 2144000 | 0 | 107143.3 | 57.343 | 100.0 | 98.5 | 98.5 | true |
| 32 | 64 | 2 | weir | 1343616 | 0 | 1343616 | 0 | 67095.0 | 57.343 | 66.7 | 0.0 | 99.7 | true |
| 32 | 64 | 3 | direct | 2140960 | 0 | 2140960 | 0 | 107020.8 | 57.343 | 100.0 | 98.7 | 98.7 | true |
| 32 | 64 | 3 | weir | 1344896 | 0 | 1344896 | 0 | 67180.7 | 53.247 | 66.9 | 0.0 | 99.2 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 35915 | 32.000 | 35915/0 | 0.043 | 1.578 | 0 | 32.000 |
| 32 | 8 | 2 | 35554 | 32.000 | 35554/0 | 0.042 | 1.603 | 0 | 32.000 |
| 32 | 8 | 3 | 36106 | 32.000 | 36106/0 | 0.041 | 1.579 | 0 | 32.000 |
| 32 | 32 | 1 | 40607 | 32.000 | 40607/0 | 0.389 | 4.496 | 0 | 32.000 |
| 32 | 32 | 2 | 40937 | 32.000 | 40937/0 | 0.385 | 4.523 | 0 | 32.000 |
| 32 | 32 | 3 | 40816 | 32.000 | 40816/0 | 0.395 | 4.543 | 0 | 32.000 |
| 32 | 64 | 1 | 42329 | 32.000 | 42329/0 | 2.007 | 7.575 | 0 | 32.000 |
| 32 | 64 | 2 | 41988 | 32.000 | 41988/0 | 2.004 | 7.562 | 0 | 32.000 |
| 32 | 64 | 3 | 42028 | 32.000 | 42028/0 | 2.163 | 7.656 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
