#!/usr/bin/env python3
"""Generate deterministic revision data and the Pages live-refresh client."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path


REVISION_RELATIVE = Path("assets/site-revision.json")
CLIENT_RELATIVE = Path("assets/site-refresh.js")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", action="append", default=[])
    parser.add_argument("--revision-output", type=Path, required=True)
    parser.add_argument("--client-output", type=Path, required=True)
    return parser.parse_args()


def digest_sources(values: list[str]) -> str:
    digest = hashlib.sha256()
    sources = sorted((value.split("=", 1) for value in values), key=lambda item: item[0])
    for logical_path, physical_path in sources:
        digest.update(logical_path.encode("utf-8"))
        digest.update(b"\0")
        digest.update(Path(physical_path).read_bytes())
        digest.update(b"\0")
    return digest.hexdigest()


def digest_tree(root: Path) -> str:
    digest = hashlib.sha256()
    excluded = {REVISION_RELATIVE, CLIENT_RELATIVE}
    sources = sorted(path for path in root.rglob("*") if path.is_file() and path.relative_to(root) not in excluded)
    for source in sources:
        relative = source.relative_to(root).as_posix()
        digest.update(relative.encode("utf-8"))
        digest.update(b"\0")
        digest.update(source.read_bytes())
        digest.update(b"\0")
    return digest.hexdigest()


def refresh_tree(root: Path) -> str:
    revision = digest_tree(root)
    write_outputs(revision, root / REVISION_RELATIVE, root / CLIENT_RELATIVE)
    return revision


def write_outputs(revision: str, revision_output: Path, client_output: Path) -> None:
    revision_output.parent.mkdir(parents=True, exist_ok=True)
    client_output.parent.mkdir(parents=True, exist_ok=True)
    revision_output.write_text(
        json.dumps({"schemaVersion": "hovel.site-revision/v1", "revision": revision}, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    client_output.write_text(
        f"""(function () {{
  const deployedRevision = {json.dumps(revision)};
  const script = document.currentScript;
  if (!script || !script.src) return;
  const revisionURL = new URL("site-revision.json", script.src);
  let refreshing = false;

  async function refreshWhenDeployed() {{
    if (refreshing || document.visibilityState === "hidden") return;
    revisionURL.searchParams.set("refresh", String(Date.now()));
    try {{
      const response = await fetch(revisionURL, {{ cache: "no-store" }});
      if (!response.ok) return;
      const current = await response.json();
      if (current.revision && current.revision !== deployedRevision) {{
        refreshing = true;
        window.location.reload();
      }}
    }} catch (_) {{
      // A deployment can briefly replace the Pages artifact between requests.
    }}
  }}

  window.setInterval(refreshWhenDeployed, 15000);
  document.addEventListener("visibilitychange", refreshWhenDeployed);
}})();
""",
        encoding="utf-8",
    )


def main() -> None:
    args = parse_args()
    revision = digest_sources(args.source)
    write_outputs(revision, args.revision_output, args.client_output)


if __name__ == "__main__":
    main()
