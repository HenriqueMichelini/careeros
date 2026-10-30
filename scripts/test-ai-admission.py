#!/usr/bin/env python3
"""Run the synthetic admission contract against a private disposable PostgreSQL.

No existing database, hosted service, credentials, or provider transport is used.
The JSON receipt fails closed on missing, skipped, or failed acceptance cases.
"""

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile


CASES = [f"A{i:02}" for i in range(1, 38)]
CASE_NAME = re.compile(r"(?:^|/)(?:Test)?(A(?:0[1-9]|[12][0-9]|3[0-7]))(?=[A-Z_/-]|$)")
PACKAGES = [
    "./internal/aicontrol", "./internal/aidispatch", "./internal/openaihttp",
    "./internal/fieldvalidation", "./internal/accountapi", "./internal/accountstore",
    "./backend/...", "./netlify/functions/...", "./cmd/profile-review",
]


def binary(name):
    candidates = [shutil.which(name)]
    candidates += [f"/usr/pgsql-{v}/bin/{name}" for v in range(18, 13, -1)]
    candidates += [f"/usr/lib/postgresql/{v}/bin/{name}" for v in range(18, 13, -1)]
    for candidate in candidates:
        if candidate and Path(candidate).is_file():
            return candidate
    raise RuntimeError(f"Install local PostgreSQL server binaries: missing {name}")


def command(args, *, env=None, output=None):
    return subprocess.run(["rtk", "proxy", *args], env=env, stdout=output,
                          stderr=subprocess.STDOUT, check=False)


def summarize(path):
    events = {}
    package_failures = []
    for line in path.read_text().splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        action, test = event.get("Action"), event.get("Test", "")
        match = CASE_NAME.search(test)
        if match and action in ("pass", "fail", "skip"):
            key = (event.get("Package", ""), test)
            events[key] = (match.group(1), action)
        if action == "fail" and not test:
            package_failures.append(event.get("Package", "unknown"))
    receipt = {}
    for case in CASES:
        tests = [{"package": p, "test": t, "status": status}
                 for (p, t), (identifier, status) in sorted(events.items())
                 if identifier == case]
        status = "not_executed"
        if tests:
            status = "pass" if all(t["status"] == "pass" for t in tests) else "unmet"
        receipt[case] = {"status": status, "tests": tests}
    return {"synthetic_only": True, "cases": receipt,
            "failed_packages": sorted(set(package_failures))}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, help="New local directory for JSON receipts")
    parser.add_argument("--no-race", action="store_true", help="Omit Go race instrumentation")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
    os.chdir(repo)
    evidence = args.evidence or Path(tempfile.mkdtemp(prefix="careeros-ai-evidence-"))
    evidence.mkdir(parents=True, exist_ok=True)
    output = evidence / "go-tests.jsonl"
    if output.exists():
        raise RuntimeError("Use a fresh evidence directory; refusing to overwrite a prior run")
    pg_ctl = binary("pg_ctl")
    with tempfile.TemporaryDirectory(prefix="careeros-ai-pg-") as temporary:
        root = Path(temporary)
        root.chmod(0o700)
        data, socket = root / "data", root / "socket"
        socket.mkdir()
        # Unix sockets only; the private directory isolates this port from
        # other clusters without binding or exposing a network listener.
        port = "55439"
        started = False
        with (evidence / "postgres-setup.log").open("w") as setup:
            try:
                init = command([binary("initdb"), "-D", str(data), "--no-locale",
                                "--encoding=UTF8", "--auth-local=trust", "--auth-host=reject"], output=setup)
                if init.returncode:
                    raise RuntimeError("Disposable initdb failed; see postgres-setup.log")
                start = command([pg_ctl, "-D", str(data), "-l", str(evidence / "postgres.log"),
                                 "-o", f"-F -h '' -k {socket} -p {port}", "-w", "start"], output=setup)
                if start.returncode:
                    raise RuntimeError("Disposable PostgreSQL start failed; see postgres logs")
                started = True
                create = command([binary("psql"), "-h", str(socket), "-p", port,
                                  "-d", "postgres", "-c", "CREATE DATABASE careeros_ai_synthetic"], output=setup)
                if create.returncode:
                    raise RuntimeError("Disposable database creation failed")
                env = dict(os.environ)
                env["CAREEROS_TEST_DATABASE_URL"] = (
                    f"postgresql:///careeros_ai_synthetic?host={socket}&port={port}&sslmode=disable")
                env["GOCACHE"] = str(root / "go-cache")
                # /tmp can contain unrelated VCS metadata. This affects build
                # provenance stamping only; test selection/assertions stay intact.
                env["GOFLAGS"] = "-buildvcs=false"
                go_args = ["go", "test", "-p=4", "-json", "-count=1", "-timeout=180s"]
                if not args.no_race:
                    go_args.append("-race")
                with output.open("w") as log:
                    result = command(go_args + PACKAGES, env=env, output=log)
                receipt = summarize(output)
                receipt["go_exit_code"] = result.returncode
                (evidence / "acceptance-results.json").write_text(json.dumps(receipt, indent=2) + "\n")
                passed = sum(v["status"] == "pass" for v in receipt["cases"].values())
                print(f"Acceptance cases: {passed}/37 passed; evidence: {evidence}")
                return 0 if passed == 37 and result.returncode == 0 else 1
            finally:
                if started:
                    stopped = command([pg_ctl, "-D", str(data), "-m", "immediate", "-w", "stop"], output=setup)
                    if stopped.returncode:
                        raise RuntimeError("Disposable PostgreSQL shutdown failed; inspect local setup log")


if __name__ == "__main__":
    raise SystemExit(main())
