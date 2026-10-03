# Weir throughput comparison

Backend: **search**. Started: 2026-10-03T03:10:39.66869Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 10000 operations (0 reads, 10000 writes), 8 workers, 1024 pre-created records and 1024 padding bytes per document. Warmup: 1000 operations; paired rounds: 3.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 10000 | 0 | 0 | 0 | 4269.5 | 1.663 | 3.071 | 5.631 | true |
| 1 | direct / weir | weir | 10000 | 0 | 0 | 0 | 2031.2 | 3.839 | 5.631 | 10.239 | true |
| 2 | weir / direct | direct | 10000 | 0 | 0 | 0 | 4866.0 | 1.407 | 2.815 | 4.095 | true |
| 2 | weir / direct | weir | 10000 | 0 | 0 | 0 | 2345.6 | 3.327 | 5.119 | 7.167 | true |
| 3 | direct / weir | direct | 10000 | 0 | 0 | 0 | 4682.8 | 1.535 | 2.815 | 3.839 | true |
| 3 | direct / weir | weir | 10000 | 0 | 0 | 0 | 2416.1 | 3.327 | 5.119 | 6.655 | true |

Pooled throughput across all 3 verified pairs: direct **4592.2 ops/s**, Weir **2251.4 ops/s**; Weir/direct **0.4903**, Weir throughput change **-50.97%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
