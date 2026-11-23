#!/usr/bin/env python3
"""Exercise the built closed pilot bundle and its real roles in owned PostgreSQL."""

import argparse
from contextlib import contextmanager
import hashlib
import http.client
import json
import os
from pathlib import Path
import runpy
import shutil
import signal
import socket
import subprocess
import tempfile
import time


HERE = Path(__file__).resolve().parent
DATABASE = "careeros_pilot_runtime_fixture"
RUNTIME = "careeros_pilot_runtime"
MIGRATOR = "careeros_pilot_migrator"
SESSION = "local-closed-pilot-session-fixture"
CSRF = "local-closed-pilot-csrf-fixture"


def command(args, log, env):
    return subprocess.run(["rtk", "proxy", *args], stdout=subprocess.PIPE,
                          stderr=log, env=env, check=True, timeout=75).stdout.decode().strip()


def unused_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def clean_environment():
    return {
        key: value for key, value in os.environ.items()
        if not key.startswith(("PG", "CAREEROS_", "VITE_"))
    }


@contextmanager
def local_server(bundle, port, log, env):
    with subprocess.Popen(
        ["rtk", "proxy", str(bundle / "bin/pilot-runtime-local"),
         "-listen", f"127.0.0.1:{port}", "-static", str(bundle / "frontend")],
        stdout=log, stderr=log, env=env, start_new_session=True,
    ) as process:
        try:
            deadline = time.monotonic() + 5
            while True:
                if process.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError("bundled pilot listener did not become live")
                try:
                    status, _, _ = request(port, "/healthz")
                    if status == 200:
                        break
                except OSError:
                    pass
                time.sleep(0.05)
            yield
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGTERM)
            try:
                process.wait(timeout=17)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
                raise RuntimeError("bundled pilot listener did not drain")


def request(port, path, method="GET", headers=None, body=None):
    connection = http.client.HTTPConnection("127.0.0.1", port, timeout=4)
    try:
        connection.request(method, path, body, headers or {})
        response = connection.getresponse()
        return response.status, response.headers, response.read()
    finally:
        connection.close()


class Checks:
    def __init__(self, port, events):
        self.port = port
        self.events = events

    def record(self, name, observed=None):
        self.events.append({"name": name, "result": "pass", "observed": observed})

    def http(self, name, path, expected, contains, method="GET", headers=None, body=None):
        status, response, data = request(self.port, path, method, headers, body)
        if status != expected or contains.encode() not in data:
            raise RuntimeError(f"{name}: expected HTTP {expected} with {contains!r}, got {status}")
        if status >= 400 and (
            not response.get("Content-Type", "").startswith("application/json")
            or response.get("Cache-Control") != "no-store"
        ):
            raise RuntimeError(f"{name}: HTTP error lost JSON or no-store")
        self.record(name, status)
        return response, data


