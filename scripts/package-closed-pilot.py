#!/usr/bin/env python3
"""Build or verify a local-only, closed pilot bundle from immutable Git source."""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
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
    "scripts/package-closed-pilot.py", "scripts/test-closed-pilot.py",
)


def run(*args, cwd, env=None, capture=False):
    result = subprocess.run(["rtk", "proxy", *args], cwd=cwd, env=env, check=True,
                            stdout=subprocess.PIPE if capture else None)
    return result.stdout if capture else None


def digest(file):
    with file.open("rb") as stream:
        return {"bytes": file.stat().st_size, "sha256": hashlib.file_digest(stream, "sha256").hexdigest()}


def build_environment(frontend, source):
    env = {
        key: value for key, value in os.environ.items()
        if not key.startswith(("VITE_", "CAREEROS_", "FIGMA_", "NPM_CONFIG_"))
        and key not in ("NODE_OPTIONS", "NODE_PATH", "GOFLAGS", "GODEBUG")
    }
    env.update({
        "CI": "1", "CGO_ENABLED": "0", "GOENV": "off", "GOPROXY": "off",
        "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOWORK": "off",
        "NPM_CONFIG_USERCONFIG": os.devnull,
        "NPM_CONFIG_GLOBALCONFIG": str(source / ".isolated-npm-global"),
        "NODE_ENV": "production",
    })
    env.update(frontend)
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
        if any(source.glob(".env*")):
            raise ValueError("archive must not supply dotenv inputs")
        profile = json.loads((source / PROFILE).read_text())
        # Run the archived script's configured build, regardless of ambient
        # CareerOS/Vite flags or a different working tree's generated assets.
        env = build_environment(profile["frontend"], source)
        target = run("go", "env", "GOOS", "GOARCH", cwd=source, env=env, capture=True).decode().splitlines()
        toolchain = {
            "go": run("go", "env", "GOVERSION", cwd=source, env=env, capture=True).decode().strip(),
            "node": run("node", "--version", cwd=source, env=env, capture=True).decode().strip(),
            "npm": run("npm", "--version", cwd=source, env=env, capture=True).decode().strip(),
        }
        if toolchain != profile["toolchain"]:
            raise ValueError("installed toolchain does not match the pinned local build profile")
        run("npm", "ci", "--offline", "--include=dev", "--ignore-scripts", "--no-audit", "--no-fund",
            cwd=source, env=env)
        run("npm", "run", "build", "--", "--outDir", str(bundle / "frontend"), cwd=source, env=env)
        for command in ("pilot-runtime-local", "account-migrate"):
            run("go", "build", "-trimpath", "-buildvcs=false", "-mod=readonly", "-o", str(bundle / "bin" / command),
                "./cmd/" + command, cwd=source, env=env)
        schema_id = run(str(bundle / "bin/pilot-runtime-local"), "-schema-id", cwd=source,
                        env=env, capture=True).decode().strip()
        if schema_id != digest(source / SCHEMA)["sha256"]:
            raise ValueError("compiled account migration does not match the archived schema")

        for name in ("bootstrap.sql", "closed-runtime-grants.sql"):
            shutil.copyfile(source / "scripts/pilot-runtime" / name, bundle / "sql" / name)
        shutil.copyfile(source / PROFILE, bundle / "build-profile.json")
        manifest = {
            "bundle": profile["id"], "source_commit": commit, "source_tree": tree,
            "target": "/".join(target), "toolchain": toolchain, "account_schema_sha256": schema_id,
            "inputs": {name: digest(source / name) for name in MEASURED_INPUTS},
            "payload": payload(bundle),
        }
        output.mkdir(mode=0o700)
        shutil.copytree(bundle, output, dirs_exist_ok=True)
        with (output / "manifest.json").open("x") as target_file:
            target_file.write(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    verify(output)


def verify(bundle):
    identity = bundle / "manifest.json"
    if bundle.is_symlink() or not stat.S_ISREG(identity.lstat().st_mode):
        raise ValueError("regular bundle directory and manifest required")
    manifest = json.loads(identity.read_text())
    if manifest.get("bundle") != "closed-pilot-local-v1" or payload(bundle) != manifest.get("payload"):
        raise ValueError("bundle payload does not match the manifest")
    print(f"Verified {manifest['bundle']} {manifest['source_commit']} manifest={digest(identity)['sha256']}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    modes = parser.add_subparsers(dest="mode", required=True)
    packaging = modes.add_parser("build")
    packaging.add_argument("--revision", default="HEAD")
    packaging.add_argument("--output", required=True, type=Path)
    checking = modes.add_parser("verify")
    checking.add_argument("--bundle", required=True, type=Path)
    args = parser.parse_args()
    try:
        if args.mode == "build":
            build(Path(__file__).resolve().parents[1], args.revision, args.output.absolute())
        else:
            verify(args.bundle.absolute())
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as exc:
        parser.exit(1, f"closed pilot package failed: {exc}\n")


if __name__ == "__main__":
    main()
