#!/usr/bin/env python3
"""Validate and deterministically package Hovel agent integrations."""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
import re
import shutil
import tarfile
import tempfile
from pathlib import Path


ROOT = Path(os.environ.get("BUILD_WORKSPACE_DIRECTORY", Path(__file__).resolve().parents[2])).resolve()
AGENT = ROOT / "agent"
SKILLS = AGENT / "skills"
NAME_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
EXPECTED_SKILLS = (
    "hovel",
    "hovel-artifacts",
    "hovel-chain-building",
    "hovel-discovery",
    "hovel-payloads",
    "hovel-sessions",
    "hovel-throw",
    "hovel-troubleshooting",
)
REQUIRED_THROW_PHRASES = (
    "hovel_throw_plan",
    "hovel_throw_confirm",
    "hovel_throw_start",
    "plan hash",
    "dangerous",
    "launch-key",
)


def version() -> str:
    return (ROOT / "VERSION").read_text(encoding="utf-8").strip()


def parse_frontmatter(path: Path) -> dict[str, object]:
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---\n") or "\n---\n" not in text[4:]:
        raise ValueError(f"{path}: missing YAML frontmatter")
    raw, _body = text[4:].split("\n---\n", 1)
    result: dict[str, object] = {}
    current_map: dict[str, str] | None = None
    for line in raw.splitlines():
        if line.startswith("  ") and current_map is not None:
            key, value = line.strip().split(":", 1)
            current_map[key.strip()] = value.strip().strip('"')
            continue
        key, value = line.split(":", 1)
        if value.strip():
            result[key.strip()] = value.strip()
            current_map = None
        else:
            current_map = {}
            result[key.strip()] = current_map
    return result


def validate() -> None:
    current = version()
    major, minor, _patch = (int(part) for part in current.split("."))
    expected_minimum = f"{major}.{minor}.0"
    expected_maximum = f"{major}.{minor + 1}.0" if major == 0 else f"{major + 1}.0.0"
    found = tuple(sorted(path.parent.name for path in SKILLS.glob("*/SKILL.md")))
    if found != EXPECTED_SKILLS:
        raise ValueError(f"skill set = {found!r}, want {EXPECTED_SKILLS!r}")
    for name in found:
        path = SKILLS / name / "SKILL.md"
        frontmatter = parse_frontmatter(path)
        if not NAME_RE.fullmatch(name) or frontmatter.get("name") != name:
            raise ValueError(f"{path}: name must match its directory")
        description = str(frontmatter.get("description", ""))
        if not 1 <= len(description) <= 1024:
            raise ValueError(f"{path}: description must contain 1-1024 characters")
        if frontmatter.get("license") != "Apache-2.0":
            raise ValueError(f"{path}: license must be Apache-2.0")
        metadata = frontmatter.get("metadata")
        if not isinstance(metadata, dict):
            raise ValueError(f"{path}: missing Hovel compatibility metadata")
        if metadata.get("hovel-min-version") != expected_minimum or metadata.get("hovel-max-version") != expected_maximum:
            raise ValueError(f"{path}: Hovel compatibility metadata does not match VERSION")
        if f"Requires Hovel {major}.{minor}.x" not in str(frontmatter.get("compatibility", "")):
            raise ValueError(f"{path}: compatibility description does not match VERSION")
    throw_text = (SKILLS / "hovel-throw" / "SKILL.md").read_text(encoding="utf-8")
    for phrase in REQUIRED_THROW_PHRASES:
        if phrase not in throw_text:
            raise ValueError(f"hovel-throw skill is missing required safety phrase {phrase!r}")
    plugin = json.loads((AGENT / ".claude-plugin" / "plugin.json").read_text(encoding="utf-8"))
    if plugin.get("version") != current:
        raise ValueError("Claude plugin version must match VERSION")
    marketplace = json.loads((ROOT / ".claude-plugin" / "marketplace.json").read_text(encoding="utf-8"))
    if marketplace.get("name") != "vibepwners-hovel":
        raise ValueError("Claude marketplace name must be vibepwners-hovel")


def copy_skills(destination: Path) -> None:
    shutil.copytree(SKILLS, destination / "skills")


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=False) + "\n", encoding="utf-8")


def write_package_manifest(root: Path, host: str) -> None:
    write_json(root / "hovel-agent.json", {"name": "hovel", "version": version(), "host": host})


