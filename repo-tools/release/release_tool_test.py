from __future__ import annotations

import hashlib
import json
import tempfile
import unittest
from pathlib import Path

import release_tool


class ReleaseToolTest(unittest.TestCase):
    def test_manifest_is_sorted_and_hashed(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = root / "artifacts"
            artifacts.mkdir()
            (artifacts / "b.whl").write_bytes(b"b")
            (artifacts / "a.whl").write_bytes(b"a")
            original = release_tool.version_inventory
            release_tool.version_inventory = lambda _root: {"hovel": "0.4.0"}
            try:
                release_tool.write_manifest(artifacts, "v0.4.0", root / "manifest.json", root / "SHA256SUMS", root)
            finally:
                release_tool.version_inventory = original
            payload = json.loads((root / "manifest.json").read_text())
            self.assertEqual([item["path"] for item in payload["artifacts"]], ["a.whl", "b.whl"])
            self.assertEqual(payload["artifacts"][0]["sha256"], hashlib.sha256(b"a").hexdigest())

    def test_tag_syntax(self) -> None:
        self.assertEqual(release_tool.VERSION_RE.fullmatch("v0.4.0").group(1), "0.4.0")
        self.assertIsNone(release_tool.VERSION_RE.fullmatch("picblobs-v0.1.7"))

    def test_stage_assets_flattens_and_omits_component_manifests(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            first = root / "first"
            second = root / "second"
            first.mkdir()
            second.mkdir()
            (first / "module.tar.gz").write_bytes(b"module")
            (first / "SHA256SUMS").write_text("component checksums")
            (second / "agent.tar.gz").write_bytes(b"agent")
            output = root / "release"
            release_tool.stage_assets([first, second], output)
            self.assertEqual(sorted(path.name for path in output.iterdir()), ["agent.tar.gz", "module.tar.gz"])


if __name__ == "__main__":
    unittest.main()
