# Weir throughput comparison

Backend: **mongo**. Started: 2026-10-03T12:38:21.667773977Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 128 operations (117 reads, 11 writes), 8 workers, 64 pre-created records and 1024 padding bytes per document. Warmup: 32 operations; paired rounds: 2.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 128 | 0 | 0 | 0 | 7661.8 | 0.831 | 1.791 | 2.559 | true |
| 1 | direct / weir | weir | 128 | 0 | 0 | 0 | 3008.3 | 2.303 | 4.095 | 6.143 | true |
| 2 | weir / direct | direct | 128 | 0 | 0 | 0 | 3831.5 | 1.407 | 4.607 | 4.607 | true |
| 2 | weir / direct | weir | 128 | 0 | 0 | 0 | 2665.3 | 2.815 | 5.631 | 6.143 | true |

Pooled throughput across all 2 verified pairs: direct **5108.4 ops/s**, Weir **2826.4 ops/s**; Weir/direct **0.5533**, Weir throughput change **-44.67%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
