#!/usr/bin/env python3
"""Private synthetic PostgreSQL fixture for Draft recovery and restart checks."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import tarfile
import hashlib
import shutil


def run(args, *, env=None, output=None, cwd=None):
    return subprocess.run(["rtk", "proxy", *args], env=env, stdout=output,
                          stderr=subprocess.STDOUT, cwd=cwd, check=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pattern", default="TestControlledDraft")
    parser.add_argument("--full", action="store_true")
    parser.add_argument("--legacy-base", action="store_true", help="Build immutable pre-reconciliation fixture for upgrade proof")
    parser.add_argument("--reconciliation-physical-restart", action="store_true", help="Dedicated controlled PostgreSQL restart schedule")
    parser.add_argument("--package", action="append", help="Focused Go package; defaults to recovery packages")
    parser.add_argument("--relevant", action="store_true")
    parser.add_argument("--browser", action="store_true")
    parser.add_argument("--deletion-browser", action="store_true")
    parser.add_argument("--lifecycle-browser", action="store_true")
    parser.add_argument("--operator-browser", action="store_true")
    args = parser.parse_args()
    if args.reconciliation_physical_restart and (args.full or args.relevant or args.browser or args.package != ["./internal/aicontrol"] or args.pattern != "TestRestrictedReconciliationExplicitKeyRetirementKeepsIndependentSurvivors"):
        parser.error("physical restart requires its dedicated single-package, single-test schedule")
    repo = Path(__file__).resolve().parents[1]
    os.chdir(repo)
    spec = importlib.util.spec_from_file_location("admission_runner", repo / "scripts/test-ai-admission.py")
    helper = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(helper)
    evidence = Path(tempfile.mkdtemp(prefix="careeros-281-check-"))
    base_executable = None
    if args.legacy_base:
        base_ref = "ea8b97defb6d0c9593f2bf959bfe007d32ac93c8"
        archive = evidence / "base.tar"
        with archive.open("xb") as out:
            if run(["git", "archive", base_ref], output=out).returncode:
                raise RuntimeError("immutable base archive failed")
        base_dir = evidence / "immutable-base"
        base_dir.mkdir()
        with tarfile.open(archive) as source:
            source.extractall(base_dir, filter="data")
        template = repo / "tests/testdata/reconciliation-base-export.go.txt"
        (base_dir / "internal/aicontrol/reconciliation_base_export_test.go").write_bytes(template.read_bytes())
        base_executable = evidence / "base-fixture.test"
        build_env = dict(os.environ, GOCACHE="/tmp/careeros-go-cache", GOFLAGS="-buildvcs=false")
        with (evidence / "base-build.log").open("x") as out:
            if run(["go", "test", "-race", "-vet=off", "-c", "-o", str(base_executable), "./internal/aicontrol"], env=build_env, output=out, cwd=base_dir).returncode:
                raise RuntimeError("immutable base fixture build failed")
        (evidence / "base-fixture.json").write_text(json.dumps({"base": base_ref, "fixture_sha256": hashlib.sha256(template.read_bytes()).hexdigest(), "executable_sha256": hashlib.sha256(base_executable.read_bytes()).hexdigest()}, indent=2)+"\n")
    with tempfile.TemporaryDirectory(prefix="careeros-draft-pg-") as directory:
        root = Path(directory)
        root.chmod(0o700)
        data, sock = root / "data", root / "socket"
        sock.mkdir()
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = str(listener.getsockname()[1])
        started = False
        with (evidence / "setup.log").open("x") as log:
            try:
                if run([helper.binary("initdb"), "-D", str(data), "--no-locale", "--encoding=UTF8", "--auth-local=trust", "--auth-host=trust"], output=log).returncode:
                    raise RuntimeError("private initdb failed")
                # Default fsync/WAL settings remain on for the real process-restart
                # schedule. Only this private loopback port is exposed.
                options = f"-h 127.0.0.1 -k {sock} -p {port}"
                if run([helper.binary("pg_ctl"), "-D", str(data), "-l", str(evidence / "postgres.log"), "-o", options, "-w", "start"], output=log).returncode:
                    raise RuntimeError("private PostgreSQL startup failed")
                started = True
                if run([helper.binary("psql"), "-h", "127.0.0.1", "-p", port, "-d", "postgres", "-c", "CREATE DATABASE careeros_test"], output=log).returncode:
                    raise RuntimeError("private database creation failed")
                env = dict(os.environ)
                env.update(CAREEROS_TEST_DATABASE_URL=f"postgresql://127.0.0.1:{port}/careeros_test?sslmode=disable", CAREEROS_DRAFT_TEST_PGDATA=str(data), CAREEROS_DRAFT_TEST_PGCTL=helper.binary("pg_ctl"), CAREEROS_LOCAL_CHECK_EVIDENCE=str(evidence), GOCACHE="/tmp/careeros-go-cache", GOFLAGS="-buildvcs=false", PYTHONDONTWRITEBYTECODE="1")
                if args.reconciliation_physical_restart:
                    env["CAREEROS_RECONCILIATION_PHYSICAL_RESTART"] = "1"
                if base_executable:
                    env["CAREEROS_RECONCILIATION_BASE_EXECUTABLE"] = str(base_executable)
                command = ["go", "test", "-p=1", "-json", "-race", "-vet=off", "-timeout=600s", "-count=1"]
                if args.browser or args.deletion_browser or args.lifecycle_browser or args.operator_browser:
                    script = "tests/operator-control-browser.check.mjs" if args.operator_browser else "tests/account-lifecycle-browser.check.mjs" if args.lifecycle_browser else "tests/account-deletion-browser.check.mjs" if args.deletion_browser else "tests/draft-recovery-browser.check.mjs"
                    command = ["node", script]
                elif args.full:
                    command += ["./..."]
                elif args.relevant:
                    command += helper.PACKAGES + ["./internal/draftrecovery", "./internal/auth", "./cmd/deletion-local"]
                else:
                    command += ["-run", args.pattern, *(args.package or ["./internal/accountapi", "./internal/draftrecovery"])]
                receipt_name = "browser.jsonl" if args.browser or args.deletion_browser or args.lifecycle_browser or args.operator_browser else "go.jsonl"
                with (evidence / receipt_name).open("x") as output:
                    result = run(command, env=env, output=output)
                events = []
                for line in (evidence / receipt_name).read_text().splitlines():
                    try:
                        events.append(json.loads(line))
                    except json.JSONDecodeError:
                        pass
                summary = {"synthetic_only": True, "go_exit": result.returncode,
                           "pass_events": sum(e.get("Action") == "pass" for e in events),
                           "failures": [{"package": e.get("Package"), "test": e.get("Test")} for e in events if e.get("Action") == "fail"],
                           "skips": [{"package": e.get("Package"), "test": e.get("Test")} for e in events if e.get("Action") == "skip"]}
                if args.browser or args.deletion_browser or args.lifecycle_browser or args.operator_browser:
                    summary["browser_cells"] = [e for e in events if e.get("status") == "passed"]
                if args.relevant:
                    summary["admission"] = helper.summarize(evidence / "go.jsonl")
                (evidence / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
                print(json.dumps({"evidence": str(evidence), **summary}))
                return result.returncode
            finally:
                if started:
                    run([helper.binary("pg_ctl"), "-D", str(data), "-m", "immediate", "-w", "stop"], output=log)
                if (data / "log").is_dir():
                    shutil.copytree(data / "log", evidence / "postgres-server-logs")


if __name__ == "__main__":
    raise SystemExit(main())
