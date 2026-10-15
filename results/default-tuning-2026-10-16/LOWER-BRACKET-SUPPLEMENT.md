# Prospective lower-rate capacity bracket supplement

Registered after all selection evidence was retained and the unchanged selection
rules chose 128 operations / 4 MiB / 20ms / 16, before any independent confirmation
was dispatched. This adds cases; it does not replace the original 210-stage
confirmation inventory, retune defaults or relax any evidence/SLO gate.

The one-CPU selection native partial path completed about 229 updates/s and
rejected half the offered 512/s in the selected configuration's paired case.
The native two-step point completed about 62 products/s including drain at about
90% of one CPU. These observations suggest that the original quarter-CPU lower
endpoints (128 updates/s and 34 products/s) may already overload the baseline.
Linear quota scaling is only a range-selection heuristic, not a capacity claim.

Add two independent case-sets, each retaining its entire ladder on one runner:

- `confirm-partial-lower-q025`: rates 32, 64, 128 and 256 updates/s; partial update,
  16 KiB seeded documents, application-acknowledged completion.
- `confirm-crawler-lower-q025`: rates 8, 17 and 34 products/s; two-step Lua,
  query-visible completion, original unpadded seeded domain.

Both use actual omitted defaults versus fragmented native single-record calls,
four OS processes, 512 total workers, observed 0.25 database CPU, three rotating
paired rounds, 60s warmup and 60s arrivals. They add 24+18 timed stages, 84 runner
minutes; the twelve-set combined inventory is 252 stages / 420 runner minutes.
The longest declared warmup-plus-timed case-set remains 72 minutes.

All original loads and negative outcomes remain mandatory and visible. The same
all-arrivals, p99 <=5s, drain, generator, CPU coverage, host/client headroom and
completion-stability rules apply. A rate bound uses points from its own audited
workflow and quota; results from different runners are not spliced into a bound.
The previously registered soak tie policy considers these complete ladder groups
and still permits at most one check per workload, at most three total.
