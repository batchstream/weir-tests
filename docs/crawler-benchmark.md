# Crawler mutation benchmark

This explicit experiment measures database work per completed business product and completed throughput under a fixed database CPU quota. It does not compare total infrastructure cost. Sink is the legacy service that Weir replaces and is not a benchmark path.

The three same-runner paths are native Elasticsearch, current Weir, and optimized Weir. All three receive identical generated product and seller documents, use identical durable completion and query visibility, and share the same exclusively owned Elasticsearch container. The native path uses cached, atomic server-side Painless update scripts in bulk requests, following the same phase boundaries as PSE. It does not use an unnecessarily weak client-side read/replace cycle.

## Business data and semantics

The frozen rules come from PSE product/offer merges (`7f36b501cded06a1bcc7484575346bb9c4d6311b`), with the combined reconciliation change in `8725e4743c6ddba66132874e0e0fe9d393ab4a85`. The Weir primitive translation started from the existing independently tested PSE rules at `283940ef442b479440e45b39c86988351b92d7b5`. Every Lua source and the native script has a SHA-256 in the report. The benchmark imports no private PSE data or deployment settings.

One business product comprises one product document and two associated seller documents. A default page contains 20 business products. Product documents contain descriptions, prices, country/language unions, three histories initially containing 20 entries, existing offers and new offers. Seller documents contain descriptions, addresses, contacts and tracking ID unions. Worker-owned keys change between two input variants, preventing warmup from turning the measured workload into unchanged writes.

One existing shared seller receives an additional placeholder mutation when `(worker + page) % 100 == 0`. This small shared-key component is explicitly synthetic. It is not an estimate of production key popularity. Other seller documents keep changing throughout the stage, so an automatic no-op visibility barrier is measured in an actively written seller index.

The product and seller data use separate indices, each with one primary shard, zero replicas, `translog.durability=request` and `refresh_interval=1s`. Weir mutations request `QUERY_VISIBLE`; native bulk requests use `refresh=wait_for` and wait for the active primary. Native `retry_on_conflict=3` and both Weir `retry_attempts=4` allow the same four total attempts; ambiguous writes are never automatically replayed by the harness. A page counts as successful only after every product and seller mutation in that page has completed with an application and visibility acknowledgement. A partial page is conservatively counted as failed. It cannot contribute to a qualified comparison.

The generated fixture exercises incoming string replacement, Unicode-safe brand normalization on the supplied ASCII brand, scalar replacement, true/false normalization, history deduplication/caps, incoming offer priority, retained expired offers, stable observation time, UID unions, seller histories, tracking unions and existing-seller placeholder protection. Native Painless implements these exact generated values and rules. This is not a claim that Painless implements arbitrary Lua, or that ungenerated date formats, hostname combinations, nullable values and every production branch have been benchmarked. All fixture URLs use one hostname; URL mapping is absent, so its guard cannot change any generated document.

The independent Go domain oracle reconstructs every seeded document, including untouched and rejected keys, and compares all fields and JSON value types with a normal Elasticsearch search. It checks the expected product/seller index as well. The report stores the verified document count, canonical expected-document hash and acknowledged page ordinals. `audit` reconstructs that hash offline from the page ordinals. Full expected document bodies are discarded after successful verification to keep multi-stage artifacts bounded.

## Workload profiles

`-profiles two-step` is the default and preserves the existing detail-worker reconciliation followed by product and seller updates. The current experiment only tests Weir: PSE is a read-only source of business rules. No business repository changes or migrations are part of this experiment.

`-profiles combined` is an optional controlled workload where every path runs the same single product reconciliation-plus-merge rule. It does not describe a deployed PSE optimization. Differences between current and optimized Weir can be attributed to the server/configuration comparison; they cannot be attributed to combining two PSE calls.

`-profiles combined,two-step` adds an explicit business-call ablation. The two-step profile first reconciles product offers and waits for query visibility, then performs the normal product and seller calls, each with its own visibility wait. The final independent oracle is identical. Both native and Weir follow the PSE page call shape: products become visible first, then sellers become visible. The two-step profile adds a separate query-visible product-reconciliation call before those two phases, making three phases. The 20-product page is a controlled approximation of a PSE page; the additional reconciliation models the detail-worker change and is not a claim that every PSE page formerly reconciled offers. Compare profiles on the same optimized server to assess removing a product read/merge/write and its visibility wait. This is a separate contribution from the server change.

`-after-max-in-flight 0` keeps adapter concurrency unlimited. Values such as 4 or 16 are explicit experiments and apply only to optimized Weir; the old server never receives an unknown field. `QUERY_VISIBLE` can hold a slot until a refresh completes. A small slot cap may reduce throughput even when the database CPU is idle. The experiment records this outcome rather than assuming the cap improves throughput.

Automatic unchanged-document detection can avoid replacing an existing seller while still issuing an explicit `_refresh` barrier to satisfy query visibility. Compare physical `_refresh` requests, native refresh count/time, merge time, actual index/get work, request/response bytes and database CPU together. Fewer indexed documents alone do not establish a reduction in database load.

## Arrival and completion measurement

