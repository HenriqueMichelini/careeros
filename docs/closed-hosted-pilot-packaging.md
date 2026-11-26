# Closed hosted pilot packaging

The [#294 local packaging tool](../scripts/package-hosted-pilot.py) prepares two Linux amd64 OCI layouts for review. The serving image contains the static Go command, production Vite assets and pinned TLS roots. Its entrypoint runs as numeric user and group 65532 and starts `/app/pilot-runtime-hosted -assets /app/dist`. The separate migration image runs `/app/pilot-runtime-hosted-migrate` with the same TLS roots. The outer manifest pins the Account schema and hosted grants used by migration and the separate administrator bootstrap SQL. Supply platform configuration and each image's distinct database secret reference separately at runtime. [Hosted configuration proposal](closed-pilot-hosted-proposal.md)

## Build and verify locally

Use the packagers from the chosen source, the Go, Node and npm versions in the [build profile](../scripts/pilot-runtime/build-profile.json), Python, RTK, cached Go/npm dependencies, and a reviewed PEM CA bundle. Keep the CA source digest independently with the input review. The tool checks the exact input bytes and includes only certificates in the output, with the input and packaged digests recorded separately.

```sh
rtk proxy python3 scripts/package-hosted-pilot.py build \
  --revision REVIEWED_COMMIT --output /path/to/new-bundle \
  --ca-bundle /path/to/reviewed-ca-bundle.pem \
  --expected-ca-bundle-sha256 REVIEWED_CA_DIGEST
rtk proxy python3 scripts/package-hosted-pilot.py verify \
  --bundle /path/to/new-bundle --expected-manifest-sha256 TRUSTED_MANIFEST_DIGEST
rtk proxy skopeo inspect --config oci:/path/to/new-bundle/serving
rtk proxy skopeo inspect --config oci:/path/to/new-bundle/migration
```

The committed-source mode uses the [#292 Git object verifier](closed-pilot-runtime-local.md#reproduce-and-inspect-the-package). It materializes exact local objects without applying checkout filters or fetching missing objects. The shared offline builder uses the closed frontend settings and compiles both commands with a pinned Go target, CGO disabled and trimmed paths. OCI layers use stable paths, file modes, ownership and timestamps. Build in a private directory and compare two fresh builds of the same inputs. Retain the expected outer manifest digest and both image digests with the reviewed source, pinned CA input, and configuration record. Recheck the exact image digests if any later operation transforms the manifest or layers.

For a local build before committing, replace `--revision REVIEWED_COMMIT` with `--working-tree` and supply one `--include` for each untracked source file or directory needed by the candidate. This mode materializes tracked working-tree paths and only the explicitly selected untracked paths. It reports the baseline commit and the SHA-256 of the copied files and modes. The image carries the worktree snapshot digest in place of a commit revision. Keep the worktree stable while it is copied. Use the committed-source mode to freeze the final artifact.

Each OCI layout can be inspected independently. The bundle verifier checks the outer manifest against a trusted expectation and checks every packaged file against it. It rejects extra or changed files and symlinks. Publish and execute decisions belong to the separate hosted configuration and bounded execution approvals. The package contains no credentials, live provider configuration or database contents. Keep both the migration and verification work inside approved roles and environments. [#294 preflight and approval inputs](closed-hosted-candidate-preflight.md#sign-off-inputs-and-bounded-observation-plan)

## Render the closed Cloud Run configuration

Render the service and migration job locally after selecting the dedicated project, Artifact Registry repository, stable service origin and numbered versions for the five secret references. Set up the serving and migration principals as separate identities in that project. The renderer accepts a committed-source bundle and checks it against the expected manifest digest before generating the files. [Hosted configuration proposal](closed-pilot-hosted-proposal.md) · [Cloud Run service schema](https://docs.cloud.google.com/run/docs/reference/yaml/v1) · [Cloud Run job secret configuration](https://docs.cloud.google.com/run/docs/configuring/jobs/secrets)

```sh
rtk proxy python3 scripts/render-closed-hosted-pilot.py \
  --bundle "$CANDIDATE_BUNDLE" --expected-manifest-sha256 "$CANDIDATE_MANIFEST_SHA256" \
  --project-id "$CANDIDATE_PROJECT_ID" --project-number "$CANDIDATE_PROJECT_NUMBER" \
  --repository "$CANDIDATE_REGISTRY_REPOSITORY" --origin "$CANDIDATE_ORIGIN" \
  --careeros-294-runtime-dsn-version "$RUNTIME_DSN_VERSION" \
  --careeros-294-migrator-dsn-version "$MIGRATOR_DSN_VERSION" \
  --careeros-294-google-client-id-version "$GOOGLE_CLIENT_ID_VERSION" \
  --careeros-294-google-client-secret-version "$GOOGLE_CLIENT_SECRET_VERSION" \
  --careeros-294-google-subjects-version "$GOOGLE_SUBJECTS_VERSION" \
  --output "$CANDIDATE_CONFIG_DIRECTORY"
```

Record the two emitted file digests and confirm them against the approved candidate. The service manifest enables public invocation and routes traffic to the latest closed revision. Prepare and review the applicable default, custom, parent and exported logging sinks before applying it. Confirm the service URL, secret and IAM bindings, database configuration and other review inputs before external execution.
