# Weir throughput comparison

Backend: **search**. Started: 2026-10-03T03:07:49.834588Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 10000 operations (9029 reads, 971 writes), 8 workers, 1024 pre-created records and 1024 padding bytes per document. Warmup: 1000 operations; paired rounds: 3.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 10000 | 0 | 0 | 0 | 4539.2 | 0.895 | 3.327 | 36.863 | true |
| 1 | direct / weir | weir | 10000 | 0 | 0 | 0 | 2723.1 | 2.303 | 5.631 | 15.359 | true |
| 2 | weir / direct | direct | 10000 | 0 | 0 | 0 | 8801.2 | 0.703 | 2.559 | 3.583 | true |
| 2 | weir / direct | weir | 10000 | 0 | 0 | 0 | 3276.5 | 2.303 | 5.119 | 6.655 | true |
| 3 | direct / weir | direct | 10000 | 0 | 0 | 0 | 9073.4 | 0.703 | 2.559 | 3.583 | true |
| 3 | direct / weir | weir | 10000 | 0 | 0 | 0 | 3324.1 | 2.047 | 4.607 | 6.143 | true |

Pooled throughput across all 3 verified pairs: direct **6754.6 ops/s**, Weir **3082.4 ops/s**; Weir/direct **0.4563**, Weir throughput change **-54.37%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
