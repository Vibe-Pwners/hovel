from __future__ import annotations

import re
import sys
from pathlib import Path


FILES = {
    (f"{Path(argument).parent.name}/{Path(argument).name}" if Path(argument).name == "action.yml" else Path(argument).name): Path(
        argument
    ).read_text(encoding="utf-8")
    for argument in sys.argv[1:]
}
PUBLISHERS = {
    "hovel-pypi.yml": ("hovel", "pypi", "startsWith(github.ref_name, 'v')"),
    "hovel-sdk-pypi.yml": ("hovel-sdk", "pypi", "startsWith(github.ref_name, 'v')"),
    "picblobs-pypi.yml": ("picblobs", "pypi-picblobs", "startsWith(github.ref_name, 'picblobs-v')"),
    "picblobs-cli-pypi.yml": (
        "picblobs-cli",
        "pypi-picblobs-cli",
        "startsWith(github.ref_name, 'picblobs-cli-v')",
    ),
}


def test_every_remote_action_is_pinned_by_commit() -> None:
    for name, content in FILES.items():
        for action in re.findall(r"(?m)^\s*-?\s*uses:\s*([^\s#]+)", content):
            if action.startswith("./"):
                continue
            assert re.search(r"@[0-9a-f]{40}$", action), (name, action)


def test_publishers_keep_stable_trusted_publisher_identity() -> None:
    for filename, (package, environment, tag_guard) in PUBLISHERS.items():
        workflow = FILES[filename]
        assert f"package: {package}" in workflow
        assert f"environment: {environment}" in workflow
        assert tag_guard in workflow
        assert "id-token: write" in workflow
        assert "pypa/gh-action-pypi-publish@" in workflow
        assert "./.github/actions/build-python-package" in workflow


def test_shared_release_action_uses_only_aspect_build_entry_points() -> None:
    action = FILES["build-python-package/action.yml"]
    for package in PUBLISHERS.values():
        assert package[0] in action
    assert "aspect hovel-release hovel" in action
    assert "aspect hovel-release sdk" in action
    assert "aspect hovel-release picblobs" in action
    assert "aspect hovel-release picblobs-cli" in action
    assert "pip install" not in action
    assert "setup-python" not in action
    assert "setup-go" not in action


def test_repository_workflows_share_one_setup_action() -> None:
    for filename in ("ci.yml", "modules-release.yml", "pages.yml"):
        assert "./.github/actions/setup-hovel" in FILES[filename]
        assert "aspect-build/setup-aspect@" not in FILES[filename]
        assert "actions/cache@" not in FILES[filename]
