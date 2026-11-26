#!/usr/bin/env python3
"""Render the closed Cloud Run service and migration job for local review."""

import argparse
from datetime import datetime
import hashlib
import json
from pathlib import Path
import re
import runpy


REGION = "southamerica-east1"
SERVICE = "careeros-294-closed"
JOB = "careeros-294-migrate"
ATTESTOR = "careeros-294-attestor"
RUNTIME_SECRETS = (
    ("CAREEROS_DATABASE_URL", "careeros-294-runtime-dsn"),
    ("CAREEROS_GOOGLE_CLIENT_ID", "careeros-294-google-client-id"),
    ("CAREEROS_GOOGLE_CLIENT_SECRET", "careeros-294-google-client-secret"),
    ("CAREEROS_HOSTED_GOOGLE_SUBJECTS", "careeros-294-google-subjects"),
)
MIGRATION_SECRETS = (
    ("CAREEROS_HOSTED_MIGRATOR_DATABASE_URL", "careeros-294-migrator-dsn"),
)
DRAFT_TOKEN = ("CAREEROS_HOSTED_DRAFT_ATTESTOR_TOKEN", "careeros-294-draft-attestor-token")
DRAFT_RUNTIME_SECRETS = (
    ("CAREEROS_HOSTED_DRAFT_LEDGER_DATABASE_URL", "careeros-294-draft-ledger-dsn"),
    ("CAREEROS_HOSTED_DRAFT_ACTION_KEY", "careeros-294-draft-action-key"),
    ("CAREEROS_HOSTED_DRAFT_ARTIFACT_KEY", "careeros-294-draft-artifact-key"),
    DRAFT_TOKEN,
)
ATTESTOR_SECRETS = (
    ("CAREEROS_HOSTED_DRAFT_ATTESTOR_DATABASE_URL", "careeros-294-draft-attestor-dsn"),
    DRAFT_TOKEN,
)


def service_origin(name, project_number):
    return f"https://{name}-{project_number}.{REGION}.run.app"


def secret_bindings(project_number, bindings, versions):
    annotations = ",".join(
        f"{secret}:projects/{project_number}/secrets/{secret}" for _, secret in bindings
    )
    env = [
        {"name": variable, "valueFrom": {"secretKeyRef": {"name": secret, "key": versions[secret]}}}
        for variable, secret in bindings
    ]
    return {"run.googleapis.com/secrets": annotations}, env


def service_document(name, project_number, account, container, annotations, *, timeout, concurrency, public):
    annotations["run.googleapis.com/cpu-throttling"] = "true"
    container["ports"] = [{"name": "http1", "containerPort": 8080}]
    container["startupProbe"] = {
        "tcpSocket": {"port": 8080},
        "initialDelaySeconds": 0,
        "timeoutSeconds": 1,
        "periodSeconds": 5,
        "failureThreshold": 4,
    }
    return {
        "apiVersion": "serving.knative.dev/v1",
        "kind": "Service",
        "metadata": {
            "name": name,
            "namespace": project_number,
            "annotations": {
                "run.googleapis.com/ingress": "all",
                "run.googleapis.com/invoker-iam-disabled": "true" if public else "false",
                "run.googleapis.com/minScale": "0",
                "run.googleapis.com/maxScale": "1",
            },
        },
        "spec": {
            "template": {
                "metadata": {"annotations": annotations},
                "spec": {
                    "serviceAccountName": account,
                    "timeoutSeconds": timeout,
                    "containerConcurrency": concurrency,
                    "containers": [container],
                },
            },
            "traffic": [{"latestRevision": True, "percent": 100}],
        },
    }