def stage_claude(root: Path) -> None:
    write_package_manifest(root, "claude")
    plugin = root / "plugins" / "hovel"
    copy_skills(plugin)
    shutil.copy2(AGENT / ".mcp.json", plugin / ".mcp.json")
    shutil.copytree(AGENT / ".claude-plugin", plugin / ".claude-plugin")
    shutil.copy2(ROOT / "LICENSE", plugin / "LICENSE")
    marketplace = json.loads((ROOT / ".claude-plugin" / "marketplace.json").read_text(encoding="utf-8"))
    marketplace["plugins"][0]["source"] = "./plugins/hovel"
    write_json(root / ".claude-plugin" / "marketplace.json", marketplace)


def codex_manifest() -> dict[str, object]:
    return {
        "name": "hovel",
        "version": version(),
        "description": "Operate Hovel safely and effectively through its typed MCP interface.",
        "author": {"name": "Vibe Pwners", "url": "https://github.com/vibepwners"},
        "homepage": "https://vibepwners.github.io/hovel/spec/agent-integrations.html",
        "repository": "https://github.com/vibepwners/hovel",
        "license": "Apache-2.0",
        "keywords": ["hovel", "mcp", "security", "agent-skills"],
        "skills": "./skills/",
        "mcpServers": "./.mcp.json",
        "interface": {
            "displayName": "Hovel",
            "shortDescription": "Official Hovel operator skills",
            "longDescription": "Operate Hovel through typed MCP workflows with planning, approval, payload, session, artifact, and recovery guidance.",
            "developerName": "Vibe Pwners",
            "category": "Productivity",
            "capabilities": ["Interactive", "Write"],
            "websiteURL": "https://vibepwners.github.io/hovel/",
            "defaultPrompt": [
                "Inspect my current Hovel workspace.",
                "Build and validate a Hovel chain.",
                "Review this Hovel throw plan.",
            ],
        },
    }


def stage_codex(root: Path) -> None:
    write_package_manifest(root, "codex")
    plugin = root / ".agents" / "plugins" / "plugins" / "hovel"
    copy_skills(plugin)
    shutil.copy2(AGENT / ".mcp.json", plugin / ".mcp.json")
    shutil.copy2(ROOT / "LICENSE", plugin / "LICENSE")
    write_json(plugin / ".codex-plugin" / "plugin.json", codex_manifest())
    write_json(
        root / ".agents" / "plugins" / "marketplace.json",
        {
            "name": "vibepwners-hovel",
            "interface": {"displayName": "Hovel"},
            "plugins": [
                {
                    "name": "hovel",
                    "source": {"source": "local", "path": "./plugins/hovel"},
                    "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
                    "category": "Productivity",
                }
            ],
        },
    )


def stage_opencode(root: Path) -> None:
    write_package_manifest(root, "opencode")
    copy_skills(root / ".opencode")
    write_json(
        root / "opencode.hovel.json",
        {
            "$schema": "https://opencode.ai/config.json",
            "mcp": {"hovel": {"type": "local", "command": ["hovel", "mcp"], "enabled": True}},
        },
    )
    shutil.copy2(ROOT / "LICENSE", root / "LICENSE")


def archive_tree(source: Path, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    with destination.open("wb") as raw, gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as gz:
        with tarfile.open(fileobj=gz, mode="w") as archive:
            for path in sorted(source.rglob("*"), key=lambda item: item.relative_to(source).as_posix()):
                relative = path.relative_to(source).as_posix()
                info = archive.gettarinfo(str(path), relative)
                info.uid = info.gid = 0
                info.uname = info.gname = ""
                info.mtime = 0
                info.mode = 0o755 if path.is_dir() else 0o644
                if path.is_file():
                    with path.open("rb") as stream:
                        archive.addfile(info, stream)
                else:
                    archive.addfile(info)


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def build(out_dir: Path, clean: bool) -> None:
    validate()
    if clean and out_dir.exists():
        shutil.rmtree(out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    outputs: list[Path] = []
    with tempfile.TemporaryDirectory() as temporary:
        temporary_root = Path(temporary)
        for host, stage in (("claude", stage_claude), ("codex", stage_codex), ("opencode", stage_opencode)):
            staged = temporary_root / host
            staged.mkdir()
            stage(staged)
            output = out_dir / f"hovel-agent-{host}-v{version()}.tar.gz"
            archive_tree(staged, output)
            outputs.append(output)
    checksums = "".join(f"{digest(path)}  {path.name}\n" for path in sorted(outputs))
    (out_dir / "SHA256SUMS").write_text(checksums, encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--validate", action="store_true")
    parser.add_argument("--out-dir", type=Path, default=ROOT / "dist" / "agent")
    parser.add_argument("--clean", action="store_true")
    args = parser.parse_args()
    if args.validate:
        validate()
    else:
        build(args.out_dir.resolve(), args.clean)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
