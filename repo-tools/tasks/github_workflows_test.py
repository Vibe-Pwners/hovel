from __future__ import annotations

import re
import sys
from pathlib import Path


FILES = {Path(argument).name: Path(argument).read_text(encoding="utf-8") for argument in sys.argv[1:]}


def test_every_remote_action_is_pinned_by_commit() -> None:
    for name, content in FILES.items():
        for action in re.findall(r"(?m)^\s*-?\s*uses:\s*([^\s#]+)", content):
            if action.startswith("./"):
                continue
            assert re.search(r"@[0-9a-f]{40}$", action), (name, action)


def test_release_is_tag_driven_and_ordered() -> None:
    workflow = FILES["release.yml"]
    assert 'tags: ["v*"]' in workflow
    assert "release:" not in workflow.split("permissions:", 1)[0]
    assert "aspect hovel-check" in workflow
    assert "needs: [verify, build-picblobs, publish-picblobs]" in workflow
    assert "needs: [publish-hovel, publish-sdk, publish-picblobs-cli, build-modules, build-agent]" in workflow
    assert workflow.index("publish-picblobs:") < workflow.index("publish-picblobs-cli:")
    assert workflow.index("publish-picblobs-cli:") < workflow.index("github-release:")


def test_release_keeps_publisher_identities_and_minimal_permissions() -> None:
    workflow = FILES["release.yml"]
    for environment in ("pypi", "pypi-picblobs", "pypi-picblobs-cli"):
        assert f"environment: {environment}" in workflow
    assert workflow.count("id-token: write") == 4
    assert workflow.count("contents: write") == 1
    assert workflow.count("pypa/gh-action-pypi-publish@") == 4
    assert workflow.count("skip-existing: true") == 4
    assert workflow.count("attestations: true") == 4
    assert workflow.count("needs.verify.outputs.picblobs-changed == 'true'") == 4


def test_release_builds_and_smokes_only_through_aspect() -> None:
    workflow = FILES["release.yml"]
    for kind in ("hovel", "sdk", "picblobs-cli", "modules", "agent"):
        assert f"aspect hovel-release {kind}" in workflow
    assert workflow.count("release_tool -- smoke") == 4
    assert "release_tool -- stage-assets" in workflow
    assert "release_tool -- manifest" in workflow
    assert workflow.count("release_tool -- verify-pypi") == 4
    assert "pip install" not in workflow
    assert "setup-python" not in workflow
    assert "setup-go" not in workflow


def test_repository_workflows_share_one_setup_action() -> None:
    for filename in ("ci.yml", "pages.yml", "release.yml"):
        assert "./.github/actions/setup-hovel" in FILES[filename]
        assert "aspect-build/setup-aspect@" not in FILES[filename]
        assert "actions/cache@" not in FILES[filename]
