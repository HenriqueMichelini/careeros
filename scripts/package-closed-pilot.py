#!/usr/bin/env python3
"""Build or verify a local-only, closed pilot bundle from immutable Git source."""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import tarfile
import tempfile


PROFILE = "scripts/pilot-runtime/build-profile.json"
SCHEMA = "internal/accountstore/schema.sql"
MEASURED_INPUTS = (
    "go.mod", "go.sum", "package-lock.json", "netlify.toml", PROFILE, SCHEMA,
    "scripts/pilot-runtime/bootstrap.sql", "scripts/pilot-runtime/closed-runtime-grants.sql",
    "scripts/package-closed-pilot.py", "scripts/test-closed-pilot.py", "vite.config.ts", "src/index.css",
    "tests/account-browser.fixture.mjs", "tests/closed-pilot-package-browser.check.mjs",
)

GO_OFFLINE = {
    "GOENV": "off", "GOFLAGS": "", "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOWORK": "off",
    "GOPRIVATE": "", "GONOPROXY": "none", "GONOSUMDB": "", "GOINSECURE": "", "GOVCS": "*:off",
}
NPM_COMMAND_OPTIONS = ("--offline", "--script-shell=/bin/sh")
COMMANDS = ("pilot-runtime-local", "pilot-runtime-migrate")


def run(*args, cwd, env=None, capture=False):
    result = subprocess.run(["rtk", "proxy", *args], cwd=cwd, env=env, check=True,
                            stdout=subprocess.PIPE if capture else None)
    return result.stdout if capture else None


def digest(file):
    with file.open("rb") as stream:
        return {"bytes": file.stat().st_size, "sha256": hashlib.file_digest(stream, "sha256").hexdigest()}


def build_environment(profile, source):
    # Only tool discovery, the standard user cache home, and optional Go cache
    # locations are inherited. This also excludes every case variant of npm
    # configuration and ambient Go flags, experiments and module exceptions.
    env = {key: os.environ[key] for key in ("PATH", "HOME", "GOCACHE", "GOMODCACHE") if key in os.environ}
    env.update({
        "CI": "1",
        "NPM_CONFIG_USERCONFIG": os.devnull,
        "NPM_CONFIG_GLOBALCONFIG": str(source / ".isolated-npm-global"),
        "NPM_CONFIG_OFFLINE": "true",
        "NPM_CONFIG_SCRIPT_SHELL": "/bin/sh",
        "NODE_ENV": "production",
    })
    env.update(GO_OFFLINE)
    env.update(profile["go_target"])
    env.update(profile["frontend"])
    return env


def payload(root):
    files = {}
    for entry in sorted(root.rglob("*")):
        mode = entry.lstat().st_mode
        if stat.S_ISDIR(mode):
            continue
        if not stat.S_ISREG(mode):
            raise ValueError("bundle contains a nonregular entry")
        relative = entry.relative_to(root).as_posix()
        if relative != "manifest.json":
            files[relative] = {**digest(entry), "executable": bool(mode & 0o111)}
    return files


