#!/usr/bin/env python3
"""Verify package source materialization in an owned disposable Git fixture."""

import json
import os
from pathlib import Path
import runpy
import stat
import subprocess
import tempfile
from unittest.mock import patch


PACKAGER = runpy.run_path(str(Path(__file__).with_name("package-closed-pilot.py")))


def fixture_git(repo, *args):
    return subprocess.run(["rtk", "proxy", *PACKAGER["GIT"], *args], cwd=repo, env=PACKAGER["git_environment"](),
                          check=True, stdout=subprocess.PIPE).stdout.decode().strip()


def commit(repo, message):
    fixture_git(repo, "add", "--all")
    fixture_git(repo, "-c", "user.name=Synthetic Fixture", "-c", "user.email=fixture@example.invalid",
                "commit", "-q", "-m", message)


def check_source():
    results = {}
    with tempfile.TemporaryDirectory(prefix="closed-pilot-git-fixture-") as temp:
        root = Path(temp)
        repo = root / "repository"
        repo.mkdir()
        fixture_git(repo, "init", "-q", "--template=")
        (repo / ".gitattributes").write_text("content.txt export-ignore\nrevision.txt export-subst\n")
        (repo / "content.txt").write_text("committed fixture\n")
        (repo / "revision.txt").write_text("$Format:%H$\n")
        (repo / "run.sh").write_text("#!/bin/sh\ntrue\n")
        (repo / "run.sh").chmod(0o755)
        (repo / "bin").mkdir()
        (repo / "bin/tool").symlink_to("../run.sh")
        commit(repo, "Write regular and executable fixture sources")
        expected_commit = fixture_git(repo, "rev-parse", "HEAD")
        expected_tree = fixture_git(repo, "rev-parse", "HEAD^{tree}")

        (repo / "content.txt").write_text("uncommitted fixture\n")
        (repo / "untracked.txt").write_text("outside the selected tree\n")
        stage = root / "materialized"
        stage.mkdir()
        overrides = {
            "GIT_DIR": str(root / "unrelated-git-dir"),
            "GIT_WORK_TREE": str(root / "unrelated-worktree"),
            "GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "core.abbrev", "GIT_CONFIG_VALUE_0": "5",
            "GIT_NO_LAZY_FETCH": "0", "GIT_NO_REPLACE_OBJECTS": "0",
        }
        with patch.dict(os.environ, overrides):
            observed_commit, observed_tree, provenance = PACKAGER["materialize_source"](repo, "HEAD", stage)
        assert (observed_commit, observed_tree) == (expected_commit, expected_tree)
        assert (stage / "content.txt").read_text() == "committed fixture\n"
        assert (stage / "revision.txt").read_text() == "$Format:%H$\n"
        assert not (stage / "untracked.txt").exists()
        assert stat.S_IMODE((stage / "content.txt").stat().st_mode) == 0o644
        assert stat.S_IMODE((stage / "run.sh").stat().st_mode) == 0o755
        assert (stage / "bin/tool").readlink() == Path("../run.sh")
        assert (stage / "bin/tool").read_text() == "#!/bin/sh\ntrue\n"
        assert provenance["regular_files"] == 4 and provenance["internal_file_symlinks"] == 1
        assert provenance["executable_files"] == 1
        results["exact_committed_attributes_and_modes"] = provenance

        # Remove only a loose fixture blob. The local reader must report the
        # unavailable object before any build command is possible.
        oid = fixture_git(repo, "rev-parse", "HEAD:content.txt")
        loose = repo / ".git/objects" / oid[:2] / oid[2:]
        retained = root / "removed-fixture-blob"
        loose.rename(retained)
        missing = root / "missing"
        missing.mkdir()
        try:
            try:
                PACKAGER["materialize_source"](repo, "HEAD", missing)
            except ValueError as error:
                assert "required Git object is missing" in str(error)
            else:
                raise AssertionError("missing Git blob was accepted")
        finally:
            retained.rename(loose)
        results["missing_local_blob_denied"] = True

        # An unrelated adjacent file is outside the exact committed input.
        (root / "adjacent.txt").write_text("outside the fixture repository\n")
        (repo / "outside").symlink_to("../adjacent.txt")
        commit(repo, "Record an unsupported external fixture link")
        unsafe = root / "unsafe"
        unsafe.mkdir()
        try:
            PACKAGER["materialize_source"](repo, "HEAD", unsafe)
        except ValueError as error:
            assert "escapes the selected tree" in str(error)
        else:
            raise AssertionError("external symlink was accepted")
        assert not (unsafe / "outside").is_symlink()
        results["external_source_symlink_denied"] = True
    return results


if __name__ == "__main__":
    print(json.dumps(check_source(), indent=2))
