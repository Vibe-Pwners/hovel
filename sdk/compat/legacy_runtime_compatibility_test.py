from __future__ import annotations

import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
from pathlib import Path


def resolve(path: str) -> Path:
    candidate = Path(path)
    if candidate.exists():
        return candidate.resolve()
    for root_name in ("RUNFILES_DIR", "TEST_SRCDIR"):
        root = os.environ.get(root_name)
        if not root:
            continue
        for prefix in ("", "_main", "hovel"):
            candidate = Path(root) / prefix / path
            if candidate.exists():
                return candidate.resolve()
    raise AssertionError(f"runfile not found: {path}")


def run(argv: list[str], env: dict[str, str]) -> str:
    result = subprocess.run(argv, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
    if result.returncode:
        raise AssertionError(f"command failed ({result.returncode}): {' '.join(argv)}\n{result.stdout}")
    return result.stdout


def main() -> int:
    if len(sys.argv) != 3:
        raise AssertionError("expected Hovel and frozen module paths")
    hovel, legacy = map(resolve, sys.argv[1:])
    with tempfile.TemporaryDirectory(prefix="hovel-legacy-compat-", dir=tempfile.gettempdir()) as raw:
        root = Path(raw)
        package = root / "package"
        (package / "bin").mkdir(parents=True)
        module = package / "bin" / "legacy-contract-probe"
        shutil.copy2(legacy, module)
        module.chmod(module.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
        (package / "hovel-module.yaml").write_text(
            """apiVersion: hovel.dev/v1alpha1
kind: ModulePackage
metadata:
  name: legacy-contract-probe
  version: 0.3.2
launch:
  - selector:
      os: linux
      arch: amd64
    command: ["bin/legacy-contract-probe"]
"""
        )
        empty_config = root / "modules.json"
        empty_config.write_text(json.dumps({"modules": []}) + "\n")
        workspace = root / "workspace"
        env = os.environ | {"HOVEL_MODULE_CONFIG": str(empty_config)}
        run([str(hovel), "init", "--workspace", str(workspace), "--json"], env)
        run([str(hovel), "module", "install", "--link", str(package), "--workspace", str(workspace)], env)
        listing = run([str(hovel), "module", "list", "--workspace", str(workspace)], env)
        # The handshake is authoritative over the package hint, as it was for
        # deployed 0.3.2 modules.
        assert "contract-probe@v0.3.2-compat" in listing, listing
        checked = run([str(hovel), "module", "check", "contract-probe", "--workspace", str(workspace)], env)
        assert "PASS" in checked and "config schema" in checked and "step contracts" in checked, checked
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