def build(repo, revision, output):
    # --end-of-options keeps an untrusted revision from changing git's mode.
    commit = run("git", "rev-parse", "--verify", "--end-of-options", revision + "^{commit}",
                 cwd=repo, capture=True).decode().strip()
    tree = run("git", "rev-parse", "--verify", commit + "^{tree}", cwd=repo, capture=True).decode().strip()
    if output.exists() or output.is_symlink():
        raise ValueError("output directory already exists")
    with tempfile.TemporaryDirectory(prefix="closed-pilot-build-") as temporary:
        source = Path(temporary) / "source"
        bundle = Path(temporary) / "bundle"
        source.mkdir()
        bundle.mkdir()
        (bundle / "bin").mkdir()
        (bundle / "sql").mkdir()
        archive = run("git", "archive", "--format=tar", commit, cwd=repo, capture=True)
        with tarfile.open(fileobj=io.BytesIO(archive)) as packed:
            packed.extractall(source, filter="data")
        if any(source.glob(".env*")) or (source / ".npmrc").exists():
            raise ValueError("archive must not supply dotenv or npm configuration inputs")
        if Path(__file__).read_bytes() != (source / "scripts/package-closed-pilot.py").read_bytes():
            raise ValueError("use the packager from the selected source revision")
        profile = json.loads((source / PROFILE).read_text())
        # Run the archived script's configured build, regardless of ambient
        # CareerOS/Vite flags or a different working tree's generated assets.
        env = build_environment(profile, source)
        expected_go = {**GO_OFFLINE, **profile["go_target"]}
        effective_go = json.loads(run("go", "env", "-json", *expected_go, cwd=source, env=env, capture=True))
        # An explicitly disabled GOENV is shown as an empty effective path.
        if effective_go != {**expected_go, "GOENV": ""}:
            raise ValueError("effective Go configuration does not match the closed build profile")
        toolchain = {
            "go": run("go", "env", "GOVERSION", cwd=source, env=env, capture=True).decode().strip(),
            "node": run("node", "--version", cwd=source, env=env, capture=True).decode().strip(),
            "npm": run("npm", "--version", cwd=source, env=env, capture=True).decode().strip(),
        }
        if toolchain != profile["toolchain"]:
            raise ValueError("installed toolchain does not match the pinned local build profile")
        run("npm", "ci", *NPM_COMMAND_OPTIONS, "--include=dev", "--ignore-scripts", "--no-audit", "--no-fund",
            cwd=source, env=env)
        run("npm", "run", *NPM_COMMAND_OPTIONS, "build", "--", "--outDir", str(bundle / "frontend"), cwd=source, env=env)
        for command in COMMANDS:
            run("go", "build", "-trimpath", "-buildvcs=false", "-mod=readonly", "-o", str(bundle / "bin" / command),
                "./cmd/" + command, cwd=source, env=env)

        for name in ("bootstrap.sql", "closed-runtime-grants.sql"):
            shutil.copyfile(source / "scripts/pilot-runtime" / name, bundle / "sql" / name)
        shutil.copyfile(source / PROFILE, bundle / "build-profile.json")
        manifest = {
            "bundle": profile["id"], "source_commit": commit, "source_tree": tree,
            "target": effective_go["GOOS"] + "/" + effective_go["GOARCH"], "toolchain": toolchain,
            "effective_go": effective_go, "npm_options": list(NPM_COMMAND_OPTIONS),
            "account_schema_sha256": digest(source / SCHEMA)["sha256"],
            "inputs": {name: digest(source / name) for name in MEASURED_INPUTS},
            "payload": payload(bundle),
        }
        document = (json.dumps(manifest, indent=2, sort_keys=True) + "\n").encode()
        trusted_digest = hashlib.sha256(document).hexdigest()
        output.mkdir(mode=0o700)
        shutil.copytree(bundle, output, dirs_exist_ok=True)
        with (output / "manifest.json").open("xb") as target_file:
            target_file.write(document)
    verify(output, trusted_digest)


def verify(bundle, expected):
    if not re.fullmatch(r"[0-9a-f]{64}", expected):
        raise ValueError("independently expected manifest SHA-256 required")
    identity = bundle / "manifest.json"
    if bundle.is_symlink() or not stat.S_ISREG(identity.lstat().st_mode):
        raise ValueError("regular bundle directory and manifest required")
    document = identity.read_bytes()
    actual = hashlib.sha256(document).hexdigest()
    if actual != expected:
        raise ValueError("bundle manifest does not match the independently expected SHA-256")
    manifest = json.loads(document)
    if manifest.get("bundle") != "closed-pilot-local-v1" or payload(bundle) != manifest.get("payload"):
        raise ValueError("bundle payload does not match the manifest")
    print(f"Verified {manifest['bundle']} {manifest['source_commit']} manifest={actual}")
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    modes = parser.add_subparsers(dest="mode", required=True)
    packaging = modes.add_parser("build")
    packaging.add_argument("--revision", default="HEAD")
    packaging.add_argument("--output", required=True, type=Path)
    checking = modes.add_parser("verify")
    checking.add_argument("--bundle", required=True, type=Path)
    checking.add_argument("--expected-manifest-sha256", required=True)
    args = parser.parse_args()
    try:
        if args.mode == "build":
            build(Path(__file__).resolve().parents[1], args.revision, args.output.absolute())
        else:
            verify(args.bundle.absolute(), args.expected_manifest_sha256)
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as exc:
        parser.exit(1, f"closed pilot package failed: {exc}\n")


if __name__ == "__main__":
    main()
