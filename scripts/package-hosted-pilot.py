#!/usr/bin/env python3
"""Build and verify offline OCI layouts for the closed hosted pilot."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import runpy
import shutil
import ssl
import stat
import subprocess
import tarfile
import tempfile


HERE = Path(__file__).resolve().parent
COMMON = runpy.run_path(str(HERE / "package-closed-pilot.py"))
PROFILE = COMMON["PROFILE"]
ACCOUNT_SCHEMA = "internal/accountstore/schema.sql"
HOSTED_GRANTS = "internal/accountstore/hosted_grants.sql"
HOSTED_RENDERER = "scripts/render-closed-hosted-pilot.py"
INPUTS = (
    "go.mod", "go.sum", "package-lock.json", PROFILE, "vite.config.ts",
    "scripts/package-closed-pilot.py", "scripts/package-hosted-pilot.py",
    ACCOUNT_SCHEMA, "internal/accountstore/hosted_bootstrap.sql", HOSTED_GRANTS,
)
DRAFT_INPUTS = (
    HOSTED_RENDERER,
    "internal/hosteddraft/bootstrap.sql", "internal/hosteddraft/migrate.sql", "internal/hosteddraft/attestor.sql",
    "internal/aicontrol/schema.sql", "internal/aicontrol/operations_capture.sql", "internal/aicontrol/reconciliation.sql",
    "internal/aicontrol/hosted_draft.sql", "internal/draftrecovery/schema.sql", "internal/draftrecovery/hosted.sql",
)
SERVE = "pilot-runtime-hosted"
MIGRATE = "pilot-runtime-hosted-migrate"
ATTEST = "pilot-runtime-hosted-attestor"
FORMAT = "closed-pilot-hosted-oci-v1"
DRAFT_FORMAT = "closed-pilot-hosted-oci-v2"
IMAGE_NAMES = {
    FORMAT: frozenset(("serving", "migration")),
    DRAFT_FORMAT: frozenset(("serving", "migration", "attestor")),
}
CA_PATH = "etc/ssl/certs/ca-certificates.crt"
OCI = "application/vnd.oci.image."
SHA256 = re.compile(r"[a-f0-9]{64}")
CERTIFICATES = re.compile(rb"(?:\s*-----BEGIN CERTIFICATE-----\s+[A-Za-z0-9+/=\s]+?-----END CERTIFICATE-----)+\s*")


def encoded(document):
    return (json.dumps(document, sort_keys=True, separators=(",", ":")) + "\n").encode()


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def snapshot_worktree(repo, includes, source):
    """Capture reviewed paths for local testing without presenting them as a commit."""
    def git(*args):
        return COMMON["run"](*COMMON["GIT"], "--literal-pathspecs", *args,
                             cwd=repo, env=COMMON["git_environment"](), capture=True)

    named = [COMMON["source_path"](os.fsencode(name)) for name in includes]
    raw = git("ls-files", "--cached", "-z")
    if named:
        raw += git("ls-files", "--others", "--exclude-standard", "-z", "--", *named)
    names = sorted({COMMON["source_path"](name) for name in raw.split(b"\x00") if name})
    entries = {}
    links = []
    fingerprints = {}
    for name in names:
        original = repo / name
        parent = original.parent
        while parent != repo:
            if not stat.S_ISDIR(parent.lstat().st_mode):
                raise ValueError("working-tree input contains a non-directory ancestor")
            entries[parent.relative_to(repo).as_posix()] = ("040000",)
            parent = parent.parent
        output = source / name
        output.parent.mkdir(parents=True, exist_ok=True)
        mode = original.lstat().st_mode
        if stat.S_ISLNK(mode):
            data = os.fsencode(os.readlink(original))
            git_mode = "120000"
            links.append((name, data))
        elif stat.S_ISREG(mode):
            data = original.read_bytes()
            git_mode = "100755" if mode & 0o111 else "100644"
            output.write_bytes(data)
            output.chmod(0o755 if git_mode == "100755" else 0o644)
        else:
            raise ValueError("unsupported working-tree input type")
        entries[name] = (git_mode,)
        fingerprints[name] = [git_mode, sha256(data)]
    for name, data in links:
        (source / name).symlink_to(COMMON["source_link"](name, data, entries))
    return {
        "method": "uncommitted-working-tree-snapshot-v1",
        "baseline_commit": git("rev-parse", "--verify", "HEAD^{commit}").decode().strip(),
        "snapshot_sha256": sha256(encoded(fingerprints)),
        "untracked_paths_requested": sorted(set(named)),
        "files": len(fingerprints),
    }


def certificate_bundle(path, expected):
    data = path.read_bytes()
    if not SHA256.fullmatch(expected) or sha256(data) != expected:
        raise ValueError("CA bundle does not match its independently pinned SHA-256")
    # Distribution trust stores can annotate PEM certificates with comments.
    pem = b"\n".join(line for line in data.splitlines() if not line.lstrip().startswith(b"#")) + b"\n"
    if not CERTIFICATES.fullmatch(pem):
        raise ValueError("CA bundle must contain only PEM certificates and comments")
    ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT).load_verify_locations(cadata=pem.decode("ascii"))
    return pem


def write_layer(rootfs, target):
    with tarfile.open(target, "w", format=tarfile.USTAR_FORMAT) as archive:
        for entry in sorted(rootfs.rglob("*")):
            path = entry.relative_to(rootfs).as_posix()
            mode = entry.lstat().st_mode
            header = tarfile.TarInfo(path)
            if stat.S_ISDIR(mode):
                header.type = tarfile.DIRTYPE
                header.mode = 0o1777 if path == "tmp" else 0o755
                archive.addfile(header)
            elif stat.S_ISREG(mode):
                header.mode = 0o555 if mode & 0o111 else 0o444
                header.size = entry.stat().st_size
                with entry.open("rb") as stream:
                    archive.addfile(header, stream)
            else:
                raise ValueError("image root must contain only regular files and directories")


def image(layout, rootfs, entrypoint, source_label):
    blobs = layout / "blobs" / "sha256"
    blobs.mkdir(parents=True)
    layer = layout / "rootfs.tar"
    write_layer(rootfs, layer)
    contents = COMMON["digest"](layer)
    layer.rename(blobs / contents["sha256"])
    layer_digest = "sha256:" + contents["sha256"]

    def put(document, media_type):
        data = encoded(document)
        digest = sha256(data)
        (blobs / digest).write_bytes(data)
        return {"mediaType": media_type, "digest": "sha256:" + digest, "size": len(data)}

    config = put({
        "architecture": "amd64", "os": "linux",
        "config": {
            "Entrypoint": entrypoint,
            "Env": ["SSL_CERT_FILE=/" + CA_PATH],
            "Labels": source_label,
            "User": "65532:65532", "WorkingDir": "/app",
        },
        "rootfs": {"type": "layers", "diff_ids": [layer_digest]},
    }, OCI + "config.v1+json")
    manifest = put({
        "schemaVersion": 2, "mediaType": OCI + "manifest.v1+json", "config": config,
        "layers": [{"mediaType": OCI + "layer.v1.tar", "digest": layer_digest, "size": contents["bytes"]}],
    }, OCI + "manifest.v1+json")
    manifest.update({"platform": {"architecture": "amd64", "os": "linux"}})
    (layout / "index.json").write_bytes(encoded({"schemaVersion": 2, "manifests": [manifest]}))
    (layout / "oci-layout").write_bytes(encoded({"imageLayoutVersion": "1.0.0"}))
    return manifest["digest"]


def root_with_ca(root, certificates):
    (root / "app").mkdir(parents=True)
    (root / "tmp").mkdir()
    ca = root / CA_PATH
    ca.parent.mkdir(parents=True)
    ca.write_bytes(certificates)


def build(args):
    repo = HERE.parent
    if args.include and not args.working_tree:
        raise ValueError("--include requires --working-tree")
    if args.output.exists() or args.output.is_symlink():
        raise ValueError("output directory already exists")
    certificates = certificate_bundle(args.ca_bundle, args.expected_ca_bundle_sha256)
    with tempfile.TemporaryDirectory(prefix="hosted-pilot-build-") as directory:
        temporary = Path(directory)
        source = temporary / "source"
        source.mkdir()
        if args.working_tree:
            provenance = snapshot_worktree(repo, args.include, source)
            label = {"careeros.source.working-tree": provenance["snapshot_sha256"]}
        else:
            commit, tree, provenance = COMMON["materialize_source"](repo, args.revision, source)
            provenance.update({"source_commit": commit, "source_tree": tree})
            label = {"org.opencontainers.image.revision": commit}
        for script in ("package-hosted-pilot.py", "package-closed-pilot.py"):
            if (HERE / script).read_bytes() != (source / "scripts" / script).read_bytes():
                raise ValueError("use both packagers from the selected source")
        if any(source.glob(".env*")) or (source / ".npmrc").exists():
            raise ValueError("source must not supply dotenv or npm configuration inputs")
        profile = json.loads((source / PROFILE).read_text())
        required_target = {"GOOS": "linux", "GOARCH": "amd64", "GOAMD64": "v1", "CGO_ENABLED": "0", "GOEXPERIMENT": ""}
        if profile["go_target"] != required_target:
            raise ValueError("build profile does not match the Cloud Run target")
        input_paths = INPUTS + (DRAFT_INPUTS if args.scoped_fake_draft else ())
        inputs = {path: COMMON["digest"](source / path) for path in input_paths}
        schema_id = sha256((source / ACCOUNT_SCHEMA).read_bytes() + b"\x00" + (source / HOSTED_GRANTS).read_bytes())

        service = temporary / "service"
        migration = temporary / "migration"
        root_with_ca(service, certificates)
        root_with_ca(migration, certificates)
        assets = service / "app" / "dist"
        env, effective_go, toolchain = COMMON["build_frontend"](source, profile, assets)
        if not (assets / "index.html").is_file():
            raise ValueError("frontend shell missing from build")
        COMMON["build_binary"](source, env, SERVE, service / "app" / SERVE)
        COMMON["build_binary"](source, env, MIGRATE, migration / "app" / MIGRATE)

        result = temporary / "bundle"
        result.mkdir()
        images = {
            "serving": image(result / "serving", service, ["/app/" + SERVE, "-assets", "/app/dist"], label),
            "migration": image(result / "migration", migration, ["/app/" + MIGRATE], label),
        }
        if args.scoped_fake_draft:
            attestor = temporary / "attestor"
            root_with_ca(attestor, certificates)
            COMMON["build_binary"](source, env, ATTEST, attestor / "app" / ATTEST)
            images["attestor"] = image(result / "attestor", attestor, ["/app/" + ATTEST], label)
        document = encoded({
            "bundle": DRAFT_FORMAT if args.scoped_fake_draft else FORMAT,
            "source": provenance, "inputs": inputs, "effective_go": effective_go,
            "toolchain": toolchain, "npm_options": list(COMMON["NPM_COMMAND_OPTIONS"]),
            "hosted_account_schema_sha256": schema_id,
            "ca_bundle_source_sha256": args.expected_ca_bundle_sha256,
            "ca_bundle_payload_sha256": sha256(certificates),
            "images": images,
            "payload": COMMON["payload"](result),
        })
        (result / "manifest.json").write_bytes(document)
        args.output.mkdir(mode=0o700)
        shutil.copytree(result, args.output, dirs_exist_ok=True)
    verify(args.output, sha256(document))


def verify(bundle, expected):
    manifest_path = bundle / "manifest.json"
    if bundle.is_symlink() or not stat.S_ISREG(manifest_path.lstat().st_mode):
        raise ValueError("regular bundle directory and manifest required")
    document = manifest_path.read_bytes()
    if not SHA256.fullmatch(expected) or sha256(document) != expected:
        raise ValueError("bundle manifest does not match the independently expected SHA-256")
    manifest = json.loads(document)
    names = IMAGE_NAMES.get(manifest.get("bundle"))
    if names is None or COMMON["payload"](bundle) != manifest.get("payload"):
        raise ValueError("bundle payload does not match the hosted manifest")
    images = manifest.get("images")
    if not isinstance(images, dict) or set(images) != names:
        raise ValueError("bundle image inventory does not match the hosted format")
    for name, digest in images.items():
        if not isinstance(digest, str) or not digest.startswith("sha256:") or not SHA256.fullmatch(digest[7:]):
            raise ValueError("invalid hosted image digest")
        layout = bundle / name
        descriptors = json.loads((layout / "index.json").read_bytes())["manifests"]
        if len(descriptors) != 1 or descriptors[0]["digest"] != digest:
            raise ValueError("hosted image digest does not match its OCI index")
        descriptor = descriptors[0]
        if descriptor["platform"] != {"architecture": "amd64", "os": "linux"}:
            raise ValueError("hosted image platform does not match the pinned target")
        blob = COMMON["digest"](layout / "blobs" / "sha256" / digest[7:])
        if blob != {"sha256": digest[7:], "bytes": descriptor["size"]}:
            raise ValueError("hosted OCI image manifest does not match the index")
    print(f"Verified {manifest['bundle']} source={manifest['source']['method']} manifest={expected}")
    for name, digest in images.items():
        print(f"{name}={digest}")
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    modes = parser.add_subparsers(dest="mode", required=True)
    package = modes.add_parser("build")
    source = package.add_mutually_exclusive_group(required=True)
    source.add_argument("--revision")
    source.add_argument("--working-tree", action="store_true")
    package.add_argument("--include", action="append", default=[])
    package.add_argument("--scoped-fake-draft", action="store_true", help="add the isolated fake Draft attestor image")
    package.add_argument("--output", type=lambda path: Path(path).absolute(), required=True)
    package.add_argument("--ca-bundle", type=Path, required=True)
    package.add_argument("--expected-ca-bundle-sha256", required=True)
    checking = modes.add_parser("verify")
    checking.add_argument("--bundle", type=Path, required=True)
    checking.add_argument("--expected-manifest-sha256", required=True)
    args = parser.parse_args()
    try:
        if args.mode == "build":
            build(args)
        else:
            verify(args.bundle, args.expected_manifest_sha256)
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as exc:
        parser.exit(1, f"hosted pilot package failed: {exc}\n")


if __name__ == "__main__":
    main()
