# Crawler mutation benchmark

`cmd/weir-crawler` measures database work per completed business product and completed throughput for realistic crawler mutations under a fixed database CPU quota. It does not compare total infrastructure cost.

## Paths

Each stage runs one of three paths against the same exclusively owned Elasticsearch container:

- `direct`: native Elasticsearch. The default `single` input uses single-document `_update` calls with cached atomic Painless scripts.
- `before`: Weir from `-before` (default `.tools/weir`) with explicit 32-operation, 8 MiB, no-wait, unlimited in-flight batching.
- `after`: Weir from `-after` (default `.tools/weir`) with the `-after-batch-max-operations`, `-after-batch-max-bytes`, `-after-batch-max-wait` and `-after-max-in-flight` controls, or with no batching configuration at all when `-after-use-defaults` is set.

`before` and `after` can differ in binary, batching configuration or both. Use the same binary for both to isolate a configuration change, or the same configuration to compare two server builds. `-paths` selects a unique subset in rotating order. All paths receive identical generated product and seller documents and use identical durable completion and query visibility. Weir receives exactly one mutation per SDK `Mutate` call; batching happens inside the server. An explicit `page` input retains client bulk calls as a separate control. No path uses a client-side read/replace cycle.

The report records each Weir binary path and SHA-256. Every stage retains the owned routes and the server PID, and the auditor checks the declared batching controls against both the routes and the live before/after wait and in-flight gauges. With `-capture-effective-config`, each stage also retains the server's resolved configuration and the auditor checks all four live batching gauges.

## Business data and semantics

The Lua product and offer merge rules and the equivalent native Painless script are embedded in `internal/benchmark/crawlerdata`. Every Lua source and the native script has a SHA-256 in the report.

One business product comprises one product document and two associated seller documents. The default `-input-shape single -page-size 1` dispatches one business product at a time per goroutine. An explicit `-input-shape page -page-size 20` sends 20-product bulk pages; those results cannot stand in for a native single-record baseline. Product documents contain descriptions, prices, country/language unions, three histories initially containing 20 entries, existing offers and new offers. Seller documents contain descriptions, addresses, contacts and tracking ID unions. Worker-owned keys change between two input variants, preventing warmup from turning the measured workload into unchanged writes.

One existing shared seller receives an additional placeholder mutation when `(worker + page) % 100 == 0`. This small shared-key component is explicitly synthetic and is not an estimate of production key popularity. Other seller documents keep changing throughout the stage, so an automatic no-op visibility barrier is measured in an actively written seller index.

The product and seller data use separate indices, each with one primary shard, zero replicas, `translog.durability=request` and `refresh_interval=1s`. Weir mutations request `QUERY_VISIBLE`; native single-update and page-bulk requests use `refresh=wait_for` and wait for the active primary. Native `retry_on_conflict=3` and both Weir `retry_attempts=4` allow the same four total attempts; ambiguous writes are never automatically replayed by the harness. A page counts as successful only after every product and seller mutation in that page has completed with an application and visibility acknowledgement. A partial page is conservatively counted as failed.

The generated fixture exercises incoming string replacement, Unicode-safe brand normalization on the supplied ASCII brand, scalar replacement, true/false normalization, history deduplication/caps, incoming offer priority, retained expired offers, stable observation time, UID unions, seller histories, tracking unions and existing-seller placeholder protection. Native Painless implements these exact generated values and rules; it does not implement arbitrary Lua, and ungenerated date formats, hostname combinations and nullable values are not exercised. All fixture URLs use one hostname, so URL mapping cannot change any generated document.

The independent Go domain oracle reconstructs every seeded document, including untouched and rejected keys, and compares all fields and JSON value types with a normal Elasticsearch search against the expected product/seller index. The report stores the verified document count, canonical expected-document hash and acknowledged page ordinals; `audit` reconstructs that hash offline from the page ordinals. Full expected document bodies are discarded after successful verification to keep reports bounded.

## Workload profiles

`-profiles two-step` is the default: a reconciliation call followed by product and seller updates.

`-profiles combined` runs one product reconciliation-plus-merge rule on every path.

`-profiles combined,two-step` adds a business-call ablation. The two-step profile first reconciles product offers and waits for query visibility, then performs the normal product and seller calls, each with its own visibility wait. The final oracle is identical. In single input, reconciliation must acknowledge before the product update, and the product must acknowledge before its sellers start. The two regular sellers, plus an occasional shared placeholder seller, are submitted concurrently as separate one-record calls and joined before business success. The native connection limit is three times the goroutine count per process, so every potential seller can run without an extra native bottleneck. The page input runs products then sellers as separate bulk phases. Compare profiles on the same server to assess removing a product read/merge/write and its visibility wait.

