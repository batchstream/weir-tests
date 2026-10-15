#!/usr/bin/env python3
"""Download this experiment's completed runs; preserve failed runs and originals."""

import argparse
import json
from pathlib import Path
import subprocess
import tempfile


REPOSITORY = "batchstream/weir-tests"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--runs", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    plan = json.loads(args.runs.read_text())
    args.output.mkdir(parents=True, exist_ok=True)
    states = []
    pending = False
    for run in plan["runs"]:
        identity = str(run["id"])
        raw = subprocess.check_output(["gh", "api", "repos/" + REPOSITORY + "/actions/runs/" + identity], text=True)
        info = json.loads(raw)
        if info["head_sha"] != plan["client_source"] or info["run_attempt"] != 1:
            raise ValueError("unexpected source or attempt for run " + identity)
        state = {key: info[key] for key in ("id", "html_url", "head_sha", "run_attempt", "event", "status", "conclusion", "created_at", "updated_at")}
        states.append(state)
        if info["status"] != "completed":
            pending = True
            print(identity + " pending", flush=True)
            continue
        target = args.output / identity
        if not target.exists():
            with tempfile.TemporaryDirectory(prefix=identity + "-", dir=args.output) as temporary:
                stage = Path(temporary) / "artifacts"
                command = ["gh", "run", "download", identity, "--repo", REPOSITORY, "--dir", str(stage)]
                downloaded = subprocess.run(command, check=False)
                if downloaded.returncode and info["conclusion"] == "success":
                    raise ValueError("successful run missing artifacts: " + identity)
                stage.mkdir(parents=True, exist_ok=True)
                metadata = dict(state)
                metadata["artifacts_download_exit_code"] = downloaded.returncode
                (stage / "run.json").write_text(json.dumps(metadata, indent=2) + "\n")
                with (stage / "workflow.log").open("w") as log:
                    subprocess.run(["gh", "run", "view", identity, "--repo", REPOSITORY, "--log"], stdout=log, check=True)
                stage.rename(target)
        print(identity + " retained: " + info["conclusion"], flush=True)
    (args.output / "run-states.json").write_text(json.dumps(states, indent=2) + "\n")
    return 2 if pending else 0


if __name__ == "__main__":
    raise SystemExit(main())