Four independently started OS clients each own a driver/SDK pool and multiple goroutine workers. Pools remain open across warmup, a deterministic document reset, and the timed phase. After both the initial reset and the post-warmup reset, the harness waits for both owned indices to report no current indexing or merging, and unchanged cumulative merge/flush totals and times for at least two consecutive seconds. It samples once per second, fails after 30 seconds without settling, and retains timestamped raw index statistics and wait durations for both waits in each stage. These waits are outside measurement; no force merge is issued. Every path and round receives the same fixture. Paths rotate `ABC`, `BCA`, `CAB` between rounds/cases. The same database survives server binary replacements; server identity and PID are recorded per stage. Settling removes observable ongoing reset work, but cannot make persistent segment history identical or prevent later background maintenance. The retained segment/merge counters and rotating path order expose this remaining limitation.

Fixed-rate experiments schedule page arrivals from absolute timestamps, staggered across processes. Worker completions do not throttle the generator. Each worker has a bounded page queue; a full queue produces an explicit rejected product count. Generator lag, dispatch lag, queue peak, errors, timeouts and uncertain acknowledgements are retained. Latency starts at the planned arrival, includes client queueing and any server aggregation, and ends only after all page mutations are visible. Page latency is also each product's business-completion latency. Histograms include every dispatched page, including failed/timed-out pages; rejected arrivals have no visible completion and are reported separately rather than assigned a synthetic latency. Overloaded stages cannot claim a sustainable arrival rate or a successful-all-arrivals p99. Quantiles use the existing inclusive logarithmic histogram bounds (at most 12.5% plus 1 microsecond bucket approximation).

Saturation experiments use continuously busy sequential workers. Their bounded client queue is still included in planned-to-visible latency. They are not open-loop rate experiments. The complete arrival interval plus every subsequent completion join/drain remains in the throughput denominator. Completions after the arrival interval are counted separately; neither admission nor queueing is counted as success. Warmup is excluded and every seeded record is reset before measurement without replacing the client pools.

Select three offered rates only after a short calibration: below, near and above the observed stable capacity. A high rate with rejections is an overload result, not a higher successful capacity. A claimed sustainable rate needs all three rounds without business failures, unknown writes, rejected arrivals or substantial generator lag, plus a declared latency/drain requirement. Do not infer a production capacity from a short preflight.

## Database observations and evidence

An owned loopback transparent HTTP observer sits between every path and the same database. It records actual physical methods/endpoints, request bodies and response bodies, with monitoring traffic labelled separately. HTTP byte counters exclude headers/framing. They measure database-facing traffic, not Lua/SDK upload bytes. Both the native client and observer disable automatic HTTP compression, matching the Weir Search transport and preventing the observer from introducing database gzip work. The observer uses the same bounded transport for all paths; its extra loopback hop is an experiment limitation.

Owned Docker cumulative cgroup counters provide database CPU, container network bytes, block I/O bytes and memory samples. The inspected CPU quota is retained. Elasticsearch `_stats` retains indexing/get/search/refresh/flush/merge/translog counters before and after the timed workload. Missing counters stay unavailable rather than becoming zero. Block bytes and filesystem counters do not prove I/O saturation.

`summarize_crawler.py` recomputes sampled database CPU per completed product and budget utilization using valid cumulative-counter intervals clipped to the stage. Sampling coverage is emitted alongside each cost. Treat short-stage or poor-coverage CPU figures as unavailable for conclusions; formal stages should be at least 20 seconds and have at least 80% CPU coverage. The 50 ms generator-lag flag in its CSV is an explicit reporting threshold, not a production SLO. Raw data retains every counter and latency bucket, including failed or overloaded stages. The summarizer does not discard those rows. Its qualification also requires a complete report without cleanup errors. Run the offline auditor first and retain the runner receipt; the CSV is an observation table, not a substitute for source/receipt and full-field validation.

Immutable server preparation receipts contain the full Git commits, clean source state, binary SHA-256, version identity and Go build metadata. The runner records the exact command that it executes, client source/clean state, host/kernel/CPU, pinned database image, workflow URL and binary hashes before/after execution. Only newly owned loopback fixtures start, and cleanup receipts retain ownership and removal evidence. Ordinary unit tests remain offline and never start these services.

## Run

Build exact public source commits without modifying the harness dependency lock:

```sh
python3 scripts/prepare_crawler_servers.py \
  --current 4fe287e5915766ec3f48c8f42abde6b884d89bdd \
  --optimized c3101c6628872345d6fb09b977ce853f25936b79
go build -trimpath -buildvcs=false -o .tools/weir-crawler ./cmd/weir-crawler
```

Use the manual `Crawler database throughput` workflow for Linux Docker measurements. Start with the existing two-step rules, one round, 5-second arrival stages, a short warmup and a small matrix. Fix calibrated rates before the three-round 20-second formal matrix. The current experiment does not include a business-call refactor ablation.

A small local functional run (not publishable performance evidence):

```sh
.tools/weir-crawler -concurrency-levels 4 -client-processes 2 \
  -page-size 2 -profiles combined,two-step -rates '' \
  -rounds 1 -warmup 1s -duration 1s -output results/local/crawler-preflight.json
```

Offline arithmetic and expected-field reconstruction, followed by CSV/JSON observations:

```sh
.tools/weir-crawler audit results/local/crawler.json
python3 scripts/summarize_crawler.py --report results/local/crawler.json \
  --output results/local/crawler-observations
```
