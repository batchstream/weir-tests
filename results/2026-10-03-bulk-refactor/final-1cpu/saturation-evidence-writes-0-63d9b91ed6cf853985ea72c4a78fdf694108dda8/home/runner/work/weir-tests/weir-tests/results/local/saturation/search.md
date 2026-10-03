# Database saturation comparison

Backend **search**. Started 2026-10-03T12:46:55.700173398Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 462624 | 0 | 462624 | 0 | 23126.9 | 61.439 | 99.8 | 95.0 | 95.0 | true |
| 32 | 8 | 1 | weir | 697696 | 0 | 697696 | 0 | 34881.8 | 12.287 | 78.0 | 0.0 | 98.9 | true |
| 32 | 8 | 2 | direct | 1015520 | 0 | 1015520 | 0 | 50769.8 | 20.479 | 99.5 | 99.3 | 99.3 | true |
| 32 | 8 | 2 | weir | 704416 | 0 | 704416 | 0 | 35214.5 | 12.287 | 76.1 | 0.0 | 98.6 | true |
| 32 | 8 | 3 | direct | 1027424 | 0 | 1027424 | 0 | 51368.4 | 12.287 | 99.6 | 98.9 | 98.9 | true |
| 32 | 8 | 3 | weir | 708192 | 0 | 708192 | 0 | 35402.1 | 12.287 | 75.1 | 0.0 | 98.8 | true |
| 32 | 32 | 1 | direct | 1051456 | 0 | 1051456 | 0 | 52558.7 | 53.247 | 99.7 | 95.3 | 95.3 | true |
| 32 | 32 | 1 | weir | 794144 | 0 | 794144 | 0 | 39675.2 | 45.055 | 81.2 | 0.0 | 99.6 | true |
| 32 | 32 | 2 | direct | 1062976 | 0 | 1062976 | 0 | 53134.1 | 49.151 | 99.8 | 95.5 | 95.5 | true |
| 32 | 32 | 2 | weir | 799520 | 0 | 799520 | 0 | 39949.5 | 45.055 | 80.7 | 0.0 | 99.5 | true |
| 32 | 32 | 3 | direct | 1061280 | 0 | 1061280 | 0 | 53039.2 | 49.151 | 99.9 | 95.6 | 95.6 | true |
| 32 | 32 | 3 | weir | 798048 | 0 | 798048 | 0 | 39872.5 | 45.055 | 80.3 | 0.0 | 99.1 | true |
| 32 | 64 | 1 | direct | 1064704 | 0 | 1064704 | 0 | 53202.9 | 90.111 | 99.8 | 96.4 | 96.4 | true |
| 32 | 64 | 1 | weir | 818400 | 0 | 818400 | 0 | 40846.8 | 90.111 | 82.8 | 0.0 | 99.7 | true |
| 32 | 64 | 2 | direct | 1054304 | 0 | 1054304 | 0 | 52679.5 | 90.111 | 99.8 | 96.7 | 96.7 | true |
| 32 | 64 | 2 | weir | 821664 | 0 | 821664 | 0 | 41015.0 | 90.111 | 82.9 | 0.0 | 99.4 | true |
| 32 | 64 | 3 | direct | 1067040 | 0 | 1067040 | 0 | 53315.1 | 90.111 | 99.8 | 96.7 | 96.7 | true |
| 32 | 64 | 3 | weir | 818592 | 0 | 818592 | 0 | 40878.4 | 90.111 | 82.7 | 0.0 | 99.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 21803 | 32.000 | 21803/0 | 0.027 | 3.925 | 0 | 32.000 |
| 32 | 8 | 2 | 22013 | 32.000 | 22013/0 | 0.027 | 3.883 | 0 | 32.000 |
| 32 | 8 | 3 | 22131 | 32.000 | 22131/0 | 0.026 | 3.867 | 0 | 32.000 |
| 32 | 32 | 1 | 24817 | 32.000 | 24817/0 | 0.211 | 11.107 | 0 | 32.000 |
| 32 | 32 | 2 | 24985 | 32.000 | 24985/0 | 0.226 | 10.854 | 0 | 32.000 |
| 32 | 32 | 3 | 24939 | 32.000 | 24939/0 | 0.214 | 11.025 | 0 | 32.000 |
| 32 | 64 | 1 | 25575 | 32.000 | 25575/0 | 4.292 | 18.559 | 0 | 32.000 |
| 32 | 64 | 2 | 25677 | 32.000 | 25677/0 | 4.376 | 18.579 | 0 | 32.000 |
| 32 | 64 | 3 | 25581 | 32.000 | 25581/0 | 4.518 | 18.721 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
