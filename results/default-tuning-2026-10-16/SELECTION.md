# Default selection sensitivity study

Selected combination: **128 operations / 4 MiB / 20ms / 16 adapter invocations**. This is the constrained choice within the declared sensitivity inventory. Independent actual-default confirmation and its separate capacity qualifications are reported in the [complete evidence](README.md).

Twenty count/wait/slot configurations were measured at the 8 MiB byte anchor across three write profiles and two read guards. A separate four-point 64 KiB body calibration used the 128/5ms/16 anchor. The independently confirmed combination must pass the registered joint large-body check. This is not a claim that the full cross-product or every deployment is globally optimal.

All 104 cases and 288 stages remain in the inventory. The one-slot query-visible case failed during warmup and is ineligible; all later cases continued. Other SLO and generator failures remain in the ranking. No failed point was rerun.

The first analysis mistakenly iterated unavailable Elasticsearch metrics for Mongo. Its original output and manifest remain in `analysis-selection/`; the corrected parser is frozen at `14a3cb6f70ddfbd70e4acf8b300a68a12523b9ef`. Candidate inputs, rules and thresholds were unchanged, and the real Mongo raw regression test now passes. Independent review reproduced all 288 corrected rows, ranking and byte choice.

## Count/wait/slot ranking

| Configuration | Eligible | Throughput score | CPU score | Exclusion |
| --- | --- | ---: | ---: | --- |
| c128-w5ms-s1 | False | unavailable | unavailable | missing or unqualified write profile |
| c128-w0s-s16 | True | 0.824933 | 1.169733 |  |
| c256-w10ms-s4 | False | unavailable | unavailable | missing or unqualified write profile |
| c128-w5ms-s64 | True | 0.948160 | 1.184338 |  |
| c128-w1ms-s16 | False | unavailable | unavailable | missing or unqualified write profile |
| c128-w5ms-s8 | False | unavailable | unavailable | missing or unqualified write profile |
| c32-w5ms-s16 | True | 0.832059 | 1.188364 |  |
| c32-w0s-s0 | False | unavailable | unavailable | unlimited legacy execution control is not a protective default; missing or unqualified write profile |
| c256-w5ms-s16 | True | 0.985499 | 1.068137 |  |
| c128-w10ms-s16 | True | 0.971548 | 1.073347 |  |
| c64-w1ms-s4 | False | unavailable | unavailable | missing or unqualified write profile |
| c128-w5ms-s4 | False | unavailable | unavailable | missing or unqualified write profile |
| c64-w5ms-s16 | True | 0.959279 | 1.096483 |  |
| c128-w5ms-s16 | True | 0.977675 | 1.106212 |  |
| c128-w2ms-s16 | True | 0.899324 | 1.155238 |  |
| c128-w20ms-s16 | True | 0.985317 | 1.012701 |  |
| c128-w5ms-s32 | True | 0.992128 | 1.115677 |  |
| c256-w1ms-s32 | True | 0.875482 | 1.174365 |  |
| c64-w10ms-s32 | False | unavailable | unavailable | missing or unqualified write profile |
| c512-w5ms-s16 | False | unavailable | unavailable | missing or unqualified write profile |

Eleven protective candidates were eligible, seven entered the 5% throughput frontier and only `c128-w20ms-s16` remained in the CPU plateau. All four Search/Mongo read guards passed for this point.

## Byte calibration

| Cap bytes | Completed writes/s | Actual DB CPU ms/write |
| ---: | ---: | ---: |
| 262144 | 649.147 | 1.512475 |
| 1048576 | 932.761 | 1.049398 |
| 4194304 | 1073.341 | 0.902878 |
| 8388608 | 1102.689 | 0.898235 |

4 and 8 MiB both met the joint 5% plateau; the frozen tie rule selected the smaller 4 MiB target. The target histogram contains every actual bulk attempt. Higher-cap observations exceed smaller targets, providing explicit binding witnesses; the largest target has no higher-cap witness in this finite study.

## Limits

These short selection stages choose a candidate and do not establish steady state or maximum capacity. The original native partial512 point did not complete all arrivals and cannot support a qualified paired saving claim. The complete evidence retains the independent three-round actual omitted-default results, all 12 passed read guards, the combined large-body check and every failed capacity qualification. Conditional five-minute checks remain subject to the original eligibility gates.