def run_checks(bundle, evidence, log, events, binary):
    env = clean_environment()
    local = unused_port()
    web = Checks(local, events)
    origin = f"http://127.0.0.1:{local}"
    with local_server(bundle, local, log, env):
        _, html = web.http("built_frontend_served", "/", 200, '<div id="root">')
        _, same = web.http("spa_navigation", "/workspace/draft", 200, '<div id="root">')
        if html != same:
            raise RuntimeError("SPA navigation did not reuse the built frontend")
        web.http("unknown_asset", "/assets/not-in-package.js", 404, "not_found")
        web.http("unknown_api", "/api/not-in-package", 404, "not_found")
        web.http("unknown_function", "/.netlify/functions/not-in-package", 404, "not_found")
        web.http("invalid_aliased_path", "/.netlify/functions/account/api/%70ilot/status", 400, "invalid_path")
        web.http("unconfigured_readiness", "/readyz", 503, "unavailable")
        web.http("missing_runtime", "/api/auth/session", 503, "auth_unavailable")
        web.http("missing_runtime_direct", "/.netlify/functions/account/api/auth/session", 503, "auth_unavailable")
        web.http("origin_host_cannot_be_forwarded", "/api/auth/session", 421, "wrong_origin",
                 headers={"Host": "other.invalid", "X-Forwarded-Host": f"127.0.0.1:{local}"})
        web.http("cross_origin_alias", "/.netlify/functions/qualification-gaps", 403, "forbidden",
                 method="POST", headers={"Origin": origin + ".invalid"})
        web.http("unconfigured_allowance", "/api/draft-allowance", 503, "auth_unavailable")
        web.http("unconfigured_allowance_alias", "/.netlify/functions/account/api/draft-allowance", 503, "auth_unavailable")
        rendered = json.loads(command(["node", str(HERE.parent / "tests/closed-pilot-package-browser.check.mjs"), origin],
                                      log, env))
        if rendered["rendered"] is not True or rendered["external_requests"] != 0:
            raise RuntimeError("closed pilot browser did not render without external assets")
        web.record("packaged_browser_no_external_asset_requests", rendered)

    # A short, owned Unix socket path is solely for administration. The
    # bundled server authenticates its own restricted role on literal loopback.
    with tempfile.TemporaryDirectory(prefix="cp292-", dir="/tmp") as temporary:
        folder = Path(temporary)
        data = folder / "data"
        pg_port = unused_port()
        command([binary("initdb"), "-D", str(data), "--no-locale", "--encoding=UTF8",
                 "--auth-local=trust", "--auth-host=reject"], log, env)
        access = data / "pg_hba.conf"
        access.write_text(f"host {DATABASE} {RUNTIME},{MIGRATOR} 127.0.0.1/32 trust\n" + access.read_text())
        pgctl = [binary("pg_ctl"), "-D", str(data)]
        command(pgctl + ["-l", str(evidence / "postgres.log"), "-o",
                        f"-F -h 127.0.0.1 -k {folder} -p {pg_port} -c max_connections=20", "-w", "start"], log, env)
        running = True
        try:
            psql = [binary("psql"), "-X", "-h", str(folder), "-p", str(pg_port), "-v", "ON_ERROR_STOP=1"]
            command(psql + ["-d", "postgres", "-c", f"CREATE DATABASE {DATABASE}"], log, env)
            admin = psql + ["-d", DATABASE]
            command(admin + ["-f", str(bundle / "sql/bootstrap.sql")], log, env)
            fixture_env = {**env, "CAREEROS_PILOT_LOCAL_DATABASE_URL":
                           f"postgresql://{RUNTIME}@127.0.0.1:{pg_port}/{DATABASE}"}
            with local_server(bundle, local, log, fixture_env):
                web.http("absent_schema_stays_closed", "/readyz", 503, "unavailable")
                web.http("absent_schema_api", "/api/profile/current", 503, "auth_unavailable")
                if command(admin + ["-Atc", "SELECT to_regclass('careeros_pilot_runtime.careeros_accounts') IS NULL"],
                           log, env) != "t":
                    raise RuntimeError("startup changed schema")
                web.record("startup_never_migrates")
            migration_key = "CAREEROS_PILOT_LOCAL_MIGRATOR_DATABASE_URL"
            migrate_url = f"postgresql://{MIGRATOR}@127.0.0.1:{pg_port}/{DATABASE}"
            migrate = {**env, migration_key: migrate_url}
            migrate_command = [str(bundle / "bin/pilot-runtime-migrate")]
            incorrect = {
                "generic_credentials_not_used_by_packaged_migrator": {**env, "CAREEROS_DATABASE_URL": migrate_url},
                "wrong_database_is_denied_before_ddl": {**env, migration_key: migrate_url + "_other"},
                "wrong_role_is_denied_before_ddl": {**env, migration_key: fixture_env["CAREEROS_PILOT_LOCAL_DATABASE_URL"]},
                "arbitrary_connection_options_denied": {**env, migration_key: migrate_url + "?sslmode=disable"},
            }
            for name, settings in incorrect.items():
                denied_migration(migrate_command, settings, log)
                web.record(name)
            command(admin + ["-c", "ALTER SCHEMA careeros_pilot_runtime OWNER TO CURRENT_USER"], log, env)
            denied_migration(migrate_command, migrate, log)
            if command(admin + ["-Atc", "SELECT to_regclass('careeros_pilot_runtime.careeros_accounts') IS NULL"],
                       log, env) != "t":
                raise RuntimeError("denied migration changed schema")
            web.record("actual_schema_owner_checked_before_ddl")
            command(admin + ["-c", f"ALTER SCHEMA careeros_pilot_runtime OWNER TO {MIGRATOR}"], log, env)
            command(migrate_command, log, migrate)
            migration = psql + ["-d", DATABASE, "-U", MIGRATOR]
            component = command([str(bundle / "bin/pilot-runtime-local"), "-schema-id"], log, env)
            manifest = json.loads((bundle / "manifest.json").read_text())
            if component != manifest["account_schema_sha256"]:
                raise RuntimeError("compiled package schema does not match the verified source manifest")
            web.record("embedded_schema_identity_matches_verified_manifest")
            grants = migration + ["-f", str(bundle / "sql/closed-runtime-grants.sql")]
            command(grants + ["-v", "schema_id=" + "0" * 64], log, env)
            with local_server(bundle, local, log, fixture_env):
                web.http("mismatched_schema_identity", "/readyz", 503, "unavailable")
            command(grants + ["-v", "schema_id=" + component], log, env)
            secret_hash = hashlib.sha256(SESSION.encode()).hexdigest()
            csrf_hash = hashlib.sha256(CSRF.encode()).hexdigest()
            command(migration + ["-c", f"""
INSERT INTO careeros_accounts VALUES ('closed-pilot-fixture-account','active',1,clock_timestamp(),''::bytea,''::bytea);
INSERT INTO careeros_sessions VALUES ('closed-pilot-fixture-session','closed-pilot-fixture-account',
 decode('{secret_hash}','hex'),decode('{csrf_hash}','hex'),1,clock_timestamp(),clock_timestamp()+interval '1 hour');
"""], log, env)
            command(migrate_command, log, migrate)
            command(grants + ["-v", "schema_id=" + component], log, env)
            web.record("explicit_additive_migration_reapplied_with_separate_role")
            if command(admin + ["-Atc", f"""
SELECT NOT has_schema_privilege('{RUNTIME}','careeros_pilot_runtime','CREATE')
 AND NOT has_any_column_privilege('{RUNTIME}','careeros_pilot_runtime.careeros_profiles','UPDATE')
 AND NOT has_any_column_privilege('{RUNTIME}','careeros_pilot_runtime.careeros_control_state','UPDATE')
 AND NOT has_any_column_privilege('{RUNTIME}','careeros_pilot_runtime.careeros_pilot_access','UPDATE')
 AND NOT has_table_privilege('{RUNTIME}','careeros_pilot_runtime.careeros_operations_pending','SELECT')
 AND (SELECT rolconnlimit FROM pg_roles WHERE rolname='{RUNTIME}')=8;
"""], log, env) != "t":
                raise RuntimeError("runtime role has unexpected grants or connection limit")
            web.record("closed_database_role_grants_and_connection_limit")

            valid = {"Origin": origin, "Cookie": f"__Host-careeros_session={SESSION}; __Host-careeros_csrf={CSRF}",
                     "X-CareerOS-Account": "closed-pilot-fixture-account", "X-CSRF-Token": CSRF}
            with local_server(bundle, local, log, fixture_env):
                web.http("prepared_but_closed_readiness", "/readyz", 200, '"inference":"closed"')
                web.http("guest_session", "/api/auth/session", 200, '"authenticated":false')
                web.http("existing_session_after_repeat_migration",
                         "/.netlify/functions/account/api/auth/session", 200, '"authenticated":true', headers=valid)
                web.http("account_cookie_typed_denial", "/api/profile/current", 403, "forbidden", headers=valid)
                web.http("status_closed_for_uninvited_fixture", "/api/pilot/status", 200, '"state":"uninvited"', headers=valid)
                web.http("no_new_google_sign_in", "/api/auth/google/start", 503, "auth_unavailable")
                web.http("no_google_callback", "/.netlify/functions/account/api/auth/google/callback?code=fixture",
                         503, "auth_unavailable")
                web.http("wrong_browser_origin", "/api/auth/session", 403, "forbidden",
                         headers={**valid, "Origin": origin.replace("127.0.0.1", "localhost")})
                web.http("activation_without_bound_csrf", "/api/pilot/activate", 403, "forbidden",
                         method="POST", headers={**valid, "X-CSRF-Token": "invalid"}, body="{}")
                web.http("activation_still_closed", "/api/pilot/activate", 503, "auth_unavailable",
                         method="POST", headers=valid, body="{}")
                web.http("unmounted_allowance_stays_unavailable", "/api/draft-allowance",
                         503, "auth_unavailable", headers=valid)
                web.http("unmounted_allowance_alias_stays_unavailable", "/.netlify/functions/account/api/draft-allowance",
                         503, "auth_unavailable", headers=valid)
                for number, path in enumerate((
                    "/api/cv/generate", "/.netlify/functions/cv-generation",
                    "/api/profile/ingest", "/.netlify/functions/profile-ingestion",
                    "/api/profile/review", "/.netlify/functions/profile-review",
                    "/api/qualification-gaps", "/.netlify/functions/qualification-gaps",
                    "/api/application-draft", "/.netlify/functions/application-draft",
                    "/api/drafts", "/api/ai/qualification/actions/fake",
                ), 1):
                    web.http(f"missing_policy_{number:02d}", path, 503, "auth_unavailable",
                             method="POST", headers=valid, body="{}")
                web.http("bounded_request_body", "/api/profile/current", 413, "request_too_large",
                         method="PUT", headers={**valid, "Content-Length": str(5 * 2**20 + 1),
                                                "Expect": "100-continue"}, body=None)

            with local_server(bundle, local, log, fixture_env):
                web.http("restart_retains_authorized_session", "/api/auth/session", 200,
                         '"authenticated":true', headers=valid)
                response, _ = web.http("same_origin_logout_all", "/.netlify/functions/account/api/auth/logout-all",
                                       204, "", method="POST", headers=valid)
                cookies = response.get_all("Set-Cookie") or []
                expected = {"__Host-careeros_session", "__Host-careeros_csrf"}
                if len(cookies) != 2 or {c.split("=", 1)[0] for c in cookies} != expected or any(
                    any(flag not in c for flag in ("Secure", "SameSite=Lax", "Path=/", "Max-Age=0")) or
                    "Domain=" in c or (c.startswith("__Host-careeros_session") != ("HttpOnly" in c))
                    for c in cookies
                ):
                    raise RuntimeError("existing secure logout cookies were changed or collapsed")
                web.record("multiple_secure_host_only_cookie_headers")
                web.http("logged_out_cookie_no_longer_authorized", "/api/auth/session", 200,
                         '"authenticated":false', headers=valid)
                command(migration + ["-c", "UPDATE careeros_control_state SET inference_stopped=false"], log, env)
                denied_migration(migrate_command, migrate, log)
                web.record("packaged_migrator_denies_open_control_state")
                web.http("open_control_is_not_accepted_by_closed_package", "/readyz", 503, "unavailable")
                web.http("open_control_cannot_enable_managed_alias", "/.netlify/functions/application-draft",
                         503, "auth_unavailable", method="POST", headers=valid, body="{}")
                command(migration + ["-c", "UPDATE careeros_control_state SET inference_stopped=true"], log, env)
                web.http("restored_closed_control_readiness", "/readyz", 200, '"invitations":"closed"')
                command(pgctl + ["-m", "fast", "-w", "stop"], log, env)
                running = False
                web.http("database_loss_preserves_liveness", "/healthz", 200, "live")
                web.http("database_loss_closes_readiness", "/readyz", 503, "unavailable")
                web.http("database_loss_closes_api", "/api/auth/session", 503, "auth_unavailable")
        finally:
            if running:
                command(pgctl + ["-m", "fast", "-w", "stop"], log, env)
    web.record("owned_postgres_stopped_and_data_removed")


