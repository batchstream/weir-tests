#!/usr/bin/env python3
"""Plot the retained analysis without selecting favorable rounds."""

import argparse
import json
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib.ticker import ScalarFormatter


COLORS = {"direct": "#2563a6", "weir": "#d46728"}
WORK = (("Read", 0, False), ("90/10 read/write", 10, False),
        ("Write", 100, False), ("Atomic transform", 100, True))


def matching(rows, filters):
    selected = [row for row in rows if row.get("qualified")
                and all(row.get(key) == value for key, value in filters.items())]
    selected.sort(key=lambda row: row["concurrency"])
    return selected


def draw(ax, rows, metric, label):
    if not rows:
        return
    path = rows[0]["path"]
    workers = [row["concurrency"] for row in rows]
    if metric == "qps":
        values = [row["qps"] / 1000 for row in rows]
        low = [min(row["round_qps"]) / 1000 for row in rows]
        high = [max(row["round_qps"]) / 1000 for row in rows]
    else:
        values = [row["pooled_single_request_latency"]["p95_upper_ns"] / 1e6 for row in rows]
        low = [row["round_p95_upper_ns_range"][0] / 1e6 for row in rows]
        high = [row["round_p95_upper_ns_range"][1] / 1e6 for row in rows]
    limit = rows[0]["backend_batch_limit"]
    style = "--" if rows[0]["role"] == "ablation" and limit == 1 else "-"
    marker = "o" if path == "direct" else "s"
    ax.plot(workers, values, linestyle=style, marker=marker,
            color=COLORS[path], label=label, linewidth=1.7, markersize=4)
    ax.fill_between(workers, low, high, color=COLORS[path], alpha=0.11)


def finish_axis(ax, workers, metric):
    ax.set_xscale("log", base=2)
    ax.set_xticks(workers)
    ax.xaxis.set_major_formatter(ScalarFormatter())
    ax.set_xlabel("Concurrent workers")
    ax.grid(alpha=0.2)
    if metric == "qps":
        ax.set_ylim(bottom=0)
        ax.set_ylabel("Thousand successful records/s")
    else:
        ax.set_yscale("log")
        ax.set_ylabel("p95 upper bound, ms")
    if not ax.lines:
        ax.text(0.5, 0.5, "FAILED / INCOMPLETE\nSee raw evidence and analysis.json",
                ha="center", va="center", transform=ax.transAxes, color="#a52727")


def save(fig, name, output):
    metadata = {"Date": None}
    svg = output / (name + ".svg")
    fig.savefig(svg, metadata=metadata)
    svg.write_text("\n".join(line.rstrip() for line in svg.read_text().splitlines()) + "\n")
    fig.savefig(output / (name + ".png"), dpi=170)
    plt.close(fig)


def primary(rows, output, metric):
    size = (17, 7.6)
    fig, axes = plt.subplots(2, 4, figsize=size, constrained_layout=True)
    for backend_index, backend in enumerate(("mongo", "search")):
        for column, (title, writes, lua) in enumerate(WORK):
            ax = axes[backend_index, column]
            for path in ("direct", "weir"):
                filters = {"backend": backend, "role": "lua" if lua else "primary",
                           "requested_write_percent": writes, "lua": lua,
                           "database_cpus": 1, "backend_batch_limit": 32, "path": path}
                selected = matching(rows, filters)
                draw(ax, selected, metric, "Native" if path == "direct" else "Weir")
            ax.set_title(("MongoDB" if backend == "mongo" else "Search") + " · " + title)
            finish_axis(ax, (4, 8, 32, 128, 256), metric)
    axes[0, 0].legend(frameon=False)
    heading = "Successful throughput" if metric == "qps" else "Tail latency of independent single-record requests"
    fig.suptitle(heading + " · database quota 1 CPU · four client processes", fontsize=14)
    note = "Lines: all-round records / elapsed time. Bands: all three round rates."
    if metric != "qps":
        note = "Lines: pooled histogram p95 upper bounds; bands: all three round bounds. Approximation ≤12.5% + 1µs."
    fig.supxlabel(note + "  8192 hot records, 1024-byte padding; 10s warmup + 20s measured per path, three paired rounds.", fontsize=9)
    save(fig, "throughput" if metric == "qps" else "tail-latency", output)


def ablation(rows, output):
    size = (13, 7.4)
    fig, axes = plt.subplots(2, 3, figsize=size, constrained_layout=True)
    for backend_index, backend in enumerate(("mongo", "search")):
        for column, (title, writes, _) in enumerate(WORK[:3]):
            ax = axes[backend_index, column]
            for limit in (1, 32):
                for path in ("direct", "weir"):
                    filters = {"backend": backend, "role": "ablation",
                               "requested_write_percent": writes, "backend_batch_limit": limit,
                               "database_cpus": 1, "path": path}
                    selected = matching(rows, filters)
                    label = ("Native" if path == "direct" else "Weir") + ", grouping " + str(limit)
                    draw(ax, selected, "qps", label)
            ax.set_title(("MongoDB" if backend == "mongo" else "Search") + " · " + title)
            finish_axis(ax, (32, 128), "qps")
    axes[0, 0].legend(frameon=False, fontsize=8)
    fig.suptitle("Aggregation ablation · grouping limits 1 and 32 on the same runner", fontsize=14)
    fig.supxlabel("Limit 1 is measured first, then 32. Each has its own paired native baseline; inspect native drift. Bands retain every round.", fontsize=9)
    save(fig, "aggregation-ablation", output)


def bulk(rows, output):
    size = (10.5, 7.4)
    fig, axes = plt.subplots(2, 2, figsize=size, constrained_layout=True)
    for backend_index, backend in enumerate(("mongo", "search")):
        for column, (title, writes) in enumerate((("Read", 0), ("Write", 100))):
            ax = axes[backend_index, column]
            for path in ("direct", "weir"):
                filters = {"backend": backend, "role": "bulk", "requested_write_percent": writes,
                           "batch_size": 32, "database_cpus": 1, "path": path}
                selected = matching(rows, filters)
                draw(ax, selected, "qps", "Native bulk32" if path == "direct" else "Weir SDK32")
            ax.set_title(("MongoDB" if backend == "mongo" else "Search") + " · " + title)
            finish_axis(ax, (4, 16, 64), "qps")
    axes[0, 0].legend(frameon=False)
    fig.suptitle("Native bulk control · 32 records per call · one client process", fontsize=14)
    fig.supxlabel("This comparison measures bulk-call overhead; independent single-record aggregation is measured separately. Bands retain every round.", fontsize=9)
    save(fig, "native-bulk-control", output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--analysis", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    report = json.loads(args.analysis.read_text())
    args.output.mkdir(parents=True, exist_ok=True)
    matplotlib.rcParams["svg.hashsalt"] = "weir-value-2026-10-15"
    rows = report["aggregates"]
    primary(rows, args.output, "qps")
    primary(rows, args.output, "p95")
    ablation(rows, args.output)
    bulk(rows, args.output)


if __name__ == "__main__":
    main()
