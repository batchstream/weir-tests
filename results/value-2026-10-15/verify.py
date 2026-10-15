#!/usr/bin/env python3
"""Verify retained bytes and complete coverage of the declared workflow matrix."""

import argparse
import gzip
import hashlib
import json
from pathlib import Path


def digest(value):
    return hashlib.sha256(value).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", type=Path, default=Path(__file__).parent)
    args = parser.parse_args()
    directory = args.directory.resolve()
    inventory = json.loads((directory / "raw-inventory.json").read_text())
    for name, expected in inventory.items():
        path = (directory / name).resolve()
        path.relative_to(directory)
        encoded = path.read_bytes()
        raw = gzip.decompress(encoded)
        actual = {"json_or_log_sha256": digest(raw), "original_bytes": len(raw),
                  "gzip_sha256": digest(encoded), "gzip_bytes": len(encoded)}
        if actual != expected:
            raise ValueError("retained evidence differs: " + name)
    retained = {path.relative_to(directory).as_posix() for path in (directory / "raw").rglob("*") if path.is_file()}
    if retained != set(inventory):
        raise ValueError("inventory differs from retained files")
    if json.loads((directory / "collection-failures.json").read_text()):
        raise ValueError("artifact collection failed; see collection-failures.json")
    plan = json.loads((directory / "runs.json").read_text())
    expected_entries = {}
    for run in plan["runs"]:
        encoded = (directory / "raw" / "runs" / str(run["id"]) / "run.json.gz").read_bytes()
        metadata = json.loads(gzip.decompress(encoded))
        if metadata["head_sha"] != plan["client_source"] or metadata["run_attempt"] != 1:
            raise ValueError("workflow source or attempt differs")
        if metadata["status"] != "completed" or metadata["conclusion"] != "success":
            raise ValueError("workflow did not succeed: " + str(run["id"]))
        limits = (1, 32) if run["role"] == "ablation" else (32,)
        for label in run["cases"].values():
            for limit in limits:
                for backend in ("mongo", "search"):
                    key = label + "-g" + str(limit), backend + ".json.gz"
                    if key in expected_entries:
                        raise ValueError("duplicate planned comparison")
                    expected_entries[key] = run["role"], metadata["html_url"]
    entries = json.loads((directory / "manifest.json").read_text())["entries"]
    actual_entries = {}
    for entry in entries:
        key = entry["label"], Path(entry["report"]).name
        if key in actual_entries:
            raise ValueError("duplicate manifest comparison")
        actual_entries[key] = entry["role"], entry["workflow_run"]
    if actual_entries != expected_entries:
        raise ValueError("manifest does not cover the complete declared matrix")
    print(f"Verified {len(inventory)} original files and all {len(entries)} backend reports from {len(plan['runs'])} successful workflows.")


if __name__ == "__main__":
    main()
