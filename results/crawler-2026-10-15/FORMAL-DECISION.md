# Formal fixed-rate run decision

Decision recorded on 2026-10-15 before dispatching the formal run. Keep all three calibrated offered rates, 20/80/160 products/s, including the unfavorable results. Use the existing two-step rules and three query-visible phases on every path. Run only fixed-rate arrivals with 32 workers across four client processes, 20 products/page, 10 s warmup, 20 s arrivals, three rotating rounds, one database CPU, the unchanged 512 MiB Elasticsearch heap and unchanged 10 s business deadline. The server execution cap remains zero.

The initial e391cb59 calibration is an incomplete diagnostic, retained in full. Its 128-worker fixture setup sent an 81.5 MiB request and tripped a database memory breaker. The 32/128-worker closed-loop cases also exceeded the business deadline during warmup. They are not sustainable capacity results. No timeout or resource limit is relaxed to make a comparison pass.

Client source 2ef2675d2acb24308d1d7bd2dd798c6826ad7b83 fixes initialization request framing to <=8 MiB with strict item acknowledgements and makes modes explicit. Reducing the fixed-rate worker set to 32 avoids 128-worker seed/reset overhead and is sufficient to keep multiple page operations active at the selected rates. This changes the controlled fixture, so the earlier absolute CPU/rate values are not pooled with this run. Only same-run, same-round native/current/optimized paths are compared.

No business-call refactor, business repository edit or production Weir edit is part of this formal test. Report every stage and any failure; a high rate that misses the predeclared p99/drain/all-arrivals limits remains an overload observation. No additional tuning is planned merely to obtain a favorable Weir result.
