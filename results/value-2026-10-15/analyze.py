#!/usr/bin/env python3
"""Recompute the 2026-10-15 experiment from retained, immutable evidence.

Example manifest (paths are relative to this manifest):
{"entries": [{"label": "read-mongo", "role": "primary",
  "workflow_run": "https://github.com/batchstream/weir-tests/actions/runs/123",
  "receipt": "raw/read/measurement-batch-32.receipt.json",
  "report": "raw/read/saturation-batch-32/mongo.json.gz",
  "fixture_manifest": "raw/read/fixture/manifest.json.gz",
  "cleanup": "raw/read/fixture/cleanup.json.gz"}]}

Run: python3 analyze.py --manifest manifest.json --output analysis
Writes analysis.csv and analysis.json. Any invalid/incomplete input is retained
as an explicit failure and makes the command exit unsuccessfully.
"""

import argparse
import csv
from datetime import datetime
import gzip
import hashlib
import json
import math
from pathlib import Path
import re
import sys


CLIENT_SOURCE = "99d7951bc935c811bc155a40e390ef7a200e3fa5"
SERVER_SOURCE = "8955cde10e574dae55d890b0c9c824fd39219d1c"
PINS = {
    "go": "1.27.1", "sdk": "v0.15.0", "protocol": "v0.13.0",
    "weir_source": SERVER_SOURCE,
    "weir_version": "v0.2.1-0.20261015053327-8955cde10e57",
    "weir_sum": "h1:u1YqjXPBmo6sIxaZUGaGW4IEx8Z5XUzq2BdNDnN9/zE=",
    "mongodb_image": "mongo@sha256:d0d926f94df099bff534b7ee5b5986458131a22489dfff8664509af0c1e2ca9c",
    "elasticsearch_image": "docker.elastic.co/elasticsearch/elasticsearch@sha256:e98f9c3b09beb2fbb9eaf667d602df3f0e00bd3644138b8458dc17ba1a675595",
}
COMMANDS = ("find", "update", "bulkWrite", "getMore", "killCursors",
            "commitTransaction", "abortTransaction")
ROLES = {"primary", "ablation", "cpu2", "bulk", "lua"}
SCOPE = {
    "latency": "Go 256 logarithmic buckets; pooled single-request quantiles are inclusive upper bounds, at most 12.5% plus 1us; bulk quantiles are per-round call-latency bounds and are never pooled",
    "commands": "owned MongoDB serverStatus deltas for the seven selected business command types; excludes hello, serverStatus and other command types; Elasticsearch unavailable",
    "prometheus_cpu": "Weir process_cpu_seconds_total difference between snapshots wrapping the timed stage, including the short barrier/collection window; divided by successful business records",
    "ps_cpu": "sampling estimate only: cumulative ps CPU differences over valid observed intervals, divided by those intervals; Linux ps CPU is quantized at approximately one second; no CPU/op or small cost-benefit claim",
    "memory": "sampled maxima; Weir/client memory is ps RSS; Docker database memory is container memory usage, not database-process RSS; sums of individual PID peaks are not simultaneous peaks",
    "capacity": "same-host clients and Weir, owned database with inspected CPU quota; CPU saturation and next-level plateau do not establish production capacity or uncached/disk-bound performance",
}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def integer(value):
    return isinstance(value, int) and not isinstance(value, bool) and value >= 0


def number(value):
    return isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value)


def digest(value):
    return isinstance(value, str) and re.fullmatch(r"[0-9a-f]{64}", value) is not None


def load_json(path):
    raw = path.read_bytes()
    decoded = gzip.decompress(raw) if path.name.endswith(".json.gz") else raw
    value = json.loads(decoded, parse_constant=lambda value: (_ for _ in ()).throw(ValueError("non-finite JSON: " + value)))
    return value, hashlib.sha256(raw).hexdigest(), hashlib.sha256(decoded).hexdigest()


def arguments(command):
    require(isinstance(command, list) and len(command) > 1 and all(isinstance(value, str) for value in command), "missing/non-string command")
    options = {}
    index = 1
    allowed = {"-mode", "-weir", "-backend", "-database-cpus", "-backend-batch-limit",
               "-client-processes", "-concurrency-levels", "-batch-sizes", "-records",
               "-payload-bytes", "-write-percent", "-rounds", "-warmup-duration", "-duration", "-output"}
    while index < len(command):
        name = command[index]
        require(name not in options, "duplicate command flag " + name)
        if name == "-lua-mutations":
            options[name] = True
            index += 1
            continue
        require(name in allowed and index + 1 < len(command), "unexpected command flag " + str(name))
        options[name] = command[index + 1]
        index += 2
    require(allowed <= options.keys(), "missing command arguments")
    return options


