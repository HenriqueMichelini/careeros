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


def run(args, *, env=None, output=None):
    return subprocess.run(["rtk", "proxy", *args], env=env, stdout=output,
                          stderr=subprocess.STDOUT, check=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pattern", default="TestControlledDraft")
    parser.add_argument("--full", action="store_true")
    parser.add_argument("--relevant", action="store_true")
    parser.add_argument("--browser", action="store_true")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
    os.chdir(repo)
    spec = importlib.util.spec_from_file_location("admission_runner", repo / "scripts/test-ai-admission.py")
    helper = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(helper)
    evidence = Path(tempfile.mkdtemp(prefix="careeros-281-check-"))
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
                env.update(CAREEROS_TEST_DATABASE_URL=f"postgresql://127.0.0.1:{port}/careeros_test?sslmode=disable", CAREEROS_DRAFT_TEST_PGDATA=str(data), CAREEROS_DRAFT_TEST_PGCTL=helper.binary("pg_ctl"), GOCACHE="/tmp/careeros-go-cache", GOFLAGS="-buildvcs=false", PYTHONDONTWRITEBYTECODE="1")
                command = ["go", "test", "-json", "-race", "-vet=off", "-timeout=180s", "-count=1"]
                if args.browser:
                    command = ["node", "tests/draft-recovery-browser.check.mjs"]
                elif args.full:
                    command += ["./..."]
                elif args.relevant:
                    command += helper.PACKAGES + ["./internal/draftrecovery"]
                else:
                    command += ["-run", args.pattern, "./internal/accountapi", "./internal/draftrecovery"]
                receipt_name = "browser.jsonl" if args.browser else "go.jsonl"
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
                if args.browser:
                    summary["browser_cells"] = [e for e in events if e.get("status") == "passed"]
                if args.relevant:
                    summary["admission"] = helper.summarize(evidence / "go.jsonl")
                (evidence / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
                print(json.dumps({"evidence": str(evidence), **summary}))
                return result.returncode
            finally:
                if started:
                    run([helper.binary("pg_ctl"), "-D", str(data), "-m", "immediate", "-w", "stop"], output=log)


if __name__ == "__main__":
    raise SystemExit(main())
