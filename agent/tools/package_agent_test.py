from __future__ import annotations

import hashlib
import importlib.util
import tarfile
import tempfile
import unittest
from pathlib import Path


def load_packager():
    path = Path(__file__).with_name("package_agent.py")
    spec = importlib.util.spec_from_file_location("package_agent", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


package_agent = load_packager()


class PackageAgentTest(unittest.TestCase):
    def test_validate(self) -> None:
        package_agent.validate()

    def test_packages_are_reproducible_and_complete(self) -> None:
        with tempfile.TemporaryDirectory() as first, tempfile.TemporaryDirectory() as second:
            first_path = Path(first)
            second_path = Path(second)
            package_agent.build(first_path, clean=True)
            package_agent.build(second_path, clean=True)
            first_files = sorted(path.name for path in first_path.iterdir())
            self.assertEqual(first_files, sorted(path.name for path in second_path.iterdir()))
            for name in first_files:
                self.assertEqual(
                    hashlib.sha256((first_path / name).read_bytes()).digest(),
                    hashlib.sha256((second_path / name).read_bytes()).digest(),
                )
            expected = set(package_agent.EXPECTED_SKILLS)
            for archive_path in first_path.glob("*.tar.gz"):
                with tarfile.open(archive_path, "r:gz") as archive:
                    names = set(archive.getnames())
                    skill_names = {
                        Path(name).parent.name
                        for name in names
                        if name.endswith("/SKILL.md")
                    }
                    self.assertEqual(skill_names, expected)
                    for member in archive.getmembers():
                        self.assertFalse(member.issym())
                        self.assertFalse(member.name.startswith("/"))
                        self.assertNotIn("..", Path(member.name).parts)


if __name__ == "__main__":
    unittest.main()
