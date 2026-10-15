# Observed capacity variability in two archived rounds

This retrospective inspection uses only the two completed, archived pure-write
experiments below, each at 1024 workers, round 3, path `weir`. It does not inspect
the running wait-sensitivity experiments or change any qualification rule.
Both use server `084572d79b7a53230801297f90ca2363fd864913`, four OS clients,
60 seconds warmup and 60 seconds timed traffic.

| Case set | Client source | Workflow | Approximate timed interval, UTC on 2026-10-15 |
| --- | --- | --- | --- |
| `confirm-pure-q0125-startup-fix` | `b934c20ab3d09d6d2aa5bcdd0e6fac7ecfac26b5` | [38801128626](https://github.com/batchstream/weir-tests/actions/runs/38801128626) | 21:01:42.586020–21:02:42.684979 |
| `confirm-pure-q025` | `682e725df1a81e29f4d4983d8711df1989c4adad` | [38781896048](https://github.com/batchstream/weir-tests/actions/runs/38781896048) | 18:40:44.081325–18:41:44.179376 |

The interval starts are reconstructed from the first resource sample's UTC
timestamp minus its `elapsed_ns`, then extended by the stage's `elapsed_ns`;
the displayed UTC values are rounded to microseconds. Completion and CPU-half
values come from the unchanged audited
[`analysis-startup-corrected-confirmation/analysis.json`](analysis-startup-corrected-confirmation/analysis.json).
Completion halves use the six regular 10-second buckets, excluding the final
drain bucket. GC, throttling and block-I/O comparisons use differences between
cumulative resource samples nearest 0, 30 and 60 seconds, without interpolation:
0.006492 / 29.704267 / 59.805127 seconds for q0125 and
0.006356 / 30.410549 / 59.118946 seconds for q025. These resource intervals are
therefore approximate halves, not exactly the completion windows.

## Three observations

1. **The late throughput decline is measured despite sustained database CPU.**
   Completion rates fall from 6374.53 to 5606.37 records/s at q0125
   (second/first half 0.879494), and from 6389.77 to 5521.07 at q025
   (0.864048). Both have their lowest regular bucket at 40–50 seconds:
   5089.2 and 4751.3 records/s respectively. Database CPU remains
   103.92%→103.64% and 99.45%→99.43% of the allocated quota. The corresponding
   Elasticsearch nodes had already been running for approximately 37.48 and
   37.39 minutes when these stages began, so this cannot simply be attributed
   to the Elasticsearch process having just started. This does not establish
   that each freshly prepared dataset had reached steady state.

2. **The sampled young-GC counters do not support increased late GC as the
   explanation.** Approximate-half young-GC time is 332→46 ms at q0125
   (8→7 collections), and 163→174 ms at q025 (8→8 collections). Old and
   concurrent collector counts and times have zero timed-window increments
   in both cases. The corresponding process logs contain no GC-overhead
   message during these timed intervals. These counters do not prove that
   GC had no effect; they provide no evidence for a late young-GC increase
   sufficient to explain the observed decline.

3. **Quota throttling persists, while sampled rejection pressure remains low.**
   Throttled periods account for 100%→100% of periods at q0125 and
   94.41%→94.08% at q025. Sampled write-pool queue maxima are 11 and 13;
   write-pool and all recorded indexing-pressure rejection increments are
   zero. Peak indexing-pressure memory is only 2.48% and 2.38% of its limit.
   Mean host busy is 44.36% and 77.13%; mean host I/O wait is 0.164% and
   0.098%. These are aggregate host observations, not proof of absent
   transient contention. I/O wait is not disk utilization. Container block
   write rates decline approximately 7.41→6.97 and 7.48→7.06 MiB/s, which
   could itself be a consequence of lower completed throughput.

The evidence establishes variability in CPU-limited throughput and failure of
the predeclared stability gate. It does not identify a causal mechanism.
The before/after index snapshots retain whole-stage merge and refresh totals:
q0125 has 4 merges / 13099 ms and 39 refreshes / 20485 ms; q025 has 6 merges /
20988 ms and 38 refreshes / 21501 ms. These aggregate elapsed counters cannot
locate activity within the falling half-window or assign CPU cost. The resource
samples lack corresponding merge/refresh/indexing-stage timing series and disk
latency/utilization; the logs do not resolve them. No controlled intervention
isolates their effects. Neither
60 seconds of warmup nor a single 60-second measurement proves steady-state
capacity here; retain these failed rounds and use the separately registered
longer-window experiment and sustained-check rules without exceptions.

## Raw evidence

All paths below are relative to this document; each cohort's `files.json`
records the archived file hashes. The selected stage is
`pairs[round=3, concurrency=1024].weir`; resource evidence is under
`resources.samples[].database_counters.search_node_stats`,
`database_cpu_throttling`, `container_block_write_bytes` and `host`.

- q0125 [report](raw/confirm-pure-q0125-startup-fix/artifacts/batching-tuning-b934c20ab3d09d6d2aa5bcdd0e6fac7ecfac26b5-confirm-pure-q0125-startup-fix/home/runner/work/weir-tests/weir-tests/results/local/batching-tuning/default-pure-q0125-startup-fix/measurement/search.json.gz),
  [process log](raw/confirm-pure-q0125-startup-fix/artifacts/batching-tuning-b934c20ab3d09d6d2aa5bcdd0e6fac7ecfac26b5-confirm-pure-q0125-startup-fix/tmp/weir-tests-278932455/search-process.log.gz),
  [file hashes](raw/confirm-pure-q0125-startup-fix/files.json).
- q025 [report](raw/confirm-pure-q025/artifacts/batching-tuning-682e725df1a81e29f4d4983d8711df1989c4adad-confirm-pure-q025/home/runner/work/weir-tests/weir-tests/results/local/batching-tuning/default-pure-q025/measurement/search.json.gz),
  [process log](raw/confirm-pure-q025/artifacts/batching-tuning-682e725df1a81e29f4d4983d8711df1989c4adad-confirm-pure-q025/tmp/weir-tests-2654452866/search-process.log.gz),
  [file hashes](raw/confirm-pure-q025/files.json).
