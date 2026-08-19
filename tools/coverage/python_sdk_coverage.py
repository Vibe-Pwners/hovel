"""Collect strict line and branch coverage for the public Python SDK."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path

import coverage
import pytest


MINIMUM = 100.0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--no-enforce", action="store_true", help="Write evidence without failing below 100%.")
    args = parser.parse_args()
    workspace = Path(os.environ["BUILD_WORKSPACE_DIRECTORY"])
    sdk = workspace / "sdk/python"
    package = sdk / "hovel_sdk"
    output = workspace / "coverage"
    output.mkdir(exist_ok=True)
    raw_path = output / "python-sdk.raw.json"
    result_path = output / "python-sdk.results.json"

    collector = coverage.Coverage(
        branch=True,
        source=[str(package)],
        omit=[str(package / "*_test.py")],
        data_file=None,
    )
    collector.start()
    test_status = pytest.main(
        [
            "-q",
            str(package / "sdk_test.py"),
            str(package / "mesh_bridge_test.py"),
            str(package / "coverage_test.py"),
        ]
    )
    collector.stop()
    collector.json_report(outfile=str(raw_path), pretty_print=True)

    raw = json.loads(raw_path.read_text(encoding="utf-8"))
    totals = raw["totals"]
    metrics = [
        metric("Python SDK lines", "line", totals["covered_lines"], totals["num_statements"]),
        metric("Python SDK branches", "branch", totals["covered_branches"], totals["num_branches"]),
    ]
    result_path.write_text(json.dumps(metrics, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    for item in metrics:
        percentage = 100.0 * item["covered"] / item["total"] if item["total"] else 0.0
        print(f'{item["name"]}: {item["covered"]}/{item["total"]} ({percentage:.2f}%)')
    tests_passed = test_status == pytest.ExitCode.OK
    coverage_passed = all(item["covered"] == item["total"] for item in metrics)
    return 0 if tests_passed and (coverage_passed or args.no_enforce) else 1


def metric(name: str, metric_type: str, covered: int, total: int) -> dict[str, object]:
    return {
        "name": name,
        "metric_type": metric_type,
        "language": "python",
        "platforms": ["linux"],
        "covered": covered,
        "total": total,
        "minimum": MINIMUM,
    }


if __name__ == "__main__":
    raise SystemExit(main())
