# Database saturation comparison

Backend **search**. Started 2026-10-03T12:47:56.103997483Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 153472 | 0 | 153472 | 0 | 7671.2 | 98.303 | 99.6 | 98.5 | 98.5 | true |
| 32 | 8 | 1 | weir | 186624 | 0 | 186624 | 0 | 9328.9 | 90.111 | 99.6 | 97.9 | 97.9 | true |
| 32 | 8 | 2 | direct | 462272 | 0 | 462272 | 0 | 23111.4 | 73.727 | 98.8 | 95.4 | 95.4 | true |
| 32 | 8 | 2 | weir | 393664 | 0 | 393664 | 0 | 19679.4 | 49.151 | 99.4 | 99.1 | 99.1 | true |
| 32 | 8 | 3 | direct | 475424 | 0 | 475424 | 0 | 23768.0 | 73.727 | 99.0 | 95.0 | 95.0 | true |
| 32 | 8 | 3 | weir | 434656 | 0 | 434656 | 0 | 21725.7 | 45.055 | 99.4 | 98.9 | 98.9 | true |
| 32 | 32 | 1 | direct | 497184 | 0 | 497184 | 0 | 24848.5 | 98.303 | 99.6 | 98.5 | 98.5 | true |
| 32 | 32 | 1 | weir | 466176 | 0 | 466176 | 0 | 23288.0 | 90.111 | 99.8 | 95.0 | 95.0 | true |
| 32 | 32 | 2 | direct | 493280 | 0 | 493280 | 0 | 24656.8 | 98.303 | 99.5 | 98.9 | 98.9 | true |
| 32 | 32 | 2 | weir | 465312 | 0 | 465312 | 0 | 23242.7 | 90.111 | 99.7 | 95.0 | 95.0 | true |
| 32 | 32 | 3 | direct | 521568 | 0 | 521568 | 0 | 26063.3 | 90.111 | 99.8 | 98.9 | 98.9 | true |
| 32 | 32 | 3 | weir | 474880 | 0 | 474880 | 0 | 23727.4 | 90.111 | 99.6 | 99.5 | 99.5 | true |
| 32 | 64 | 1 | direct | 520512 | 0 | 520512 | 0 | 25920.8 | 114.687 | 99.6 | 99.1 | 99.1 | true |
| 32 | 64 | 1 | weir | 462240 | 0 | 462240 | 0 | 23012.8 | 131.071 | 99.3 | 99.8 | 99.8 | true |
| 32 | 64 | 2 | direct | 531936 | 0 | 531936 | 0 | 26567.7 | 114.687 | 99.7 | 98.9 | 98.9 | true |
| 32 | 64 | 2 | weir | 479136 | 0 | 479136 | 0 | 23877.5 | 131.071 | 99.5 | 99.5 | 99.5 | true |
| 32 | 64 | 3 | direct | 538944 | 0 | 538944 | 0 | 26924.8 | 114.687 | 99.8 | 98.9 | 98.9 | true |
| 32 | 64 | 3 | weir | 486016 | 0 | 486016 | 0 | 24223.5 | 131.071 | 99.5 | 95.0 | 95.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 5832 | 32.000 | 5832/0 | 0.024 | 23.510 | 0 | 32.000 |
| 32 | 8 | 2 | 12302 | 32.000 | 12302/0 | 0.027 | 9.500 | 0 | 32.000 |
| 32 | 8 | 3 | 13583 | 32.000 | 13583/0 | 0.024 | 8.410 | 0 | 32.000 |
| 32 | 32 | 1 | 14568 | 32.000 | 14568/0 | 0.188 | 32.001 | 0 | 32.000 |
| 32 | 32 | 2 | 14541 | 32.000 | 14541/0 | 0.187 | 32.210 | 0 | 32.000 |
| 32 | 32 | 3 | 14840 | 32.000 | 14840/0 | 0.194 | 31.303 | 0 | 32.000 |
| 32 | 64 | 1 | 14445 | 32.000 | 14445/0 | 29.489 | 41.592 | 0 | 32.000 |
| 32 | 64 | 2 | 14973 | 32.000 | 14973/0 | 27.702 | 39.705 | 0 | 32.000 |
| 32 | 64 | 3 | 15188 | 32.000 | 15188/0 | 27.631 | 39.345 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 25189.6 logical ops/s (32 workers), Weir 23419.3 logical ops/s (32 workers), Weir/direct 0.9297, change -7.03%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
