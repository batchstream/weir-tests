# Database saturation comparison

Backend **mongo**. Started 2026-10-03T12:37:46.878365126Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 1188352 | 133472 | 1321824 | 0 | 66055.1 | 8.191 | 99.5 | 99.8 | 99.8 | true |
| 32 | 8 | 1 | weir | 1035424 | 115328 | 1150752 | 0 | 57521.4 | 10.239 | 97.7 | 92.9 | 98.1 | true |
| 32 | 8 | 2 | direct | 993952 | 110656 | 1104608 | 0 | 55228.1 | 10.239 | 99.4 | 94.9 | 94.9 | true |
| 32 | 8 | 2 | weir | 962048 | 107168 | 1069216 | 0 | 53450.0 | 10.239 | 99.0 | 98.8 | 98.8 | true |
| 32 | 8 | 3 | direct | 961856 | 107264 | 1069120 | 0 | 53450.1 | 12.287 | 99.7 | 99.6 | 99.6 | true |
| 32 | 8 | 3 | weir | 965312 | 107968 | 1073280 | 0 | 53653.7 | 10.239 | 99.1 | 98.3 | 98.3 | true |
| 32 | 32 | 1 | direct | 1014112 | 114080 | 1128192 | 0 | 56080.4 | 81.919 | 100.1 | 95.1 | 95.1 | true |
| 32 | 32 | 1 | weir | 909600 | 102912 | 1012512 | 0 | 50601.3 | 61.439 | 100.0 | 98.9 | 98.9 | true |
| 32 | 32 | 2 | direct | 925728 | 104672 | 1030400 | 0 | 51499.3 | 81.919 | 99.8 | 95.4 | 95.4 | true |
| 32 | 32 | 2 | weir | 952768 | 108192 | 1060960 | 0 | 53030.5 | 57.343 | 100.1 | 99.3 | 99.3 | true |
| 32 | 32 | 3 | direct | 989856 | 112032 | 1101888 | 0 | 55081.8 | 81.919 | 99.9 | 96.0 | 96.0 | true |
| 32 | 32 | 3 | weir | 937248 | 105952 | 1043200 | 0 | 52142.4 | 61.439 | 100.0 | 99.7 | 99.7 | true |
| 32 | 64 | 1 | direct | 931232 | 103424 | 1034656 | 0 | 51696.3 | 106.495 | 100.2 | 98.1 | 98.1 | true |
| 32 | 64 | 1 | weir | 928768 | 102176 | 1030944 | 0 | 51460.4 | 90.111 | 100.0 | 99.5 | 99.5 | true |
| 32 | 64 | 2 | direct | 966912 | 107776 | 1074688 | 0 | 53514.1 | 98.303 | 100.1 | 98.6 | 98.6 | true |
| 32 | 64 | 2 | weir | 901184 | 98848 | 1000032 | 0 | 49846.0 | 90.111 | 100.1 | 99.7 | 99.7 | true |
| 32 | 64 | 3 | direct | 956096 | 105984 | 1062080 | 0 | 52880.2 | 98.303 | 100.0 | 98.0 | 98.0 | true |
| 32 | 64 | 3 | weir | 918976 | 101472 | 1020448 | 0 | 51002.2 | 90.111 | 99.9 | 95.3 | 95.3 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 35961 | 32.000 | 32357/3604 | 0.023 | 2.305 | 0 | 32.000 |
| 32 | 8 | 2 | 33413 | 32.000 | 30064/3349 | 0.023 | 2.678 | 0 | 32.000 |
| 32 | 8 | 3 | 33540 | 32.000 | 30166/3374 | 0.023 | 2.687 | 0 | 32.000 |
| 32 | 32 | 1 | 31641 | 32.000 | 28425/3216 | 0.169 | 12.197 | 0 | 32.000 |
| 32 | 32 | 2 | 33155 | 32.000 | 29774/3381 | 0.190 | 11.414 | 0 | 32.000 |
| 32 | 32 | 3 | 32600 | 32.000 | 29289/3311 | 0.193 | 11.849 | 0 | 32.000 |
| 32 | 64 | 1 | 32217 | 32.000 | 29024/3193 | 10.255 | 16.387 | 0 | 32.000 |
| 32 | 64 | 2 | 31251 | 32.000 | 28162/3089 | 11.229 | 17.385 | 0 | 32.000 |
| 32 | 64 | 3 | 31889 | 32.000 | 28718/3171 | 10.399 | 16.543 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 58245.6 logical ops/s (8 workers), Weir 54875.1 logical ops/s (8 workers), Weir/direct 0.9421, change -5.79%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