`-profiles partial-update -input-shape single -page-size 1` isolates server batching from Lua and external read-modify-write. It must run alone and is never pooled with the product/seller results. Each independent caller sends one deterministic partial document to one existing product: native `POST /index/_update/id` with `{doc: ...}` versus one Weir `BackendExpression` with the identical body. There are no seller calls, client batches, Lua transforms or client-side reads. Two keys belong to each sequential worker; `batch_sequence`, title, prices and availability change deterministically so every operation changes its document. Both paths use request durability and **zero conflict retries**; the Search Lua retry setting does not apply to backend expressions. All seeded product/seller documents remain in the full-field oracle, including untouched fields. Its fixture-source hash describes the partial-update contract.

Partial updates can select `-visibility application_acknowledged` or `query_visible` completion and a seed-document padding size (`-seed-padding-bytes`). Native and Weir use the same patch and completion mode. Application-acknowledged stages perform an explicit, checked postflight refresh after timed CPU and database snapshots, before full search verification. The two-step and combined profiles always use query-visible completion. Observation rows label `logical_unit=partial_update`, and `ProductsPerSecond`/`CompletedProductsPerSecond` fields and product-named CPU/latency columns count **one partial-update operation** in this profile.

`-after-batch-max-wait 20ms` sets the after path's explicit `batching.max_wait` collection window; the default `0s` preserves immediate execution. Every stage stores `BatchMaxWaitNS`.

`-after-max-in-flight 0` keeps adapter concurrency unlimited. Values such as 4 or 16 apply only to the after path. `QUERY_VISIBLE` can hold a slot until a refresh completes, so a small slot cap may reduce throughput even when the database CPU is idle.

Automatic unchanged-document detection can avoid replacing an existing seller while still issuing an explicit `_refresh` barrier to satisfy query visibility. Compare physical `_refresh` requests, native refresh count/time, merge time, actual index/get work, request/response bytes and database CPU together. Fewer indexed documents alone do not establish a reduction in database load.

## Arrival and completion measurement

Independently started OS clients (`-client-processes`, default four) each own a driver/SDK pool and multiple goroutine workers. Each child records its input shape and actual native pool limit (three times its worker count for single input; zero for Weir, which does not use the native pool). Pools remain open across warmup, a deterministic document reset and the timed phase. After both the initial reset and the post-warmup reset, the harness waits for both owned indices to report no current indexing or merging, and unchanged cumulative merge/flush totals and times for at least two consecutive seconds. It samples once per second, fails after 30 seconds without settling, and retains timestamped raw index statistics and wait durations for both waits in each stage. These waits are outside measurement; no force merge is issued. Paths rotate `ABC`, `BCA`, `CAB` between rounds/cases. The same database survives server binary replacements. Settling removes observable ongoing reset work, but cannot make persistent segment history identical or prevent later background maintenance; the retained segment/merge counters and rotating path order expose this limitation.

Fixed-rate cases schedule page arrivals from absolute timestamps, staggered across processes. Worker completions do not throttle the generator. Each worker has a bounded page queue (`-queue-pages`); a full queue produces an explicit rejected product count. Generator lag, dispatch lag, queue peak, errors, timeouts and uncertain acknowledgements are retained. Latency starts at the planned arrival, includes client queueing and any server aggregation, and ends only after all page mutations are visible. Histograms include every dispatched page, including failed/timed-out pages; rejected arrivals have no visible completion and are reported separately. Overloaded stages cannot claim a sustainable arrival rate or a successful-all-arrivals p99. Quantiles use inclusive logarithmic histogram bounds (at most 12.5% plus 1 microsecond bucket approximation).

Saturation cases use continuously busy sequential workers. Their bounded client queue is still included in planned-to-visible latency; they are not open-loop rate experiments. The complete arrival interval plus every subsequent completion join/drain remains in the throughput denominator. Completions after the arrival interval are counted separately; neither admission nor queueing is counted as success. Warmup is excluded and every seeded record is reset before measurement without replacing the client pools. Reset traffic uses bulk chunks bounded to 8 MiB including action metadata, and every item acknowledgement is checked before continuing.

`-modes` selects `fixed-rate` (the default), `saturation`, or `fixed-rate,saturation`. `-rates none` disables rate cases; fixed-rate mode requires at least one rate. Run and offline audit share the same case builder.

Concurrency is bounded to 2048 total workers and 512 workers per OS process; every total must divide evenly between the processes. The default is 128 total workers across four processes. Fixed-rate cases use only the largest `-concurrency-levels` value, so compare 512 and 2048 workers in separate runs. Single-record native pools scale to three connections per worker. Each worker still has only the configured bounded queue and one active business product. The fixture keeps its server queue, pending-record and memory limits; high-concurrency rejection, timeout, overload, OOM or failed settling is a failed or overloaded measurement. Increasing workers does not demonstrate a database bottleneck; require measured sustained database CPU pressure before interpreting a run as CPU capacity.

Select offered rates after a short calibration: below, near and above the observed stable capacity. A high rate with rejections is an overload result, not a higher successful capacity. A sustainable rate needs all rounds without business failures, unknown writes, rejected arrivals or substantial generator lag, plus a declared latency/drain requirement.

