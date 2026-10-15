# Fixed wait sensitivity result

The preregistered 20/40/80/160 ms study completed both query-visible crawler
and application-acknowledged partial-update cohorts: 24 timed stages across
eight owned fixtures. All 417,600 arrivals completed with zero failed, unknown,
timed-out or rejected operations; every completion p99 was at most 3.932159 s.
All original Go audits, full-field persisted oracles, source proofs and fixture
cleanup checks passed. Raw archives were committed at
`542df18c1d49c33bb8c576d05a4c481b569c48cd` before interpreting performance.

The [frozen selector output](wait-sensitivity-choice.json) is **inconclusive**,
with `selected_wait_ms: null`. The 20 ms crawler anchor's third round ended
with backlog 254, exceeding its original stability bound of 154 + 68 = 222.
The 160 ms crawler's first round drained in 3.17392354 s, exceeding 3 s.
Although the 40 and 80 ms rows meet their individual gates, the registered
comparison requires a qualified 20 ms anchor in every round. Their CPU values
cannot justify replacing the default, and this experiment does not prove 20 ms
beats the longer waits. There is no replacement anchor or favorable rerun.

The existing 20 ms default remains the earlier finite-grid choice; universal
optimality is unproved. The [full audit](analysis-wait-sensitivity/analysis.json)
retains all 591 rows, including the prior 567 rows and their unchanged selection
and formal-capacity decisions. See the [equivalence receipt](wait-sensitivity-equivalence.json).
The accepted completion bound remains p99 at most 5 s.