def quantile(buckets, maximum, percent):
    count = sum(buckets)
    if not count:
        return 0
    rank = (count * percent + 99) // 100
    observed = 0
    for index, value in enumerate(buckets):
        observed += value
        if observed < rank:
            continue
        if index == 0:
            return 0
        if index == 255:
            return maximum
        exponent, fraction = divmod(index - 1, 8)
        base = 1 << exponent
        upper = base + ((fraction + 1) * base + 7) // 8 - 1
        return max(upper * 1000, 1000)
    raise ValueError("histogram rank unavailable")


def latency(buckets, maximum):
    result = {"samples": sum(buckets), "p50_upper_ns": quantile(buckets, maximum, 50),
              "p95_upper_ns": quantile(buckets, maximum, 95), "p99_upper_ns": quantile(buckets, maximum, 99)}
    return result


def validate(entry, receipt, report):
    require(entry["role"] in ROLES, "unknown experiment role")
    require(re.fullmatch(r"https://github.com/batchstream/weir-tests/actions/runs/[0-9]+", entry["workflow_run"]) is not None, "invalid workflow URL")
    require(receipt["workflow_run"] == entry["workflow_run"], "workflow URL differs from receipt")
    require(receipt["source"] == CLIENT_SOURCE and receipt["source_dirty"] is False, "requires clean client source " + CLIENT_SOURCE)
    require(receipt["exit_code"] == 0, "benchmark process failed")
    for key in ("client_binary_sha256", "server_binary_sha256"):
        require(digest(receipt.get(key)) and receipt.get(key.replace("_sha256", "_after_sha256")) == receipt[key], key + " changed or unavailable")
    require(all(receipt["versions"].get(key) == value for key, value in PINS.items()), "pinned dependencies differ")
    server = receipt["server_receipt"]
    require(server["source"] == SERVER_SOURCE and server["identity"]["revision"] == SERVER_SOURCE,
            "server source differs")
    require(server["version"] == PINS["weir_version"] and server["go"] == PINS["go"] and server["sha256"] == receipt["server_binary_sha256"], "server receipt differs")
    require(server["identity"]["go"] == "go" + PINS["go"], "server toolchain differs")
    require(receipt["go"].startswith("go version go" + PINS["go"] + " "), "client toolchain differs")
    build = receipt["client_build"]
    for module, version in (("github.com/batchstream/weir-go", PINS["sdk"]), ("github.com/batchstream/weir-protocol", PINS["protocol"])):
        require(any(line.split()[:3] == ["dep", module, version] for line in build.splitlines()), "client binary dependency differs: " + module)
    require(not any(line.strip().startswith("=>") for line in build.splitlines()), "client has replaced dependencies")
    opts = arguments(receipt["command"])
    require(opts["-mode"] in ("saturation", "bulk-saturation"), "unexpected mode")
    single = opts["-mode"] == "saturation"
    levels = [int(value) for value in opts["-concurrency-levels"].split(",")]
    batches = [int(value) for value in opts["-batch-sizes"].split(",")]
    require(len(levels) >= 2 and levels == sorted(set(levels)) and min(levels) > 0, "invalid concurrency ladder")
    require(len(batches) == 1 and (batches == [1] if single else 1 <= batches[0] <= 64), "unexpected batches")
    require(max(levels) <= (512 if single else 64), "concurrency out of bounds")
    require(all(level % 4 == 0 for level in levels) and opts["-client-processes"] == "4", "client process count differs")
    expected = {"-records": "8192", "-payload-bytes": "1024", "-rounds": "3", "-warmup-duration": "10s", "-duration": "20s"}
    require(all(opts[key] == value for key, value in expected.items()), "requested benchmark parameters differ")
    require(opts["-write-percent"] in ("0", "10", "100") and opts["-backend"] in ("all", "mongo", "search"), "invalid workload")
    cpu, limit = float(opts["-database-cpus"]), int(opts["-backend-batch-limit"])
    require(cpu in (1, 2) and 1 <= limit <= 1024, "unexpected CPU quota or adapter limit")
    require(entry["role"] != "ablation" or (cpu == 1 and limit in (1, 32)), "ablation requires limit 1 or its same-run limit-32 control")
    require(entry["role"] != "cpu2" or cpu == 2, "cpu2 requires two database CPUs")
    require(entry["role"] != "primary" or (cpu == 1 and limit == 32), "primary requires one database CPU and adapter limit 32")
    require((entry["role"] == "bulk") == (not single), "bulk role/mode differs")
    lua = bool(opts.get("-lua-mutations"))
    require((entry["role"] == "lua") == lua, "Lua role/command differs")
    params, provenance = report["parameters"], report["provenance"]
    dataset, backend = params["dataset"], params["dataset"]["backend"]
    require(backend in ("mongo", "search") and opts["-backend"] in ("all", backend), "backend differs")
    require(report["schema"] == (3 if single else 2), "unexpected report schema")
    require(params["concurrency_levels"] == levels and params["batch_sizes"] == batches and params["paired_rounds"] == 3, "report matrix differs from command")
    require(params["warmup_duration"] == "10s" and params["measurement_duration"] == "20s" and params["operation_timeout"] == "10s" and params["database_cpu_budget_threshold_percent"] == 90, "report durations/budget differ")
    require(params["requested_write_percent"] == int(opts["-write-percent"]), "write mix differs")
    require(dataset["records"] == 8192 and dataset["padding_bytes"] == 1024 and dataset["concurrency"] == max(levels) and bool(dataset.get("lua_mutations")) == lua, "dataset differs")
    require(params.get("client_process_count", 0) == (4 if single else 0), "report process count differs")
    require(not single or params.get("records_per_business_request") == 1, "single-request count differs")
    expected_provenance = {"weir_source": SERVER_SOURCE, "test_source": CLIENT_SOURCE, "test_source_dirty": "false", "weir_binary_sha256": receipt["server_binary_sha256"], "client_operation_timeout": "10s", "database_docker_cpu_quota": opts["-database-cpus"], "weir_batching_max_operations_override": str(limit), "weir_transport_max_pending_records_override": "0", "weir_backend_max_exchange_bytes_override": "0", "weir_source_kind": "locked immutable module source"}
    pinned_provenance = {key: PINS[key] for key in ("sdk", "protocol", "mongodb_image", "elasticsearch_image")}
    expected_provenance.update(pinned_provenance)
    require(all(provenance.get(key) == value for key, value in expected_provenance.items()), "report provenance differs from receipt/command")
    quota = report["database_cpu_quota"]
    require(quota["host_config_nano_cpus"] == int(cpu * 1e9) and quota["observed_cpus"] == cpu and quota["requested_cpus"] == cpu, "inspected CPU quota differs")
    require(digest(quota["container_id"]), "invalid owned database container identity")
    require(not report.get("incomplete"), "incomplete benchmark: " + report.get("incomplete", ""))
    identities = []
    for pair in report["pairs"]:
        identities.append((pair["batch_size"], pair["concurrency"], pair["round"]))
        level, round_index = levels.index(pair["concurrency"]), pair["round"] - 1
        order = ["direct", "weir"] if (level + round_index) % 2 == 0 else ["weir", "direct"]
        require(pair["order"] == order, "paired execution order differs")
        for path in ("direct", "weir"):
            value = pair[path]
            require(value["path"] == path and value["verified"] is True, "unverified or misidentified path")
            require(all(integer(value.get(key, 0)) for key in ("elapsed_ns", "attempted", "planned", "succeeded", "reads", "writes", "client_requests", "errors", "indeterminate", "not_attempted", "applied_with_error", "business_operation_timeouts")), "non-integer/negative business counts")
            require(value["elapsed_ns"] >= 20_000_000_000 and value["attempted"] == value["planned"] == value["succeeded"] > 0, "incomplete operation counts/duration")
            require(all(value.get(key, 0) == 0 for key in ("errors", "indeterminate", "not_attempted", "applied_with_error", "business_operation_timeouts")), "failed or indeterminate business operation")
            require(value["reads"] + value["writes"] == value["succeeded"] and value["client_requests"] * pair["batch_size"] == value["succeeded"], "request/record counts differ")
            observed_latency = value["latency"]
            require(observed_latency["samples"] == value["attempted"] and all(integer(observed_latency["p" + str(p) + "_upper_bound_ns"]) for p in (50, 95, 99)), "latency count/type differs")
            require(observed_latency["p50_upper_bound_ns"] <= observed_latency["p95_upper_bound_ns"] <= observed_latency["p99_upper_bound_ns"], "latency quantiles out of order")
            require(value["resources"]["database_allocated_cpus"] == cpu, "resource denominator differs")
            if single:
                children = value["client_processes"]
                require(len(children) == 4 and len({child["pid"] for child in children}) == 4, "independent process identities differ")
                require(sorted(child["worker_offset"] for child in children) == list(range(0, pair["concurrency"], pair["concurrency"] // 4)), "worker ranges differ")
                require(sum(child["business_requests"] for child in children) == value["client_requests"], "child request totals differ")
                for child in children:
                    require(child["owned_process_exited"] and not child.get("owned_process_exit_error") and child["phase"] == "timed", "client failed to exit or wrong phase")
                    require(child["concurrent_workers"] == pair["concurrency"] // 4 and child["native_connection_pool_limit"] == (pair["concurrency"] // 4 if path == "direct" else 0), "worker/pool limits differ")
                    require(0 <= child["start_lag_ns"] <= 100_000_000, "client barrier lag invalid")
                    buckets = child["latency_histogram_buckets"]
                    require(len(buckets) == 256 and all(integer(count) for count in buckets) and sum(buckets) == child["business_requests"], "child histogram count differs")
    expected_ids = [(batch, level, round_index) for batch in batches for level in levels for round_index in (1, 2, 3)]
    require(sorted(identities) == sorted(expected_ids), "incomplete/duplicate paired rounds")
    return opts


def metric_delta(metrics, name, labels=None):
    values = []
    for phase in ("before", "after"):
        snapshot = metrics.get(phase, {})
        if snapshot.get("error"):
            return None
        observed = [item["value"] for item in snapshot.get("metrics", []) if item["name"] == name and item.get("labels", {}) == (labels or {})]
        if len(observed) != 1 or not number(observed[0]):
            return None
        values.append(observed[0])
    return values[1] - values[0] if values[1] >= values[0] else None


def validate_fixture(receipt, report, fixture, cleanup):
    require(cleanup["errors"] == "", "fixture cleanup failed: " + str(cleanup["errors"]))
    require(isinstance(fixture["owner"], str) and fixture["owner"] and cleanup["owner"] == fixture["owner"], "fixture/cleanup owner differs")
    require(fixture["binary_sha256"] == receipt["server_binary_sha256"], "fixture server binary differs from receipt")
    require(not fixture["options"].get("MongoBinary"), "this experiment requires owned Docker databases")
    containers = fixture["containers"]
    require(isinstance(containers, list) and containers, "fixture has no owned database containers")
    require(len({item["id"] for item in containers}) == len(containers), "duplicate fixture container identity")
    removed = {item["id"]: item for item in cleanup["containers"]}
    require(len(removed) == len(cleanup["containers"]) and set(removed) == {item["id"] for item in containers}, "cleanup container identities differ")
    for item in containers:
        observed = removed[item["id"]]
        require(digest(item["id"]) and observed["removed"] is True and all(observed[key] == item[key] for key in ("name", "image")), "container cleanup missing or name/image differs")
        require(item["backend"] in ("mongo", "search") and item["image"] == PINS["mongodb_image" if item["backend"] == "mongo" else "elasticsearch_image"], "fixture backend image differs from pins")
    backend = report["parameters"]["dataset"]["backend"]
    require(any(item["id"] == report["database_cpu_quota"]["container_id"] and item["backend"] == backend for item in containers), "report CPU-quota container does not belong to this fixture/backend")
    nodes, stopped = fixture["nodes"], cleanup["processes"]
    require(isinstance(nodes, list) and nodes, "fixture has no Weir nodes")
    require(len({node["ProcessID"] for node in nodes}) == len(nodes), "duplicate fixture process identity")
    require(len(stopped) == len(nodes), "cleanup process count differs")
    for index, node in enumerate(nodes):
        require(integer(node["ProcessID"]) and node["ProcessID"] > 0, "invalid fixture process identity")
        matching = [process for process in stopped if process["pid"] == node["ProcessID"] and process.get("index") == index]
        require(len(matching) == 1 and matching[0]["stopped"] is True, "Weir process cleanup missing or not stopped")


def physical_commands(value, backend):
    reasons = [value.get("physical_database_command_observation_unavailable", "")]
    before, after = value.get("timed_database_counters_before"), value.get("timed_database_counters_after")
    if backend != "mongo":
        return None, ["Elasticsearch has no physical client HTTP-command counter"]
    if before is None or after is None:
        return None, ["missing timed database counter boundary"]
    reasons += [before.get("physical_command_observation_unavailable", ""), after.get("physical_command_observation_unavailable", "")]
    result = {}
    for name in COMMANDS:
        start, end = before.get("physical_command_totals", {}).get(name), after.get("physical_command_totals", {}).get(name)
        if not integer(start) or not integer(end) or end < start:
            reasons.append("missing/reset " + name)
        else:
            result[name] = end - start
    reasons = sorted({part.strip() for reason in reasons for part in reason.split(";") if part.strip()})
    require(value.get("timed_physical_database_command_deltas", {}) == result, "stored physical command delta differs from raw boundaries")
    return (None if reasons else result), reasons


def resource_stats(value, cpu):
    samples = value["resources"].get("samples", [])
    measured = value["elapsed_ns"]
    intervals, covered, full, weighted = 0, 0, 0, 0.0
    for previous, current in zip(samples, samples[1:]):
        span = min(current["elapsed_ns"], measured) - max(previous["elapsed_ns"], 0)
        clock_span = current.get("database_cpu_counter_clock_unix_ns", current["elapsed_ns"]) - previous.get("database_cpu_counter_clock_unix_ns", previous["elapsed_ns"])
        start, end = previous.get("database_cpu_time_ns"), current.get("database_cpu_time_ns")
        if span <= 0 or clock_span <= 0 or not integer(start) or not integer(end) or end < start or previous.get("error") or current.get("error"):
            continue
        percent = (end - start) / clock_span / cpu * 100
        intervals += 1
        covered += span
        weighted += percent * span
        full += span if percent >= 90 else 0
    mean, coverage = (weighted / covered if covered else None), covered / measured
    processes = {}
    for sample in samples:
        for process in sample.get("client_processes", []):
            processes.setdefault(process["pid"], []).append((sample["elapsed_ns"], process))
    client_stats = []
    for pid, observations in sorted(processes.items()):
        elapsed, usage = 0, 0
        for (start_time, start), (end_time, end) in zip(observations, observations[1:]):
            span = end_time - start_time
            if span > 0 and not start.get("error") and not end.get("error") and end["cpu_time_ns"] >= start["cpu_time_ns"]:
                elapsed += span
                usage += end["cpu_time_ns"] - start["cpu_time_ns"]
        process_resources = {"pid": pid, "peak_rss_bytes": max((item.get("memory_bytes", 0) for _, item in observations), default=0), "sampled_mean_cpu_cores_estimate": usage / elapsed if elapsed else None, "valid_sampling_interval_ns": elapsed, "sampling_coverage": elapsed / measured}
        client_stats.append(process_resources)
    result = {"database_cpu_mean_budget_percent": mean, "database_cpu_valid_intervals": intervals, "database_cpu_sampling_coverage": coverage, "database_cpu_full_measurement_fraction": full / measured, "database_cpu_saturated": intervals >= 5 and coverage >= 0.8 and mean is not None and mean >= 90 and full / measured >= 0.8, "database_peak_container_memory_bytes": max((sample.get("database_memory_bytes", 0) for sample in samples), default=0) or None, "weir_peak_rss_bytes": max((sample.get("weir_memory_bytes", 0) for sample in samples), default=0) or None, "client_process_resources": client_stats, "client_simultaneous_peak_rss_bytes": (max((sum(process.get("memory_bytes", 0) for process in sample.get("client_processes", [])) for sample in samples), default=0) or None), "client_mean_cpu_cores_estimate_sum": sum(item["sampled_mean_cpu_cores_estimate"] for item in client_stats) if client_stats and all(item["sampled_mean_cpu_cores_estimate"] is not None for item in client_stats) else None, "client_resource_unavailable": None if client_stats else "bulk coordinator has no per-PID RSS samples; no client-process resource data available", "resource_sampling_errors": [sample.get("error") or sample.get("database_counter_error") for sample in samples if sample.get("error") or sample.get("database_counter_error")]}
    return result


def observation(options):
    entry, report, receipt = options["entry"], options["report"], options["receipt"]
    pair, path, qualified = options["pair"], options["path"], options["qualified"]
    value, params = pair[path], report["parameters"]
    opts = arguments(receipt["command"])
    backend, cpu = params["dataset"]["backend"], float(opts["-database-cpus"])
    single = opts["-mode"] == "saturation"
    row = {key: entry[key] for key in ("label", "role", "workflow_run")}
    row.update(backend=backend, concurrency=pair["concurrency"], batch_size=pair["batch_size"], backend_batch_limit=int(opts["-backend-batch-limit"]), database_cpus=cpu, requested_write_percent=params["requested_write_percent"], lua=bool(params["dataset"].get("lua_mutations")), mode=opts["-mode"], round=pair["round"], path=path, qualified_input=qualified, order=pair["order"], reads=value["reads"], writes=value["writes"], succeeded=value["succeeded"], attempted=value["attempted"], requests=value["client_requests"], elapsed_ns=value["elapsed_ns"], qps=value["succeeded"] * 1e9 / value["elapsed_ns"], errors=value["errors"], unknown=value["indeterminate"], timeouts=value.get("business_operation_timeouts", 0), verified=value["verified"])
    if single:
        buckets = [sum(child["latency_histogram_buckets"][index] for child in value["client_processes"]) for index in range(256)]
        maximum = max(child["result"]["latency"]["max_observed_ns"] for child in value["client_processes"])
        observed = latency(buckets, maximum)
        require(observed["samples"] == value["attempted"], "pooled histogram sample count differs")
        require(all(observed["p" + str(p) + "_upper_ns"] == value["latency"]["p" + str(p) + "_upper_bound_ns"] for p in (50, 95, 99)), "stored percentile differs from raw pooled histogram")
        row.update(observed)
        row["_buckets"], row["_maximum"] = buckets, maximum
    else:
        quantiles = {"p" + str(p) + "_upper_ns": value["latency"]["p" + str(p) + "_upper_bound_ns"] for p in (50, 95, 99)}
        row.update(quantiles)
        row["samples"] = value["latency"]["samples"]
    commands, reasons = physical_commands(value, backend)
    row.update(selected_mongo_commands=commands, command_unavailable_reasons=reasons, selected_mongo_commands_per_success=sum(commands.values()) / row["succeeded"] if commands is not None and row["succeeded"] else None)
    metrics, labels = value.get("server_metrics") or {}, {"store": params["dataset"]["store"]}
    batch_sum, batch_count = metric_delta(metrics, "weir_store_batch_operations_sum", labels), metric_delta(metrics, "weir_store_batch_operations_count", labels)
    row.update(adapter_batch_operations=batch_sum, adapter_batch_invocations=batch_count, adapter_batch_average=batch_sum / batch_count if batch_sum is not None and batch_count else None, adapter_metrics_unavailable=metrics.get("unavailable"))
    seconds = metric_delta(metrics, "process_cpu_seconds_total") if path == "weir" else None
    row.update(weir_prometheus_cpu_seconds=seconds, weir_prometheus_cpu_us_per_success=seconds * 1e6 / row["succeeded"] if seconds is not None and row["succeeded"] else None)
    if seconds is not None:
        before = datetime.fromisoformat(metrics["before"]["at_utc"].replace("Z", "+00:00"))
        after = datetime.fromisoformat(metrics["after"]["at_utc"].replace("Z", "+00:00"))
        row["weir_prometheus_counter_window_seconds"] = (after - before).total_seconds()
    row.update(resource_stats(value, cpu))
    return row


def aggregate(rows):
    keys = ("label", "role", "backend", "concurrency", "batch_size", "backend_batch_limit", "database_cpus", "requested_write_percent", "lua", "mode", "path")
    groups = {}
    for row in rows:
        groups.setdefault(tuple(row[key] for key in keys), []).append(row)
    summaries = []
    for identity, values in sorted(groups.items()):
        values.sort(key=lambda value: value["round"])
        total, elapsed = sum(value["succeeded"] for value in values), sum(value["elapsed_ns"] for value in values)
        summary = dict(zip(keys, identity))
        summary.update(qualified=all(value["qualified_input"] for value in values) and [value["round"] for value in values] == [1, 2, 3], observed_rounds=[value["round"] for value in values], successful_records=total, elapsed_ns=elapsed, qps=total * 1e9 / elapsed, round_qps=[value["qps"] for value in values], qps_range=[min(value["qps"] for value in values), max(value["qps"] for value in values)], errors=sum(value["errors"] for value in values), unknown=sum(value["unknown"] for value in values), timeouts=sum(value["timeouts"] for value in values), all_verified=all(value["verified"] for value in values), all_rounds_database_cpu_saturated=all(value["database_cpu_saturated"] for value in values), round_database_cpu_coverage=[value["database_cpu_sampling_coverage"] for value in values], round_database_cpu_budget_percent=[value["database_cpu_mean_budget_percent"] for value in values])
        if all("_buckets" in value for value in values):
            pooled = [sum(value["_buckets"][index] for value in values) for index in range(256)]
            summary["pooled_single_request_latency"] = latency(pooled, max(value["_maximum"] for value in values))
        for p in (50, 95, 99):
            quantiles = [value["p" + str(p) + "_upper_ns"] for value in values]
            summary["round_p" + str(p) + "_upper_ns"] = quantiles
            summary["round_p" + str(p) + "_upper_ns_range"] = [min(quantiles), max(quantiles)]
        commands = [value["selected_mongo_commands"] for value in values]
        complete = all(value is not None for value in commands)
        summary["selected_mongo_commands"] = {name: sum(value[name] for value in commands) for name in COMMANDS} if complete else None
        summary["command_unavailable_reasons"] = sorted({reason for value in values for reason in value["command_unavailable_reasons"]})
        summary["selected_mongo_commands_per_success"] = sum(summary["selected_mongo_commands"].values()) / total if complete and total else None
        counts, operations = [value["adapter_batch_invocations"] for value in values], [value["adapter_batch_operations"] for value in values]
        summary["adapter_batch_average"] = sum(operations) / sum(counts) if all(value is not None for value in counts + operations) and sum(counts) else None
        seconds = [value["weir_prometheus_cpu_seconds"] for value in values]
        summary["weir_prometheus_cpu_us_per_success"] = sum(seconds) * 1e6 / total if total and all(value is not None for value in seconds) else None
        summary["round_weir_prometheus_cpu_us_per_success"] = [value["weir_prometheus_cpu_us_per_success"] for value in values]
        for field in ("database_peak_container_memory_bytes", "weir_peak_rss_bytes", "client_simultaneous_peak_rss_bytes"):
            summary[field] = max((value[field] for value in values if value[field] is not None), default=None)
        summary["round_client_process_resources"] = [{"round": value["round"], "processes": value["client_process_resources"]} for value in values]
        summaries.append(summary)
    for summary in summaries:
        following = [value for value in summaries if all(value[key] == summary[key] for key in keys if key != "concurrency") and value["concurrency"] > summary["concurrency"]]
        following.sort(key=lambda value: value["concurrency"])
        summary["next_concurrency"] = following[0]["concurrency"] if following else None
        summary["throughput_plateau_at_next_concurrency"] = bool(following and summary["qualified"] and following[0]["qualified"] and following[0]["qps"] <= summary["qps"] * 1.1)
        summary["database_saturation_demonstrated"] = summary["qualified"] and summary["all_rounds_database_cpu_saturated"] and summary["throughput_plateau_at_next_concurrency"]
    return summaries


def comparisons(summaries, rows):
    keys = ("label", "role", "backend", "concurrency", "batch_size", "backend_batch_limit", "database_cpus", "requested_write_percent", "lua", "mode")
    grouped = {}
    for summary in summaries:
        grouped.setdefault(tuple(summary[key] for key in keys), {})[summary["path"]] = summary
    result = []
    for identity, paths in sorted(grouped.items()):
        value = dict(zip(keys, identity))
        qualified = set(paths) == {"direct", "weir"} and all(item["qualified"] for item in paths.values())
        value["qualified"] = qualified
        value["weir_direct_qps_ratio"] = paths["weir"]["qps"] / paths["direct"]["qps"] if qualified and paths["direct"]["qps"] else None
        value["weir_qps_delta_percent"] = (value["weir_direct_qps_ratio"] - 1) * 100 if value["weir_direct_qps_ratio"] is not None else None
        round_values = [row for row in rows if tuple(row[key] for key in keys) == identity]
        paired = {}
        for row in round_values:
            paired.setdefault(row["round"], {})[row["path"]] = row["qps"]
        value["round_weir_direct_qps_ratios"] = [paths["weir"] / paths["direct"] for _, paths in sorted(paired.items()) if set(paths) == {"direct", "weir"} and paths["direct"]] if qualified else None
        result.append(value)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    manifest_path = args.manifest.resolve()
    manifest, manifest_hash, manifest_json_hash = load_json(manifest_path)
    require(isinstance(manifest.get("entries"), list) and manifest["entries"], "manifest requires entries")
    rows, failures, inputs = [], [], []
    identities, binary_identities = set(), set()
    for entry in manifest["entries"]:
        info, report, receipt = dict(entry), None, None
        documents = {}
        info["cleanup_verified"] = False
        try:
            require(isinstance(entry.get("label"), str) and entry["label"], "missing experiment label")
            for kind in ("receipt", "report", "fixture_manifest", "cleanup"):
                path = (manifest_path.parent / entry[kind]).resolve()
                require(path.name.endswith((".json", ".json.gz")), "evidence requires JSON or JSON.gz files")
                value, hash_value, json_hash = load_json(path)
                info[kind + "_sha256"] = hash_value
                info[kind + "_json_sha256"] = json_hash
                info[kind + "_resolved"] = str(path)
                documents[kind] = value
                if kind == "report":
                    report = value
                elif kind == "receipt":
                    receipt = value
            opts = arguments(receipt["command"])
            identity = (entry["label"], report["parameters"]["dataset"]["backend"], opts["-database-cpus"], opts["-backend-batch-limit"], opts["-mode"], opts["-write-percent"])
            require(identity not in identities, "duplicate label/backend report")
            identities.add(identity)
            validate(entry, receipt, report)
            validate_fixture(receipt, report, documents["fixture_manifest"], documents["cleanup"])
            info["cleanup_verified"] = True
            info["fixture_owner"] = documents["fixture_manifest"]["owner"]
            binary_identities.add((receipt["client_binary_sha256"], receipt["server_binary_sha256"]))
            info["qualified"] = True
        except (ValueError, KeyError, TypeError, OSError, EOFError, ZeroDivisionError) as error:
            info["qualified"] = False
            info["failure"] = str(error)
            failure = {"label": entry.get("label"), "report": entry.get("report"), "error": str(error)}
            failures.append(failure)
        inputs.append(info)
        if report is None or receipt is None:
            continue
        for pair in report.get("pairs", []):
            for path in ("direct", "weir"):
                try:
                    observation_options = {"entry": entry, "report": report, "receipt": receipt, "pair": pair, "path": path, "qualified": info["qualified"]}
                    rows.append(observation(observation_options))
                except (ValueError, KeyError, TypeError, ZeroDivisionError) as error:
                    failure = {"label": entry.get("label"), "report": entry.get("report"), "round": pair.get("round"), "concurrency": pair.get("concurrency"), "path": path, "error": str(error)}
                    failures.append(failure)
    if len(binary_identities) > 1:
        for entry in manifest["entries"]:
            failure = {"label": entry["label"], "error": "binary identity differs across this fixed-source experiment"}
            failures.append(failure)
    failed_labels = {failure["label"] for failure in failures}
    for info in inputs:
        if info.get("label") in failed_labels:
            info["qualified"] = False
    for row in rows:
        if row["label"] in failed_labels:
            row["qualified_input"] = False
    summaries = aggregate(rows)
    public_rows = [{key: value for key, value in row.items() if not key.startswith("_")} for row in rows]
    output = args.output.resolve()
    input_paths = {manifest_path} | {Path(value) for info in inputs for key, value in info.items() if key.endswith("_resolved")}
    require(output.with_suffix(".json") not in input_paths and output.with_suffix(".csv") not in input_paths, "output would overwrite input evidence")
    public_inputs = [{key: value for key, value in info.items() if not key.endswith("_resolved")} for info in inputs]
    result = {"status": "failed" if failures else "complete", "manifest": manifest_path.name, "manifest_sha256": manifest_hash, "manifest_json_sha256": manifest_json_hash, "analyzer_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "expected_client_source": CLIENT_SOURCE, "expected_server_source": SERVER_SOURCE, "scopes": SCOPE, "inputs": public_inputs, "failures": failures, "observations": public_rows, "aggregates": summaries, "comparisons": comparisons(summaries, rows)}
    output.parent.mkdir(parents=True, exist_ok=True)
    output.with_suffix(".json").write_text(json.dumps(result, indent=2, allow_nan=False) + "\n")
    columns = list(dict.fromkeys(key for row in public_rows for key in row))
    with output.with_suffix(".csv").open("w", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=columns, lineterminator="\n")
        writer.writeheader()
        for row in public_rows:
            csv_row = {key: json.dumps(value, separators=(",", ":")) if isinstance(value, (dict, list)) else value for key, value in row.items()}
            writer.writerow(csv_row)
    output_status = {"status": result["status"], "observations": len(rows), "aggregates": len(summaries), "failures": len(failures), "output": str(output)}
    print(json.dumps(output_status))
    return 1 if failures else 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, KeyError, TypeError, OSError, EOFError) as error:
        print("analysis failed: " + str(error), file=sys.stderr)
        sys.exit(1)
