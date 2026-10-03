# Weir throughput comparison

Backend: **mongo**. Started: 2026-10-03T03:06:09.652984Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 10000 operations (10000 reads, 0 writes), 8 workers, 1024 pre-created records and 1024 padding bytes per document. Warmup: 1000 operations; paired rounds: 3.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 10000 | 0 | 0 | 0 | 56812.8 | 0.143 | 0.239 | 0.319 | true |
| 1 | direct / weir | weir | 10000 | 0 | 0 | 0 | 7805.3 | 1.023 | 1.535 | 1.663 | true |
| 2 | weir / direct | direct | 10000 | 0 | 0 | 0 | 57361.2 | 0.143 | 0.223 | 0.319 | true |
| 2 | weir / direct | weir | 10000 | 0 | 0 | 0 | 7699.1 | 1.151 | 1.535 | 1.791 | true |
| 3 | direct / weir | direct | 10000 | 0 | 0 | 0 | 56501.3 | 0.143 | 0.239 | 0.319 | true |
| 3 | direct / weir | weir | 10000 | 0 | 0 | 0 | 7594.0 | 1.151 | 1.535 | 1.791 | true |

Pooled throughput across all 3 verified pairs: direct **56889.5 ops/s**, Weir **7698.5 ops/s**; Weir/direct **0.1353**, Weir throughput change **-86.47%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
