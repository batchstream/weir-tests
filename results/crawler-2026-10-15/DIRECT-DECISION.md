# Direct connection diagnostic control

Declared before dispatch on 2026-10-15. The proxy-based primary and cap experiments are retained in full. Response-copy aborts were omitted from their HTTP observer ledger, so their failures cannot be attributed to Weir core from those artifacts.

Run one final diagnostic at 80 offered products/s, two-step profile, 32 workers across four client processes, page size 20, 10 s warmup and 20 s measured arrivals, three rounds. Both Weir paths use c3101c6628872345d6fb09b977ce853f25936b79: default max_in_flight=0 and explicit 16. Include the native atomic Painless path. Use client d799170c06a625a400f1e32d9203b3fb816b463d with http-observer=disabled on all paths. Leave 1 database CPU, 512 MiB heap, 1536 MiB container memory, 10 s request deadlines, retry counts, refresh, durability and acceptance limits unchanged.

Direct connection means HTTP request/byte counters are unavailable, not zero. CPU, index statistics, acknowledged completion, query visibility and final-field oracle checks remain required. Preserve backend logs and cleanup receipts. If failures persist, report them without modifying production Weir or business repositories. If they disappear, do not retroactively repair the proxy runs or generalize this single offered rate to maximum capacity.
