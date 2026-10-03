# Database saturation comparison

Backend **search**. Started 2026-10-03T12:24:11.224442054Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 323168 | 35936 | 359104 | 0 | 17953.1 | 73.727 | 100.0 | 99.6 | 99.6 | true |
| 32 | 8 | 1 | weir | 428192 | 47808 | 476000 | 0 | 23792.3 | 49.151 | 99.0 | 98.6 | 98.6 | true |
| 32 | 8 | 2 | direct | 919808 | 101888 | 1021696 | 0 | 51076.2 | 30.719 | 99.7 | 98.4 | 98.4 | true |
| 32 | 8 | 2 | weir | 696480 | 76416 | 772896 | 0 | 38617.7 | 11.263 | 91.3 | 67.0 | 97.6 | true |
| 32 | 8 | 3 | direct | 1066496 | 118912 | 1185408 | 0 | 59264.7 | 7.167 | 99.4 | 97.8 | 97.8 | true |
| 32 | 8 | 3 | weir | 765408 | 84992 | 850400 | 0 | 42512.2 | 10.239 | 87.7 | 20.5 | 97.3 | true |
| 32 | 32 | 1 | direct | 1106176 | 124032 | 1230208 | 0 | 61494.8 | 57.343 | 99.5 | 99.3 | 99.3 | true |
| 32 | 32 | 1 | weir | 920064 | 104480 | 1024544 | 0 | 51197.5 | 36.863 | 93.0 | 97.7 | 97.7 | true |
| 32 | 32 | 2 | direct | 1132480 | 127488 | 1259968 | 0 | 62981.7 | 53.247 | 99.9 | 99.1 | 99.1 | true |
| 32 | 32 | 2 | weir | 909376 | 103072 | 1012448 | 0 | 50597.9 | 40.959 | 93.4 | 97.6 | 97.6 | true |
| 32 | 32 | 3 | direct | 1084032 | 121408 | 1205440 | 0 | 60250.4 | 57.343 | 99.7 | 99.7 | 99.7 | true |
| 32 | 32 | 3 | weir | 910176 | 102688 | 1012864 | 0 | 50620.9 | 36.863 | 92.9 | 97.7 | 97.7 | true |
| 32 | 64 | 1 | direct | 1137216 | 127424 | 1264640 | 0 | 63195.0 | 81.919 | 99.9 | 95.1 | 95.1 | true |
| 32 | 64 | 1 | weir | 906784 | 99008 | 1005792 | 0 | 50213.8 | 81.919 | 95.7 | 97.8 | 97.8 | true |
| 32 | 64 | 2 | direct | 1148736 | 128224 | 1276960 | 0 | 63814.5 | 81.919 | 99.9 | 95.1 | 95.1 | true |
| 32 | 64 | 2 | weir | 969920 | 107008 | 1076928 | 0 | 53774.5 | 73.727 | 94.9 | 97.5 | 97.5 | true |
| 32 | 64 | 3 | direct | 1171872 | 131328 | 1303200 | 0 | 65124.2 | 73.727 | 99.7 | 95.3 | 95.3 | true |
| 32 | 64 | 3 | weir | 983936 | 108320 | 1092256 | 0 | 54541.2 | 65.535 | 94.4 | 97.9 | 97.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 14875 | 32.000 | 13381/1494 | 0.020 | 7.946 | 0 | 32.000 |
| 32 | 8 | 2 | 24153 | 32.000 | 21765/2388 | 0.022 | 4.060 | 0 | 32.000 |
| 32 | 8 | 3 | 26575 | 32.000 | 23919/2656 | 0.020 | 3.523 | 0 | 32.000 |
| 32 | 32 | 1 | 32017 | 32.000 | 28752/3265 | 0.173 | 9.674 | 0 | 32.000 |
| 32 | 32 | 2 | 31639 | 32.000 | 28418/3221 | 0.176 | 9.831 | 0 | 32.000 |
| 32 | 32 | 3 | 31652 | 32.000 | 28443/3209 | 0.173 | 9.961 | 0 | 32.000 |
| 32 | 64 | 1 | 31431 | 32.000 | 28337/3094 | 5.038 | 15.357 | 0 | 32.000 |
| 32 | 64 | 2 | 33654 | 32.000 | 30310/3344 | 3.235 | 13.363 | 0 | 32.000 |
| 32 | 64 | 3 | 34133 | 32.000 | 30748/3385 | 3.169 | 13.206 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 61575.6 logical ops/s (32 workers), Weir 50805.4 logical ops/s (32 workers), Weir/direct 0.8251, change -17.49%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
