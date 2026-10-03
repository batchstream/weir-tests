# Weir throughput comparison

Backend: **mongo**. Started: 2026-10-03T03:08:45.882799Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 10000 operations (0 reads, 10000 writes), 8 workers, 1024 pre-created records and 1024 padding bytes per document. Warmup: 1000 operations; paired rounds: 3.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 10000 | 0 | 0 | 0 | 945.3 | 8.191 | 10.239 | 15.359 | true |
| 1 | direct / weir | weir | 10000 | 0 | 0 | 0 | 795.3 | 9.215 | 13.311 | 14.335 | true |
| 2 | weir / direct | direct | 10000 | 0 | 0 | 0 | 956.4 | 8.191 | 10.239 | 14.335 | true |
| 2 | weir / direct | weir | 10000 | 0 | 0 | 0 | 764.3 | 10.239 | 14.335 | 18.431 | true |
| 3 | direct / weir | direct | 10000 | 0 | 0 | 0 | 949.4 | 8.191 | 10.239 | 13.311 | true |
| 3 | direct / weir | weir | 10000 | 0 | 0 | 0 | 795.5 | 9.215 | 13.311 | 15.359 | true |

Pooled throughput across all 3 verified pairs: direct **950.4 ops/s**, Weir **784.7 ops/s**; Weir/direct **0.8257**, Weir throughput change **-17.43%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
