"""Regression checks for the local admission runner's evidence boundary."""

import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location(
    "ai_admission_runner", Path(__file__).with_name("test-ai-admission.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class EvidenceDirectoryTests(unittest.TestCase):
    def test_new_relative_directory_becomes_private_absolute_path(self):
        with tempfile.TemporaryDirectory(dir=Path.cwd()) as temporary:
            relative = Path(temporary).relative_to(Path.cwd()) / "new" / "evidence"
            evidence = runner.evidence_directory(relative)
            self.assertEqual(evidence, Path.cwd() / relative)
            self.assertEqual(evidence.stat().st_mode & 0o777, 0o700)
            self.assertEqual(list(evidence.iterdir()), [])

    def test_default_directory_is_private_and_fresh(self):
        evidence = runner.evidence_directory(None)
        try:
            self.assertTrue(evidence.is_absolute())
            self.assertEqual(evidence.stat().st_mode & 0o777, 0o700)
            self.assertEqual(list(evidence.iterdir()), [])
        finally:
            evidence.rmdir()

    def test_symlink_directory_is_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            link = root / "evidence"
            link.symlink_to(root / "missing-target", target_is_directory=True)
            with self.assertRaises(FileExistsError):
                runner.evidence_directory(link)
            self.assertFalse((root / "missing-target").exists())

    def test_existing_setup_log_cannot_overwrite_another_file(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            sentinel = root / "sentinel"
            sentinel.write_text("preserve me")
            evidence = root / "evidence"
            evidence.mkdir()
            (evidence / "postgres-setup.log").symlink_to(sentinel)
            with patch("sys.argv", ["runner", "--evidence", str(evidence)]), \
                    patch.object(runner, "binary", return_value="unused") as binary, \
                    patch.object(runner, "command", side_effect=RuntimeError("unexpected subprocess")):
                with self.assertRaises(FileExistsError):
                    runner.main()
                binary.assert_not_called()
            self.assertEqual(sentinel.read_text(), "preserve me")

    def test_dangling_go_log_symlink_cannot_create_another_file(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            target = root / "must-not-be-created"
            evidence = root / "evidence"
            evidence.mkdir()
            (evidence / "go-tests.jsonl").symlink_to(target)
            with patch("sys.argv", ["runner", "--evidence", str(evidence)]), \
                    patch.object(runner, "binary", return_value="unused") as binary, \
                    patch.object(runner, "command", side_effect=RuntimeError("unexpected subprocess")):
                with self.assertRaises(FileExistsError):
                    runner.main()
                binary.assert_not_called()
            self.assertFalse(target.exists())


if __name__ == "__main__":
    unittest.main()
