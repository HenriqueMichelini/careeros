#!/usr/bin/env python3
"""Render the closed Cloud Run service and migration job for local review."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import runpy


REGION = "southamerica-east1"
SERVICE = "careeros-294-closed"
JOB = "careeros-294-migrate"
RUNTIME_SECRETS = (
    ("CAREEROS_DATABASE_URL", "careeros-294-runtime-dsn"),
    ("CAREEROS_GOOGLE_CLIENT_ID", "careeros-294-google-client-id"),
    ("CAREEROS_GOOGLE_CLIENT_SECRET", "careeros-294-google-client-secret"),
    ("CAREEROS_HOSTED_GOOGLE_SUBJECTS", "careeros-294-google-subjects"),
)
MIGRATION_SECRETS = (
    ("CAREEROS_HOSTED_MIGRATOR_DATABASE_URL", "careeros-294-migrator-dsn"),
)


def secret_bindings(project_number, bindings, versions):
    annotations = ",".join(
        f"{secret}:projects/{project_number}/secrets/{secret}" for _, secret in bindings
    )
    env = [
        {"name": variable, "valueFrom": {"secretKeyRef": {"name": secret, "key": versions[secret]}}}
        for variable, secret in bindings
    ]
    return {"run.googleapis.com/secrets": annotations}, env


def render(project_id, project_number, repository, origin, images, versions):
    registry = f"{REGION}-docker.pkg.dev/{project_id}/{repository}"
    iam_domain = f"@{project_id}.iam.gserviceaccount.com"
    resources = {"limits": {"cpu": "1", "memory": "512Mi"}}
    revision, runtime_env = secret_bindings(project_number, RUNTIME_SECRETS, versions)
    revision["run.googleapis.com/cpu-throttling"] = "true"
    runtime_env += [
        {"name": "CAREEROS_PUBLIC_ORIGIN", "value": origin},
        {"name": "CAREEROS_GOOGLE_REDIRECT_URI", "value": origin + "/api/auth/google/callback"},
        {"name": "CAREEROS_SESSION_TTL", "value": "30m"},
        {"name": "CAREEROS_TRANSACTION_TTL", "value": "10m"},
        {"name": "CAREEROS_JWKS_CACHE_TTL", "value": "1h"},
    ]
    service = {
        "apiVersion": "serving.knative.dev/v1",
        "kind": "Service",
        "metadata": {
            "name": SERVICE,
            "namespace": project_number,
            "annotations": {
                "run.googleapis.com/ingress": "all",
                "run.googleapis.com/invoker-iam-disabled": "true",
                "run.googleapis.com/minScale": "0",
                "run.googleapis.com/maxScale": "1",
            },
        },
        "spec": {
            "template": {
                "metadata": {"annotations": revision},
                "spec": {
                    "serviceAccountName": "careeros-294-serving" + iam_domain,
                    "timeoutSeconds": 45,
                    "containerConcurrency": 8,
                    "containers": [{
                        "image": f"{registry}/pilot-runtime-hosted@{images['serving']}",
                        "ports": [{"name": "http1", "containerPort": 8080}],
                        "resources": resources,
                        "env": runtime_env,
                        "startupProbe": {
                            "tcpSocket": {"port": 8080},
                            "initialDelaySeconds": 0,
                            "timeoutSeconds": 1,
                            "periodSeconds": 5,
                            "failureThreshold": 4,
                        },
                    }],
                },
            },
            "traffic": [{"latestRevision": True, "percent": 100}],
        },
    }

    execution, job_env = secret_bindings(project_number, MIGRATION_SECRETS, versions)
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
                            "containers": [{
                                "image": f"{registry}/pilot-runtime-hosted-migrate@{images['migration']}",
                                "resources": resources,
                                "env": job_env,
                            }],
                        },
                    },
                },
            },
        },
    }
    return {"service.yaml": service, "migration-job.yaml": job}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("project-id", "project-number", "repository", "origin", "expected-manifest-sha256"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--bundle", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True, help="new local output directory")
    for _, secret in (*RUNTIME_SECRETS, *MIGRATION_SECRETS):
        parser.add_argument(f"--{secret}-version", required=True)
    args = parser.parse_args()

    try:
        if not re.fullmatch(r"[a-z][a-z0-9-]{4,28}[a-z0-9]", args.project_id):
            raise ValueError("a Google Cloud project ID is required")
        if not re.fullmatch(r"[1-9][0-9]{5,19}", args.project_number):
            raise ValueError("a Google Cloud project number is required")
        if not re.fullmatch(r"[a-z][a-z0-9-]{0,61}[a-z0-9]", args.repository):
            raise ValueError("a dedicated Artifact Registry repository ID is required")
        if args.origin != f"https://{SERVICE}-{args.project_number}.{REGION}.run.app":
            raise ValueError("the exact stable Cloud Run HTTPS origin is required")
        versions = {
            secret: getattr(args, secret.replace("-", "_") + "_version")
            for _, secret in (*RUNTIME_SECRETS, *MIGRATION_SECRETS)
        }
        if any(not re.fullmatch(r"[1-9][0-9]*", version) for version in versions.values()):
            raise ValueError("secret bindings must reference numbered versions")

        package = runpy.run_path(str(Path(__file__).with_name("package-hosted-pilot.py")))
        manifest = package["verify"](args.bundle, args.expected_manifest_sha256)
        if manifest["source"]["method"] != "verified-git-tree-objects-v1":
            raise ValueError("the review configuration requires a committed-source bundle")
        images = manifest["images"]
        if set(images) != {"serving", "migration"} or any(
            not re.fullmatch(r"sha256:[a-f0-9]{64}", digest) for digest in images.values()
        ):
            raise ValueError("the bundle must identify both images by digest")
        documents = render(args.project_id, args.project_number, args.repository, args.origin, images, versions)
        args.output.mkdir(mode=0o700)
        for name, document in documents.items():
            data = (json.dumps(document, indent=2, sort_keys=True) + "\n").encode()
            (args.output / name).write_bytes(data)
            print(f"{hashlib.sha256(data).hexdigest()}  {name}")
    except (OSError, ValueError, KeyError) as exc:
        parser.exit(1, f"closed hosted pilot configuration failed: {exc}\n")


if __name__ == "__main__":
    main()
