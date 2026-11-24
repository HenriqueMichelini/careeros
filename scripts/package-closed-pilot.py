#!/usr/bin/env python3
"""Build or verify a local-only, closed pilot bundle from immutable Git source."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import posixpath
import re
import shutil
import stat
import subprocess
import tempfile


PROFILE = "scripts/pilot-runtime/build-profile.json"
SCHEMA = "internal/accountstore/schema.sql"
MEASURED_INPUTS = (
    "go.mod", "go.sum", "package-lock.json", "netlify.toml", PROFILE, SCHEMA,
    "scripts/pilot-runtime/bootstrap.sql", "scripts/pilot-runtime/closed-runtime-grants.sql",
    "scripts/package-closed-pilot.py", "scripts/test-closed-pilot.py", "scripts/test-closed-pilot-source.py",
    "vite.config.ts", "src/index.css",
    "tests/account-browser.fixture.mjs", "tests/closed-pilot-package-browser.check.mjs",
)

GO_OFFLINE = {
    "GOENV": "off", "GOFLAGS": "", "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOWORK": "off",
    "GOPRIVATE": "", "GONOPROXY": "none", "GONOSUMDB": "", "GOINSECURE": "", "GOVCS": "*:off",
}
NPM_COMMAND_OPTIONS = ("--offline", "--script-shell=/bin/sh")
COMMANDS = ("pilot-runtime-local", "pilot-runtime-migrate")
GIT = ("git", "--no-replace-objects", "--no-lazy-fetch")
SOURCE_MODES = {"040000": "tree", "100644": "blob", "100755": "blob", "120000": "blob"}


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


def git_environment():
    # Git finds the owning worktree's on-disk metadata. The caller cannot
    # override its repository, object store, config, or fetch policy.
    env = {key: os.environ[key] for key in ("PATH",) if key in os.environ}
    env.update({
        "LC_ALL": "C", "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_SYSTEM": os.devnull,
        "GIT_CONFIG_NOSYSTEM": "1", "GIT_NO_REPLACE_OBJECTS": "1", "GIT_NO_LAZY_FETCH": "1",
        "GIT_ALLOW_PROTOCOL": "", "GIT_PROTOCOL_FROM_USER": "0", "GIT_TERMINAL_PROMPT": "0",
        "GIT_OPTIONAL_LOCKS": "0",
    })
    return env


def source_path(raw):
    name = raw.decode("utf-8")
    parts = name.split("/")
    if ("\\" in name or any(ord(c) < 32 or ord(c) == 127 for c in name)
            or any(p.casefold() in ("", ".", "..", ".git") for p in parts)):
        raise ValueError("unsafe path in the selected Git tree")
    return name


def source_link(name, data, entries):
    target = data.decode("utf-8")
    parts = target.split("/")
    if ("" in parts or "\\" in target
            or any(ord(c) < 32 or ord(c) == 127 for c in target)):
        raise ValueError("unsafe Git source symlink")
    current = name.split("/")[:-1]
    for index, part in enumerate(parts):
        if part == "..":
            if not current:
                raise ValueError("Git source symlink escapes the selected tree")
            current.pop()
        elif part != ".":
            current.append(part)
        mode = entries.get("/".join(current), ("",))[0] if current else "040000"
        if index + 1 < len(parts) and mode != "040000":
            raise ValueError("Git source symlink has an unsafe intermediate target")
    if mode not in ("100644", "100755"):
        raise ValueError("Git source symlink must point to a committed regular file")
    return target


def materialize_source(repo, revision, source):
    """Write verified committed blobs without archive attributes or checkout filters."""
    env = git_environment()

    def git(*args):
        return run(*GIT, *args, cwd=repo, env=env, capture=True)

    top = Path(os.fsdecode(git("rev-parse", "--show-toplevel")).rstrip("\n"))
    if top.resolve() != repo.resolve():
        raise ValueError("package source must be the selected Git worktree")
    object_format = git("rev-parse", "--show-object-format").decode().strip()
    if object_format not in ("sha1", "sha256"):
        raise ValueError("unsupported Git object format")
    width = hashlib.new(object_format).digest_size * 2
    oid_pattern = re.compile(rf"[0-9a-f]{{{width}}}")

    # --end-of-options keeps a supplied revision from changing Git's mode.
    commit = git("rev-parse", "--verify", "--end-of-options", revision + "^{commit}").decode().strip()
    tree = git("rev-parse", "--verify", commit + "^{tree}").decode().strip()
    if not oid_pattern.fullmatch(commit) or not oid_pattern.fullmatch(tree):
        raise ValueError("invalid commit or tree identity")
    listing = git("ls-tree", "-r", "-t", "-z", "--full-tree", tree)
    if not listing.endswith(b"\x00"):
        raise ValueError("missing or incomplete Git tree listing")

    entries = {}
    for record in listing[:-1].split(b"\x00"):
        details, raw_path = record.split(b"\t", 1)
        mode, kind, oid = details.decode("ascii").split(" ")
        name = source_path(raw_path)
        parent = posixpath.dirname(name)
        if (SOURCE_MODES.get(mode) != kind or not oid_pattern.fullmatch(oid)
                or name in entries or (parent and entries.get(parent, ("",))[0] != "040000")):
            raise ValueError("unsupported or inconsistent entry in the selected Git tree")
        entries[name] = (mode, kind, oid)

    links = []
    # Request raw object bodies in one local process. Hash each body against
    # its object identity, including the commit and the selected tree.
    args = ["rtk", "proxy", *GIT, "cat-file", "--batch"]
    with subprocess.Popen(args, cwd=repo, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE) as objects:
        def read(oid, kind):
            objects.stdin.write(oid.encode("ascii") + b"\n")
            objects.stdin.flush()
            header = objects.stdout.readline().split()
            if len(header) != 3 or header[:2] != [oid.encode("ascii"), kind.encode("ascii")]:
                raise ValueError("required Git object is missing or has the wrong type")
            if not header[2].isdigit():
                raise ValueError("invalid Git object size")
            size = int(header[2])
            data = objects.stdout.read(size)
            identity = hashlib.new(object_format, f"{kind} {size}\0".encode("ascii") + data).hexdigest()
            if len(data) != size or objects.stdout.read(1) != b"\n" or identity != oid:
                raise ValueError("Git object bytes do not match the selected identity")
            return data

        if read(commit, "commit").split(b"\n", 1)[0] != b"tree " + tree.encode("ascii"):
            raise ValueError("commit does not bind the selected tree")
        read(tree, "tree")
        for name, (mode, kind, oid) in entries.items():
            output = source / name
            data = read(oid, kind)
            if mode == "040000":
                output.mkdir()
            elif mode == "120000":
                links.append((output, source_link(name, data, entries)))
            else:
                with output.open("xb") as stream:
                    stream.write(data)
                output.chmod(0o755 if mode == "100755" else 0o644)
        objects.stdin.close()
        if objects.wait() != 0:
            raise ValueError("local Git object read failed")
    # Links cannot become write destinations during tree materialization.
    for output, target in links:
        output.symlink_to(target)
    provenance = {
        "method": "verified-git-tree-objects-v1", "object_format": object_format,
        "tree_listing_sha256": hashlib.sha256(listing).hexdigest(),
        "entries": len(entries), "regular_files": sum(m in ("100644", "100755") for m, _, _ in entries.values()),
        "directories": sum(m == "040000" for m, _, _ in entries.values()),
        "executable_files": sum(m == "100755" for m, _, _ in entries.values()),
        "internal_file_symlinks": len(links),
    }
    return commit, tree, provenance


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


def build_frontend(source, profile, output):
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
        raise ValueError("installed toolchain does not match the pinned build profile")
    run("npm", "ci", *NPM_COMMAND_OPTIONS, "--include=dev", "--ignore-scripts", "--no-audit", "--no-fund",
        cwd=source, env=env)
    run("npm", "run", *NPM_COMMAND_OPTIONS, "build", "--", "--outDir", str(output), cwd=source, env=env)
    return env, effective_go, toolchain


def build_binary(source, env, command, output):
    run("go", "build", "-trimpath", "-buildvcs=false", "-mod=readonly", "-o", str(output),
        "./cmd/" + command, cwd=source, env=env)


def build(repo, revision, output):
    if output.exists() or output.is_symlink():
        raise ValueError("output directory already exists")
    with tempfile.TemporaryDirectory(prefix="closed-pilot-build-") as temporary:
        source = Path(temporary) / "source"
        bundle = Path(temporary) / "bundle"
        source.mkdir()
        bundle.mkdir()
        (bundle / "bin").mkdir()
        (bundle / "sql").mkdir()
        commit, tree, provenance = materialize_source(repo, revision, source)
        if any(source.glob(".env*")) or (source / ".npmrc").exists():
            raise ValueError("source tree must not supply dotenv or npm configuration inputs")
        if Path(__file__).read_bytes() != (source / "scripts/package-closed-pilot.py").read_bytes():
            raise ValueError("use the packager from the selected source revision")
        profile = json.loads((source / PROFILE).read_text())
        # Run the committed script's configured build, regardless of ambient
        # CareerOS/Vite flags or a different working tree's generated assets.
        env, effective_go, toolchain = build_frontend(source, profile, bundle / "frontend")
        for command in COMMANDS:
            build_binary(source, env, command, bundle / "bin" / command)

        for name in ("bootstrap.sql", "closed-runtime-grants.sql"):
            shutil.copyfile(source / "scripts/pilot-runtime" / name, bundle / "sql" / name)
        shutil.copyfile(source / PROFILE, bundle / "build-profile.json")
        manifest = {
            "bundle": profile["id"], "source_commit": commit, "source_tree": tree,
            "source_materialization": provenance,
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