def render(project_id, project_number, repository, origin, images, versions, *, draft=None):
    registry = f"{REGION}-docker.pkg.dev/{project_id}/{repository}"
    iam_domain = f"@{project_id}.iam.gserviceaccount.com"
    serving_account = "careeros-294-serving" + iam_domain
    resources = {"limits": {"cpu": "1", "memory": "512Mi"}}
    runtime_secrets = RUNTIME_SECRETS + (DRAFT_RUNTIME_SECRETS if draft else ())
    revision, runtime_env = secret_bindings(project_number, runtime_secrets, versions)
    runtime_env += [
        {"name": "CAREEROS_PUBLIC_ORIGIN", "value": origin},
        {"name": "CAREEROS_GOOGLE_REDIRECT_URI", "value": origin + "/api/auth/google/callback"},
        {"name": "CAREEROS_SESSION_TTL", "value": "30m"},
        {"name": "CAREEROS_TRANSACTION_TTL", "value": "10m"},
        {"name": "CAREEROS_JWKS_CACHE_TTL", "value": "1h"},
    ]
    serving = {
        "image": f"{registry}/pilot-runtime-hosted@{images['serving']}",
        "resources": resources,
        "env": runtime_env,
    }
    execution, job_env = secret_bindings(project_number, MIGRATION_SECRETS, versions)
    migration = {
        "image": f"{registry}/pilot-runtime-hosted-migrate@{images['migration']}",
        "resources": resources,
        "env": job_env,
    }
    if draft:
        runtime_env.append({"name": "CAREEROS_HOSTED_DRAFT_ATTESTOR_ORIGIN", "value": draft["origin"]})
        serving["command"] = ["/app/pilot-runtime-hosted"]
        serving["args"] = ["-assets", "/app/dist", "-scoped-fake-draft"]
        job_env += [
            {"name": "CAREEROS_HOSTED_DRAFT_ACTION_KEY_SHA256", "value": draft["action_key_sha256"]},
            {"name": "CAREEROS_HOSTED_DRAFT_ARTIFACT_KEY_SHA256", "value": draft["artifact_key_sha256"]},
            {"name": "CAREEROS_HOSTED_DRAFT_EXPIRES_AT", "value": draft["expires_at"]},
        ]
        migration["args"] = ["-scoped-fake-draft"]

    service = service_document(SERVICE, project_number, serving_account, serving, revision,
                               timeout=45, concurrency=8, public=True)
    job = {
        "apiVersion": "run.googleapis.com/v1",
        "kind": "Job",
        "metadata": {"name": JOB, "namespace": project_number},
        "spec": {
            "template": {
                "metadata": {"annotations": execution},
                "spec": {
                    "parallelism": 1,
                    "taskCount": 1,
                    "template": {
                        "spec": {
                            "serviceAccountName": "careeros-294-migration" + iam_domain,
                            "timeoutSeconds": 60,
                            "maxRetries": 0,
                            "containers": [migration],
                        },
                    },
                },
            },
        },
    }
    documents = {"service.yaml": service, "migration-job.yaml": job}
    if draft:
        attestor_revision, attestor_env = secret_bindings(project_number, ATTESTOR_SECRETS, versions)
        attestor = {
            "image": f"{registry}/pilot-runtime-hosted-attestor@{images['attestor']}",
            "resources": resources,
            "env": attestor_env,
        }
        documents["attestor-service.yaml"] = service_document(
            ATTESTOR, project_number, ATTESTOR + iam_domain, attestor, attestor_revision,
            timeout=10, concurrency=1, public=False,
        )
        documents["attestor-invoker-policy.json"] = {
            "version": 1,
            "bindings": [{"role": "roles/run.invoker", "members": ["serviceAccount:" + serving_account]}],
        }
    return documents


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("project-id", "project-number", "repository", "origin", "expected-manifest-sha256"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--bundle", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True, help="new local output directory")
    parser.add_argument("--scoped-fake-draft", action="store_true")
    parser.add_argument("--attestor-origin")
    parser.add_argument("--draft-action-key-sha256", help="lowercase SHA-256 of the decoded scoped Draft action key")
    parser.add_argument("--draft-artifact-key-sha256", help="lowercase SHA-256 of the decoded scoped Draft artifact key")
    parser.add_argument("--draft-expires-at")
    base_secrets = tuple(secret for _, secret in (*RUNTIME_SECRETS, *MIGRATION_SECRETS))
    draft_secrets = tuple(dict.fromkeys(secret for _, secret in (*DRAFT_RUNTIME_SECRETS, *ATTESTOR_SECRETS)))
    for secret in base_secrets:
        parser.add_argument(f"--{secret}-version", required=True)
    for secret in draft_secrets:
        parser.add_argument(f"--{secret}-version")
    args = parser.parse_args()

    try:
        if not re.fullmatch(r"[a-z][a-z0-9-]{4,28}[a-z0-9]", args.project_id):
            raise ValueError("a Google Cloud project ID is required")
        if not re.fullmatch(r"[1-9][0-9]{5,19}", args.project_number):
            raise ValueError("a Google Cloud project number is required")
        if not re.fullmatch(r"[a-z][a-z0-9-]{0,61}[a-z0-9]", args.repository):
            raise ValueError("a dedicated Artifact Registry repository ID is required")
        if args.origin != service_origin(SERVICE, args.project_number):
            raise ValueError("the exact stable Cloud Run HTTPS origin is required")
        versions = {
            secret: getattr(args, secret.replace("-", "_") + "_version")
            for secret in (*base_secrets, *draft_secrets)
        }
        draft = {
            "origin": args.attestor_origin, "action_key_sha256": args.draft_action_key_sha256,
            "artifact_key_sha256": args.draft_artifact_key_sha256,
            "expires_at": args.draft_expires_at,
        }
        if args.scoped_fake_draft:
            if args.attestor_origin != service_origin(ATTESTOR, args.project_number):
                raise ValueError("the exact stable Cloud Run attestor HTTPS origin is required")
            if not re.fullmatch(r"[a-f0-9]{64}", args.draft_action_key_sha256 or ""):
                raise ValueError("the scoped Draft action key SHA-256 is required")
            if not re.fullmatch(r"[a-f0-9]{64}", args.draft_artifact_key_sha256 or ""):
                raise ValueError("the scoped Draft artifact key SHA-256 is required")
            if args.draft_action_key_sha256 == args.draft_artifact_key_sha256:
                raise ValueError("the scoped Draft action and artifact key digests must be distinct")
            expiry = args.draft_expires_at or ""
            if not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z", expiry):
                raise ValueError("the scoped Draft expiry must be an exact UTC timestamp")
            datetime.strptime(expiry, "%Y-%m-%dT%H:%M:%SZ")
        elif any(value is not None for value in (*draft.values(), *(versions[name] for name in draft_secrets))):
            raise ValueError("Draft configuration requires --scoped-fake-draft")
        needed_secrets = base_secrets + (draft_secrets if args.scoped_fake_draft else ())
        if any(not re.fullmatch(r"[1-9][0-9]*", versions[secret] or "") for secret in needed_secrets):
            raise ValueError("secret bindings must reference numbered versions")

        package = runpy.run_path(str(Path(__file__).with_name("package-hosted-pilot.py")))
        manifest = package["verify"](args.bundle, args.expected_manifest_sha256)
        if manifest["source"]["method"] != "verified-git-tree-objects-v1":
            raise ValueError("the review configuration requires a committed-source bundle")
        if args.scoped_fake_draft:
            if manifest["bundle"] != package["DRAFT_FORMAT"]:
                raise ValueError("scoped fake Draft requires a bundle with a separate attestor image")
            own_digest = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
            if manifest["inputs"][package["HOSTED_RENDERER"]]["sha256"] != own_digest:
                raise ValueError("scoped fake Draft rendering requires the bundle's reviewed renderer")
        documents = render(args.project_id, args.project_number, args.repository, args.origin,
                           manifest["images"], versions, draft=draft if args.scoped_fake_draft else None)
        args.output.mkdir(mode=0o700)
        for name, document in documents.items():
            data = (json.dumps(document, indent=2, sort_keys=True) + "\n").encode()
            (args.output / name).write_bytes(data)
            print(f"{hashlib.sha256(data).hexdigest()}  {name}")
    except (OSError, ValueError, KeyError) as exc:
        parser.exit(1, f"closed hosted pilot configuration failed: {exc}\n")


if __name__ == "__main__":
    main()
