# Weir throughput comparison

Backend: **mongo**. Started: 2026-10-03T03:07:17.345058Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 10000 operations (9029 reads, 971 writes), 8 workers, 1024 pre-created records and 1024 padding bytes per document. Warmup: 1000 operations; paired rounds: 3.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 10000 | 0 | 0 | 0 | 9189.9 | 0.095 | 7.679 | 8.191 | true |
| 1 | direct / weir | weir | 10000 | 0 | 0 | 0 | 5212.1 | 0.703 | 7.679 | 9.215 | true |
| 2 | weir / direct | direct | 10000 | 0 | 0 | 0 | 9188.4 | 0.103 | 7.167 | 8.191 | true |
| 2 | weir / direct | weir | 10000 | 0 | 0 | 0 | 5210.8 | 0.703 | 7.679 | 9.215 | true |
| 3 | direct / weir | direct | 10000 | 0 | 0 | 0 | 9148.3 | 0.095 | 7.679 | 8.191 | true |
| 3 | direct / weir | weir | 10000 | 0 | 0 | 0 | 4971.8 | 0.767 | 7.679 | 9.215 | true |

Pooled throughput across all 3 verified pairs: direct **9175.5 ops/s**, Weir **5129.0 ops/s**; Weir/direct **0.5590**, Weir throughput change **-44.10%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