def denied_migration(args, env, log):
    result = subprocess.run(["rtk", "proxy", *args], env=env, stdout=log, stderr=subprocess.PIPE,
                            text=True, timeout=35)
    if result.returncode != 1 or "requires its dedicated local database and migrator" not in result.stderr:
        raise RuntimeError("expected the packaged migrator to refuse the fixture configuration")
    log.write(result.stderr)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("--expected-manifest-sha256", required=True)
    args = parser.parse_args()
    helpers = runpy.run_path(str(HERE / "test-ai-admission.py"))
    evidence = helpers["evidence_directory"](args.evidence)
    bundle = args.bundle.absolute()
    events = []
    passed = False
    with (evidence / "checks.log").open("x") as log:
        try:
            with tempfile.TemporaryDirectory(prefix="closed-pilot-package-run-") as temporary:
                if bundle.is_symlink():
                    raise RuntimeError("regular bundle directory required")
                candidate = Path(temporary) / "bundle"
                # Execute only an owned snapshot that matched the independently
                # approved digest. The supplied directory can change later.
                shutil.copytree(bundle, candidate, symlinks=True)
                log.write(command(["python3", str(HERE / "package-closed-pilot.py"), "verify", "--bundle", str(candidate),
                                   "--expected-manifest-sha256", args.expected_manifest_sha256],
                                  log, clean_environment()) + "\n")
                run_checks(candidate, evidence, log, events, helpers["binary"])
                passed = True
        finally:
            receipt = {"synthetic_only": True, "pass": passed, "events": events,
                       "package_manifest_sha256": args.expected_manifest_sha256}
            with (evidence / "summary.json").open("x") as summary:
                json.dump(receipt, summary, indent=2)
                summary.write("\n")
    print(f"{len(events)} closed-pilot package checks passed. Evidence: {evidence}")


if __name__ == "__main__":
    main()
