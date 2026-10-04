These stress runs use separate runners from the primary matrix. Compare each row within its paired run; absolute QPS across tables is not controlled.

| Database | Write % | Direct req/s | Weir req/s | Change | Actual batch |
|---|---:|---:|---:|---:|---:|
| mongo | 0 | 10,469 | 18,117 | +73.0% | 12.05 |
| mongo | 10 | 5,097 | 11,544 | +126.5% | 9.78 |
| mongo | 100 | 2,906 | 6,581 | +126.4% | 9.54 |
| search | 0 | 10,365 | 15,429 | +48.9% | 9.88 |
| search | 100 | 1,548 | 9,713 | +527.3% | 12.33 |
| search | 10 | unavailable | unavailable | unavailable | unavailable |

Search mixed-load round 1 at concurrency 512 failed during native warmup: 52 operation timeouts (47 errors and 5 UNKNOWN writes), maximum 10.003122920 seconds. Unknown write effects remain unconfirmed. The earlier Weir timed stage completed 209,844 verified operations, but does not create a paired 512 ratio. All 19 completed Search timed paths remain individually verified; five planned timed paths are absent.

Overall stress evidence contains five complete backend reports plus one incomplete report, 139 individually successful timed paths and 17,575,059 successful timed operations. It is not a complete six-report PASS. No stress dataset establishes a paired database-saturated capacity ratio.