Each worker records actual application-call attempts at the invocation boundary, including calls that fail or lack acknowledgement. The raw `application_requests` object contains `requests`, `records`, `minimum_records`, `maximum_records` and `records_per_request`. Concurrent seller calls share only that worker's synchronized ledger. Child and stage `ApplicationRequests` sum the worker ledgers; warmup clients are retained separately from timed clients. Planned `requested_logical_mutations` remains separate from actual attempted calls, so an early reconciliation failure does not invent subsequent product/seller calls.

Offline audit checks every histogram and worker/child/stage sum. For a successful stage, it regenerates the acknowledged business pages, adds the extra reconciliation mutations for the two-step rule, and requires actual submitted records to equal that logical count. In single input every request histogram bucket must be exactly one record. Native attempts refer to `_update` HTTP invocations; Weir attempts refer to SDK `Mutate` invocations. The ledger is application-level instrumentation, not a measured wire counter. Missing acknowledgements remain uncertain and are never replayed by the harness.

## Database observations

The default `-http-observer disabled` leaves the database connection direct for all paths. HTTP ledgers are then unavailable (`null`), and CSV HTTP counts/bytes remain unavailable rather than becoming zero. `-http-observer enabled` places an owned loopback observer between all paths and the database. It records physical methods/endpoints, request bodies and response bodies, with monitoring traffic labelled separately. HTTP byte counters exclude headers/framing and measure database-facing traffic, not Lua/SDK upload bytes. Both the native client and observer disable automatic HTTP compression, matching the Weir Search transport. The extra loopback hop is a limitation of the enabled observer. Aborted upstream-body copies retain their request attempt, observed partial bytes and failed/aborted counters.

When the server exposes Search phase metrics, before/after diagnostics retain `weir_search_http_requests_total` and `weir_search_http_request_duration_seconds` with operation, outcome and visibility labels. Durations run from `client.Do` through body read/validation; bulk duration includes `refresh=wait_for` and is not pure indexing time. `weir_search_http_request_records` records the attempted framed mget/bulk request size at the `client.Do` boundary; it does not count acknowledged writes. `weir_search_lua_evaluation_duration_seconds` measures Lua evaluation only. The summarizer retains every labeled counter/histogram bucket delta and emits operation totals/means, wire-record means and Lua time. Missing snapshots/series or reset counters remain `null`; absence does not prove zero requests.

Owned Docker cumulative cgroup counters provide database CPU, container network bytes, block I/O bytes and memory samples. The inspected CPU quota is retained. Elasticsearch `_stats` retains indexing/get/search/refresh/flush/merge/translog counters before and after the timed workload. Missing counters stay unavailable rather than becoming zero. Block bytes and filesystem counters do not prove I/O saturation.

`summarize_crawler.py` recomputes sampled database CPU per completed product and budget utilization using valid cumulative-counter intervals clipped to the stage, emitting sampling coverage alongside each cost. Treat short-stage or poor-coverage CPU figures as unavailable; use stages of at least 20 seconds with at least 80% CPU coverage. The 50 ms generator-lag flag in its CSV is a reporting threshold, not a production SLO. Every row is retained, including failed or overloaded stages; the `qualified` column additionally requires a complete report without cleanup errors. Run the offline auditor first; the CSV is an observation table, not a substitute for full-field validation.

Only newly owned loopback fixtures start, and the fixture's `cleanup.json` records ownership and removal. Before removing the owned Search container, the fixture keeps its last 1000 Docker log lines (at most 1 MiB, with a 5-second bound) and a `search-log-capture.json` summary; unavailable diagnostics never prevent cleanup. Ordinary unit tests remain offline and never start these services.

## Run

Prepare the pinned server and build the harness:

```sh
make prepare
go build -o .tools/weir-crawler ./cmd/weir-crawler
```

By default `-before` and `-after` are both `.tools/weir` and the `-after-*` controls equal the before path's batching, so the two Weir paths run identical setups. To compare something, set at least one `-after-*` control (for example `-after-batch-max-wait 5ms`) or pass a different `-after` binary. The default scenario is single input, one product per call, four OS clients, 128 total goroutine workers, observer disabled, and fixed offered rates of 4, 16 and 48 products/s. Start with one round, 5-second stages and a small matrix, then fix calibrated rates before a three-round, 20-second matrix on Linux with Docker.

A small local functional run:

```sh
.tools/weir-crawler -concurrency-levels 4 -client-processes 2 \
  -input-shape single -page-size 1 -profiles two-step -modes saturation -rates none \
  -after-batch-max-wait 5ms \
  -rounds 1 -warmup 1s -duration 1s -output results/local/crawler-preflight.json
```

Offline arithmetic and expected-field reconstruction, followed by CSV/JSON observations:

```sh
.tools/weir-crawler audit results/local/crawler.json
python3 scripts/summarize_crawler.py --report results/local/crawler.json \
  --output results/local/crawler-observations
```
