from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from site_revision import digest_sources, refresh_tree, write_outputs


class SiteRevisionTest(unittest.TestCase):
    def test_revision_tracks_logical_paths_and_contents(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "page.html"
            source.write_text("first", encoding="utf-8")
            first = digest_sources([f"docs/page.html={source}"])
            source.write_text("second", encoding="utf-8")
            second = digest_sources([f"docs/page.html={source}"])
            renamed = digest_sources([f"docs/renamed.html={source}"])
            self.assertNotEqual(first, second)
            self.assertNotEqual(second, renamed)

    def test_outputs_embed_revision_and_reload_contract(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            revision_output = root / "site-revision.json"
            client_output = root / "site-refresh.js"
            write_outputs("abc123", revision_output, client_output)
            self.assertEqual(json.loads(revision_output.read_text())["revision"], "abc123")
            client = client_output.read_text(encoding="utf-8")
            self.assertIn('const deployedRevision = "abc123"', client)
            self.assertIn('cache: "no-store"', client)
            self.assertIn("window.location.reload()", client)

    def test_tree_revision_tracks_final_report_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            report = root / "reports/tests/latest/data/report.json"
            report.parent.mkdir(parents=True)
            report.write_text('{"status":"first"}\n', encoding="utf-8")
            first = refresh_tree(root)
            report.write_text('{"status":"second"}\n', encoding="utf-8")
            second = refresh_tree(root)
            self.assertNotEqual(first, second)
            self.assertEqual(json.loads((root / "assets/site-revision.json").read_text())["revision"], second)


if __name__ == "__main__":
    unittest.main()
