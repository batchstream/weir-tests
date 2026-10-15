#!/usr/bin/env python3
"""Pack original benchmark artifacts with deterministic gzip and input hashes."""

import argparse
import gzip
import hashlib
import json
from pathlib import Path


def store(source, destination, inventory):
    raw = source.read_bytes()
    encoded = gzip.compress(raw, mtime=0)
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(encoded)
    inventory[destination.as_posix()] = {
        "json_or_log_sha256": hashlib.sha256(raw).hexdigest(),
        "original_bytes": len(raw), "gzip_sha256": hashlib.sha256(encoded).hexdigest(),
        "gzip_bytes": len(encoded),
    }


def argument(command, name):
    return command[command.index(name) + 1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--runs", required=True, type=Path)
    parser.add_argument("--downloads", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    plan = json.loads(args.runs.read_text())
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    entries, inventory, failures = [], {}, []
    for run in plan["runs"]:
        original = args.downloads.resolve() / str(run["id"])
        metadata = json.loads((original / "run.json").read_text())
        if metadata["head_sha"] != plan["client_source"] or metadata["status"] != "completed":
            raise ValueError("source or completion state differs for " + str(run["id"]))
        retained_run = output / "raw" / "runs" / str(run["id"])
        for name in ("run.json", "workflow.log"):
            store(original / name, retained_run / (name + ".gz"), inventory)
        receipts = {}
        for path in original.rglob("measurement-batch-*.receipt.json"):
            receipt = json.loads(path.read_text())
            key = (argument(receipt["command"], "-write-percent"),
                   argument(receipt["command"], "-backend-batch-limit"))
            if key in receipts:
                raise ValueError("duplicate workload receipt " + str(key))
            receipts[key] = path
        limits = ("1", "32") if run["role"] == "ablation" else ("32",)
        expected = {(writes, limit) for writes in run["cases"] for limit in limits}
        if set(receipts) - expected:
            raise ValueError("unexpected workload receipt for " + str(run["id"]))
        for writes, limit in sorted(expected):
            label = run["cases"][writes] + "-g" + limit
            directory = output / "raw" / label
            receipt_path = receipts.get((writes, limit))
            retained_receipt = directory / "measurement.receipt.json.gz"
            if receipt_path is not None:
                store(receipt_path, retained_receipt, inventory)
            else:
                failure = {"run": run["id"], "label": label, "failure": "missing execution receipt"}
                failures.append(failure)
            for backend in ("mongo", "search"):
                retained_report = directory / (backend + ".json.gz")
                fixture_directory = directory / "fixture"
                entry = {"label": label, "role": run["role"], "workflow_run": metadata["html_url"],
                         "receipt": retained_receipt.relative_to(output).as_posix(),
                         "report": retained_report.relative_to(output).as_posix(),
                         "fixture_manifest": (fixture_directory / "manifest.json.gz").relative_to(output).as_posix(),
                         "cleanup": (fixture_directory / "cleanup.json.gz").relative_to(output).as_posix()}
                entries.append(entry)
                if receipt_path is None:
                    continue
                report_path = receipt_path.parent / ("saturation-batch-" + limit) / (backend + ".json")
                if not report_path.exists():
                    failure = {"run": run["id"], "label": label, "backend": backend, "failure": "missing report"}
                    failures.append(failure)
                    continue
                store(report_path, retained_report, inventory)
                report = json.loads(report_path.read_text())
                owner = Path(report["provenance"]["fixture_logs"]).name
                candidates = [path.parent for path in original.rglob("manifest.json") if path.parent.name == owner]
                if len(candidates) != 1:
                    failure = {"run": run["id"], "label": label, "backend": backend, "failure": "missing/ambiguous owned fixture"}
                    failures.append(failure)
                    continue
                for path in candidates[0].iterdir():
                    if path.is_file() and path.suffix in (".json", ".log", ".txt"):
                        store(path, fixture_directory / (path.name + ".gz"), inventory)
    normalized_inventory = {str(Path(name).relative_to(output)): value for name, value in sorted(inventory.items())}
    manifest = {"entries": entries}
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    (output / "raw-inventory.json").write_text(json.dumps(normalized_inventory, indent=2) + "\n")
    (output / "collection-failures.json").write_text(json.dumps(failures, indent=2) + "\n")
    status = {"entries": len(entries), "retained_files": len(inventory), "collection_failures": len(failures)}
    print(json.dumps(status))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
