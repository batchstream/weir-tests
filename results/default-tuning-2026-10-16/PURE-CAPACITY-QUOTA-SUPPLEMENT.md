# Prospective one-eighth-CPU pure-write capacity supplement

Registered after all fourteen confirmation workflows completed and the corrected
analysis retained 549 rows. The original 501-row analysis, corrected analysis,
all failed or unavailable points, and the zero-check soak derivation remain
evidence. This is a new fixed-quota study, not a replacement or favorable rerun
of any 0.25-CPU or one-CPU case. It does not retune the selected defaults.

The input analysis is
`results/default-tuning-2026-10-16/analysis-confirmation-parser-fix/analysis.json`,
SHA256 `24f252f3f172e39116bbd7bbcd36ae44d55b819e30a9ecb3147bb6e71290a66f`,
using analysis source `175272e0e8479cea6f2a03c215e285910e466376`.

At 0.25 CPU, Weir's 512-worker points consume 99.1--99.5% of database quota with
61.0--70.8% host busy. The 1024-worker points consume 99.1--99.4% of database
quota with 76.6--80.1% host busy, but the 512-to-1024 gains are 11.1%, 40.6% and
9.0%; the third 1024-worker round has a second/first-half completion ratio of
0.864. These fail the existing capacity gates. At one CPU, 512-worker Weir points
already have 94.8--95.8% host busy while the database consumes only 0.324--0.358
CPU cores. Increasing concurrency there would not establish database capacity.

The frozen single-request client also bounds concurrency at 1024. Rather than
change that Go client or assert host saturation is database saturation, add one
independent case-set, `confirm-pure-q0125`, with exactly one case:

- case ID `default-pure-q0125`, Search full-document pure writes;
- fixed observed database quota 0.125 CPU, four independent OS clients;
- total concurrency ladder 256, 512 and 1024, with three alternating paired rounds;
- 60s warmup and 60s timed per path, native single-record calls versus actual
  omitted Weir defaults, with no client batching;
- 8192 seeded records, 1024 payload bytes, 100% writes, no Lua mutations;
- selected defaults 128 operations / 4194304 bytes / 20ms / 16 in flight;
- unchanged server `084572d79b7a53230801297f90ca2363fd864913` and Go source tree,
  Search refresh/durability, fixture ownership, original auditor and observations.

This is 18 timed stages and 36 minutes of declared warmup plus timed work on one
runner, excluding setup and postflight. All source, effective controls, quota,
CPU coverage, headroom, full-field postflight, operation accounting, p99 <=5s,
second-half stability and adjacent-concurrency <=10% gain gates remain unchanged.
Both adjacent points must pass in all three rounds. No bound is assembled from
different quotas or workflows. Lowering quota may improve host headroom; that
is an experiment-design hypothesis, not a throughput prediction or capacity
claim. The lower quota can also lengthen native calls or warmup; errors and
timeouts remain failures and are retained without retries.

The initial 0.25-CPU native 128-worker point completed about 135/s versus 578/s
and 625/s in later rounds. Weir's first 128-worker round also improves strongly
within its timed window. This is compatible with cold-state or JVM warmup
effects, but JIT compilation was not measured and is not established as the
cause. Later-stage variability, including the failing 1024-worker third round,
cannot be excused as startup. The new study retains every full timed interval
and ten-second bucket: no retrospective burn-in removal, adaptive warmup,
restarting until warm, favorable rerun, or relaxed stability rule is permitted.

Before dispatch, the soak derivation may narrowly accept the additional pure
parent named above only when its plan metadata identifies this document's fixed
commit and SHA256 and its entire case shape matches this registration. The
original `POLICY` dictionary stays byte-for-byte unchanged. Qualified pure points
from the original two quotas and this additional quota enter the same existing
workers/quota/label/case-ID tie order. At most one pure five-minute check and at
most three total checks remain possible; no qualified point means no check.
An eligible pure check retains its paired native path, 60s warmup, 300s timed,
one round and all unchanged five-minute gates, with no favorable retry.

The client source and plan hash must be frozen after this document, and an
independent review must pass before dispatch. A passing result supports only the
declared workload, quota, client ladder and observed host. It does not establish
a universal maximum, a one-CPU maximum, or a production limit. If no adjacent
pair and subsequent sustained check qualify, report that no scoped maximum was
established by this finite study.
