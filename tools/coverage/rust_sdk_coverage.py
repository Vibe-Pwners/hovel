"""Collect LLVM source branch coverage for the Rust SDK."""

from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def runfile(path: Path) -> Path:
    if path.is_absolute() and path.exists():
        return path
    runfiles = Path(os.environ["RUNFILES_DIR"])
    for candidate in (runfiles / "_main" / path, runfiles / path):
        if candidate.exists():
            return candidate.resolve()
    raise FileNotFoundError(path)


def test_only_lines(source: Path) -> set[int]:
    """Return lines inside top-level modules guarded by #[cfg(test)]."""
    lines = source.read_text(encoding="utf-8").splitlines()
    excluded: set[int] = set()
    index = 0
    while index < len(lines):
        if lines[index].strip() != "#[cfg(test)]":
            index += 1
            continue
        start = index
        depth = 0
        opened = False
        while index < len(lines):
            depth += lines[index].count("{") - lines[index].count("}")
            opened = opened or "{" in lines[index]
            excluded.add(index + 1)
            index += 1
            if opened and depth == 0:
                break
    return excluded


def main() -> int:
    test_binary, llvm_profdata, llvm_cov = map(
        lambda value: runfile(Path(value)), sys.argv[1:4]
    )
    workspace = Path(os.environ.get("BUILD_WORKSPACE_DIRECTORY", Path.cwd()))
    output = workspace / "coverage"
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="hovel-rust-coverage-") as temporary:
        temporary_path = Path(temporary)
        profile_pattern = temporary_path / "hovel-%p.profraw"
        environment = os.environ.copy()
        environment["LLVM_PROFILE_FILE"] = str(profile_pattern)
        test = subprocess.run([test_binary], env=environment, check=False)
        raw_profiles = sorted(temporary_path.glob("*.profraw"))
        if test.returncode != 0 or not raw_profiles:
            return test.returncode or 1
        profile = temporary_path / "hovel.profdata"
        subprocess.run(
            [llvm_profdata, "merge", "-sparse", *raw_profiles, "-o", profile],
            check=True,
        )
        exported = subprocess.run(
            [llvm_cov, "export", test_binary, f"-instr-profile={profile}"],
            check=True,
            stdout=subprocess.PIPE,
            text=True,
        )
    report = json.loads(exported.stdout)
    (output / "rust-sdk-llvm-export.json").write_text(
        exported.stdout, encoding="utf-8"
    )
    files = []
    edges = []
    covered = total = 0
    for item in report["data"][0]["files"]:
        path = item["filename"].replace("\\", "/")
        marker = "sdk/rust/hovel/src/"
        if marker not in path or path.endswith("/tests.rs"):
            continue
        relative = path[path.index("sdk/rust/"):]
        excluded = test_only_lines(workspace / relative)
        file_covered = file_total = 0
        merged_branches: dict[tuple[int, int, int, int], list[int]] = {}
        for branch in item["branches"]:
            key = tuple(branch[:4])
            counts = merged_branches.setdefault(key, [0, 0])
            counts[0] += branch[4]
            counts[1] += branch[5]
        for (line, column, _, _), (true_count, false_count) in merged_branches.items():
            if line in excluded:
                continue
            file_total += 2
            file_covered += int(true_count > 0) + int(false_count > 0)
            for outcome, count in (("true", true_count), ("false", false_count)):
                edges.append(
                    {
                        "path": relative,
                        "line": line,
                        "column": column,
                        "outcome": outcome,
                        "count": count,
                        "covered": count > 0,
                    }
                )
        files.append(
            {
                "path": relative,
                "covered": file_covered,
                "count": file_total,
                "notcovered": file_total - file_covered,
                "percent": 100.0 if file_total == 0 else file_covered * 100.0 / file_total,
            }
        )
        covered += file_covered
        total += file_total
    detail = {
        "schemaVersion": 1,
        "name": "Rust SDK branch coverage",
        "covered": covered,
        "total": total,
        "percent": 100.0 if total == 0 else covered * 100.0 / total,
        "files": files,
        "edges": edges,
    }
    (output / "rust-sdk-branches.detail.json").write_text(
        json.dumps(detail, indent=2) + "\n", encoding="utf-8"
    )
    metric = [{
        "name": "Rust SDK branches",
        "metric_type": "branch",
        "language": "rust",
        "platforms": ["linux"],
        "covered": covered,
        "total": total,
        "minimum": 100.0,
    }]
    (output / "rust-sdk-branches.results.json").write_text(
        json.dumps(metric, indent=2) + "\n", encoding="utf-8"
    )
    print(f"Rust SDK branches: {covered}/{total} ({detail['percent']:.2f}%)")
    return 0 if total > 0 and covered == total else 1


if __name__ == "__main__":
    raise SystemExit(main())
