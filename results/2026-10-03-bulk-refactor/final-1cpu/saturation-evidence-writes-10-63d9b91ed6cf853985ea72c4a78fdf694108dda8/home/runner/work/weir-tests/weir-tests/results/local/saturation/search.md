# Database saturation comparison

Backend **search**. Started 2026-10-03T12:46:51.239609985Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 347264 | 39008 | 386272 | 0 | 19312.0 | 73.727 | 99.8 | 95.0 | 95.0 | true |
| 32 | 8 | 1 | weir | 491456 | 54176 | 545632 | 0 | 27278.0 | 45.055 | 98.3 | 93.6 | 99.0 | true |
| 32 | 8 | 2 | direct | 921824 | 101952 | 1023776 | 0 | 51182.2 | 28.671 | 99.7 | 98.2 | 98.2 | true |
| 32 | 8 | 2 | weir | 712864 | 78208 | 791072 | 0 | 39545.6 | 11.263 | 90.9 | 51.8 | 98.2 | true |
| 32 | 8 | 3 | direct | 1047776 | 116384 | 1164160 | 0 | 58202.0 | 7.679 | 99.8 | 98.3 | 98.3 | true |
| 32 | 8 | 3 | weir | 767296 | 85376 | 852672 | 0 | 42627.1 | 10.239 | 87.9 | 25.8 | 98.0 | true |
| 32 | 32 | 1 | direct | 1071392 | 120096 | 1191488 | 0 | 59547.9 | 57.343 | 99.6 | 99.9 | 99.9 | true |
| 32 | 32 | 1 | weir | 923680 | 104544 | 1028224 | 0 | 51382.7 | 36.863 | 92.7 | 98.5 | 98.5 | true |
| 32 | 32 | 2 | direct | 1123104 | 126176 | 1249280 | 0 | 62446.7 | 53.247 | 99.8 | 99.7 | 99.7 | true |
| 32 | 32 | 2 | weir | 912256 | 103232 | 1015488 | 0 | 50750.4 | 40.959 | 93.1 | 98.6 | 98.6 | true |
| 32 | 32 | 3 | direct | 1060256 | 118912 | 1179168 | 0 | 58943.0 | 57.343 | 99.8 | 99.2 | 99.2 | true |
| 32 | 32 | 3 | weir | 938080 | 105824 | 1043904 | 0 | 52167.8 | 36.863 | 92.8 | 98.5 | 98.5 | true |
| 32 | 64 | 1 | direct | 1129184 | 126176 | 1255360 | 0 | 62733.2 | 81.919 | 99.8 | 96.2 | 96.2 | true |
| 32 | 64 | 1 | weir | 931968 | 102816 | 1034784 | 0 | 51684.0 | 73.727 | 94.8 | 98.4 | 98.4 | true |
| 32 | 64 | 2 | direct | 1084928 | 120096 | 1205024 | 0 | 60203.3 | 81.919 | 99.9 | 97.1 | 97.1 | true |
| 32 | 64 | 2 | weir | 976192 | 107936 | 1084128 | 0 | 54166.7 | 65.535 | 94.2 | 98.7 | 98.7 | true |
| 32 | 64 | 3 | direct | 1144096 | 128160 | 1272256 | 0 | 63569.0 | 81.919 | 99.9 | 95.7 | 95.7 | true |
| 32 | 64 | 3 | weir | 950496 | 104192 | 1054688 | 0 | 52670.5 | 73.727 | 93.2 | 99.2 | 99.2 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 17051 | 32.000 | 15358/1693 | 0.018 | 6.606 | 0 | 32.000 |
| 32 | 8 | 2 | 24721 | 32.000 | 22277/2444 | 0.017 | 3.920 | 0 | 32.000 |
| 32 | 8 | 3 | 26646 | 32.000 | 23978/2668 | 0.017 | 3.482 | 0 | 32.000 |
| 32 | 32 | 1 | 32132 | 32.000 | 28865/3267 | 0.143 | 9.048 | 0 | 32.000 |
| 32 | 32 | 2 | 31734 | 32.000 | 28508/3226 | 0.149 | 9.478 | 0 | 32.000 |
| 32 | 32 | 3 | 32622 | 32.000 | 29315/3307 | 0.159 | 8.849 | 0 | 32.000 |
| 32 | 64 | 1 | 32337 | 32.000 | 29124/3213 | 3.515 | 13.069 | 0 | 32.000 |
| 32 | 64 | 2 | 33879 | 32.000 | 30506/3373 | 2.575 | 12.128 | 0 | 32.000 |
| 32 | 64 | 3 | 32959 | 32.000 | 29703/3256 | 3.325 | 12.969 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 60312.5 logical ops/s (32 workers), Weir 51433.7 logical ops/s (32 workers), Weir/direct 0.8528, change -14.72%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
