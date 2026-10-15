# Prospective finite collection-wait sensitivity

Registered after the corrected 0.125-CPU confirmation and before any measurement
in this supplement. The original selection ended at 20 ms. Pure-write batches
are already close to 128 records, but the partial-update and dependent crawler
profiles have smaller batches, so they can still expose a collection-window
tradeoff. This study extends only that boundary; all earlier outcomes remain.

## Fixed inventory and sources

Use Weir `084572d79b7a53230801297f90ca2363fd864913` throughout. Hold count at
128, the byte target at 4194304 and simultaneous adapter executions at 16.
Measure exactly **20, 40, 80 and 160 ms**, all as explicit four-field batching
configurations. Do not change the server's 20 ms default during this study.

The measurement Go reference is the admission-only extension
`f06a346a8e0f37f0f8b2ae6539d9a7ce6dff5b27`. Its measurement operations,
verification and cleanup remain those of the existing crawler engine. Freeze
the exact dispatch/client/auditor sources, source trees and binary receipts
before dispatch. Python audit/source-registration changes do not alter the
measured Go tree.

The parent is `plans/default-confirmation-2026-10-16.json` at
`682e725df1a81e29f4d4983d8711df1989c4adad`. Copy the corresponding original
one-CPU case, preserving its workload, padding, document-ID generation,
completion semantics, deadline, page shape and observer policy. Change only
the case identifier, the explicit collection wait and paths to `optimized`
only; the anchor is measured again rather than borrowed from an earlier run.

| Case set | Parent case set | Profile and offered rate | Completion | Untouched padding | Fixed case order, ms |
| --- | --- | --- | --- | --- | --- |
| `wait-partial-q1` | `confirm-partial-q1` | `partial-update`, 512 updates/s | `application_acknowledged` | 16384 bytes | 20, 80, 40, 160 |
| `wait-crawler-q1` | `confirm-crawler-q1` | `two-step`, 68 products/s | `query_visible` | 0 bytes | 160, 40, 80, 20 |

Case IDs are `wait-partial-q1-w{wait}ms` and
`wait-crawler-q1-w{wait}ms`. Each of the eight cases creates its own fresh,
exclusively owned fixture; its three rounds retain the existing reset and
settling lifecycle. Every stage has 60 seconds fixed warmup and 60 seconds
timed arrivals, one database CPU, 512 workers across four OS clients, one
record per call, page size one, one queued page per worker, a 10-second
operation deadline and the HTTP observer disabled. There is no native path in
these two Weir-to-Weir sensitivity cohorts. Native comparisons remain in the
independent confirmation and are not inferred from these results.

Each case set contains four cases and **12 timed stages / 24 minutes** of
warmup plus measurement, excluding bounded preparation, settling and drain.
Together they contain exactly 24 timed stages. The manifest entry phase and
plan metadata phase are both `wait_sensitivity`, separate from original
selection, confirmation and capacity/soak derivation. No additional waits,
rates, favorable retries, trimmed intervals or replacement cases are allowed.

## Eligibility and finite choice rule

Use the unchanged source, command, owned-configuration, binary, cleanup and
full persisted-field audits. Every candidate needs all three rounds in both
profiles. Preserve every failed/missing round and its reasons. Required gates:

- complete source/receipt and four-control runtime evidence; all original
  clients exit successfully and all seeded fields pass independent verification;
- zero rejected, failed, unknown or timed-out arrivals, with offered equal to
  completed: 30720 updates or 4080 products in each respective timed stage;
- at least 95% observed database CPU coverage;
- original host/client headroom: at least 90% coverage, host busy at most 85%,
  steal at most 5%, and four owned clients each below 80% of effective host CPUs;
- reported all-arrivals p99 upper bound at most 5 seconds, drain at most
  3 seconds and maximum generator lag at most 50 ms;
- the original completion/backlog stability calculation passes, including
  second-half completion rate at least 90% of the first half.

Database saturation is not required for this fixed-arrival CPU comparison.
The separate maximum-capacity rules remain unchanged. For each eligible
candidate, compare each profile/round with the corresponding newly measured
20 ms profile/round. Completed throughput must be within the inclusive
0.95–1.05 band in every comparison. Use database CPU milliseconds per completed
logical unit, retaining updates and products as separate units.

Let each of the six CPU ratios be candidate CPU/unit divided by anchor
CPU/unit. A longer wait can replace the anchor only if:

1. No individual profile/round CPU ratio exceeds 1.05.
2. In **every** round, the geometric mean of that round's two profile CPU
   ratios is strictly below 0.95, a reduction greater than 5%.

The candidate aggregate CPU score is the geometric mean of all six ratios.
Keep the qualified 20 ms anchor eligible with score 1. Among eligible waits,
find the smallest aggregate score, retain scores within its inclusive 1.05
CPU band, and select the shortest wait. This uses the original selector's
5% near-best CPU tolerance, with the additional three-round improvement and
per-profile protection above. A longer candidate passing the strict 0.95
rule has aggregate score below 0.95, so a qualifying improvement cannot be
cancelled by treating the anchor's score 1 as inside that candidate's 1.05 band.

If the anchor lacks any required round or gate, return **inconclusive** with
no proposed replacement. If the anchor qualifies but no longer wait qualifies,
retain 20 ms. Do not impute missing observations, pool logical units, replace
failed profiles, reinterpret CPU quotas as actual load, or select from one
favorable round. A separate pure selector output records all eligibility
failures, six ratios, per-round geometric means, aggregate scores, CPU band
and final choice.

## Interpretation and registration

This chooses a wait proposal within four declared waits and two fixed
workloads, not a universal optimum or maximum capacity. Any proposed default
change needs a separately frozen server revision and actual omitted-default
confirmation; this supplement alone does not modify count, bytes, concurrency
or production behavior. It does not expand eligibility for a capacity soak.

The generated plan is `plans/default-wait-sensitivity-2026-10-16.json`.
Its metadata must name this document's full committed source and SHA-256,
the pure selector's committed source and SHA-256, the immutable parent plan
and its SHA-256, the measurement Go reference, the anchor controls and exact
inventory. Freeze the generated plan and selector before any dispatch; retain
workflow URLs, dispatch receipts, raw compressed artifacts and their hashes.
