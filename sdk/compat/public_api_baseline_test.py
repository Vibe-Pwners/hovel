"""Protect the pre-1.0 SDK source API from subtractive changes."""

from __future__ import annotations

import ast
import json
import os
import re
import sys
from pathlib import Path


def root() -> Path:
    workspace = os.environ.get("BUILD_WORKSPACE_DIRECTORY")
    if workspace:
        return Path(workspace)
    here = Path(__file__).resolve()
    for candidate in (here, *here.parents):
        if (candidate / "sdk").is_dir() and (candidate / "MODULE.bazel").is_file():
            return candidate
    raise RuntimeError("cannot locate repository root")


def normalized(text: str) -> str:
    return " ".join(text.split())


def python_api(repo: Path) -> dict[str, str]:
    result: dict[str, str] = {}
    package = repo / "sdk/python/hovel_sdk"
    for path in sorted(package.glob("*.py")):
        if path.name.endswith("_test.py"):
            continue
        tree = ast.parse(path.read_text(encoding="utf-8"))
        module = path.stem
        for node in tree.body:
            name = getattr(node, "name", "")
            if not isinstance(name, str) or not name or name.startswith("_"):
                continue
            if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                result[f"python:{module}.{name}"] = normalized(
                    f"{name}{ast.unparse(node.args)} -> {ast.unparse(node.returns) if node.returns else ''}"
                )
            elif isinstance(node, ast.ClassDef):
                fields = []
                methods = []
                for child in node.body:
                    if isinstance(child, ast.AnnAssign) and isinstance(child.target, ast.Name):
                        fields.append(f"{child.target.id}:{ast.unparse(child.annotation)}")
                    elif isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef)) and not child.name.startswith("_"):
                        methods.append(f"{child.name}{ast.unparse(child.args)}")
                bases = ",".join(ast.unparse(base) for base in node.bases)
                result[f"python:{module}.{name}"] = normalized(f"class {name}({bases}) {';'.join(fields + methods)}")
    return result


def declaration_blocks(text: str, pattern: re.Pattern[str]) -> list[str]:
    lines = text.splitlines()
    blocks: list[str] = []
    index = 0
    while index < len(lines):
        line = lines[index]
        if not pattern.match(line):
            index += 1
            continue
        block = [line]
        balance = line.count("{") + line.count("(") - line.count("}") - line.count(")")
        if line.lstrip().startswith(("func ", "pub fn ")) and "{" in line:
            blocks.append(line.split("{", 1)[0])
            index += 1
            continue
        while balance > 0 and index + 1 < len(lines):
            index += 1
            block.append(lines[index])
            balance += lines[index].count("{") + lines[index].count("(") - lines[index].count("}") - lines[index].count(")")
        blocks.append("\n".join(block))
        index += 1
    return blocks


def go_api(repo: Path) -> dict[str, str]:
    result: dict[str, str] = {}
    pattern = re.compile(r"^(?:type|const|var) [A-Z]|^func (?:\([^)]*\) )?[A-Z]")
    for path in sorted((repo / "sdk/go/hovel").glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        for block in declaration_blocks(path.read_text(encoding="utf-8"), pattern):
            method = re.match(r"func \([^)]*\*?([A-Z][A-Za-z0-9_]*)\) ([A-Z][A-Za-z0-9_]*)", block)
            declaration = re.match(r"(?:type|func|const|var) ([A-Z][A-Za-z0-9_]*)", block)
            if method:
                result[f"go:{method.group(1)}.{method.group(2)}"] = normalized(block)
            elif declaration:
                result[f"go:{declaration.group(1)}"] = normalized(block)
    return result


def rust_api(repo: Path) -> dict[str, str]:
    result: dict[str, str] = {}
    pattern = re.compile(r"^\s*pub (?:async )?(?:use|const|static|type|struct|enum|trait|fn) ")
    for path in sorted((repo / "sdk/rust/hovel/src").glob("*.rs")):
        if path.name == "tests.rs":
            continue
        for block in declaration_blocks(path.read_text(encoding="utf-8"), pattern):
            key = normalized(block.split("{", 1)[0].rstrip(";"))
            result[f"rust:{path.stem}:{key}"] = normalized(block)
    return result


def current_api(repo: Path) -> dict[str, str]:
    return dict(sorted((python_api(repo) | go_api(repo) | rust_api(repo)).items()))


def main() -> int:
    repo = root()
    current = current_api(repo)
    if "--print" in sys.argv:
        print(json.dumps(current, indent=2, sort_keys=True))
        return 0
    baseline = json.loads((repo / "sdk/compat/public_api_baseline.json").read_text(encoding="utf-8"))
    failures = []
    for symbol, signature in baseline.items():
        if symbol not in current:
            failures.append(f"removed public API: {symbol}")
        elif current[symbol] != signature:
            failures.append(f"changed public API: {symbol}\n  was: {signature}\n  now: {current[symbol]}")
    if failures:
        raise AssertionError("\n".join(failures))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
