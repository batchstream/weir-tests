# Execution-cap test decision

Recorded before receiving formal results on 2026-10-15. Add one same-run configuration control at the middle offered rate, 80 products/s, with the same existing rules, 32 workers, four OS clients, 10 s warmup, 20 s arrivals and three rotating rounds. Keep the database at 1 CPU/512 MiB heap and the business deadline at 10 s.

Both Weir paths use exactly c3101c6628872345d6fb09b977ce853f25936b79. The current path keeps the default unlimited executions; the optimized path alone sets max_in_flight=16. Identical server source/binary isolates the configuration change. The native path remains a same-run control. This tests the new Weir execution limit without business or production source changes. Keep every negative result. Compare cap=0 and cap=16 within this runner; do not compare raw CPU/rates across the two formal runners.
