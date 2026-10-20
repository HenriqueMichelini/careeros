# Local co-build manifests

`CaptureBuildManifest` records versioned, content-addressed inputs for the seven
registered native adapters. `scripts/lab.mjs` runs the owned manifest generator,
embeds its complete JSON in the API executable, builds with a fixed Go recipe,
checks the source manifest again, and atomically writes
`lab/.bin/lab-build-receipt.json`. The ignored receipt binds the manifest hash,
actual executable SHA-256, launcher source SHA-256, Go runtime/target and normalized
build recipe. The variable linker argument is the base64 JSON identified by the
manifest hash. Ambient Go flags/workspaces/toolchain selection are disabled; the
build uses the installed toolchain, cached modules and CGO disabled.

At startup the API compares its compiled manifest with freshly captured launch
inputs and checks the receipt against the running executable digest. On Linux it
hashes `/proc/self/exe`; other platforms use the executable path. Known mismatches
stop startup before history or network listeners are opened. Missing compiled
manifests or receipts stay `unavailable`, allowing ordinary local Go builds to
retain the existing native Lab functionality without claiming build verification.
`NewPlanner` also rejects a supplied mismatched or unknown correspondence status.

`node scripts/lab.mjs --build-info` builds and inspects the real executable without
building UI assets, opening history, or serving. `lab/.bin/lab-api --build-info`
inspects the existing build. JSON includes correspondence and the catalog; a
mismatch exits with status 1. `node tests/lab-build.check.mjs` verifies the actual
co-build and rejects a changed classification template, receipt and executable;
missing receipt evidence is explicitly unavailable. The smoke uses temporary
synthetic source copies and restores its ignored artifacts.

## Identity meanings

- Implementation, inline prompt, schema-builder and strategy identities are
  conservative `source-envelope` manifests. Their dependency reference resolves
  to the complete `sources` map in the build manifest. They deliberately change
  together when any captured dependency changes. They cannot establish isolated
  prompt attribution or an exact rendered template version.
- `templateArtifacts` is an `explicit-manifest` of the enumerated
  `internal/fieldvalidation/questions.json` bytes. It does not identify all prompts
  as exact templates. Missing explicit artifacts fail capture.
- The captured source set includes the existing broad launch snapshot, the
  manifest generator, module/checksum files and package locks. Go standard-library
  and external module identities rely on the installed toolchain and normal
  checksum-verified module build, rather than independently hashing their caches.
- Catalog and resolved descriptors expose inert compiled-manifest references.
  Ordinary Go builds explicitly report unavailable compiled identity.
  `compiled-identity-only` does not claim verified correspondence. Plans and run
  records retain the launch correspondence and full manifest under additive
  `provenance.build`. The existing commit, dirty/source hashes, unavailable reasons,
  fixture identities and prompt/strategy envelope fields keep their meanings.
- Rendered per-call `PromptHash`, `SchemaHash` and `RequestHash` remain separate
  observations. Equal `first256InstructionCharactersHash` values never establish
  equal complete prompts. Case data is not converted into a static template ID.

This is correspondence evidence from a trusted local builder operating on a stable
working tree, not a signed attestation, reproducible-build proof or cross-revision
worker protocol. It assumes the local toolchain and module cache are intact.
Process-selector test/tool execution is outside the launcher build's attestation;
its existing source/tool/isolation guards still apply. Explicit ingestion A/B
manifests belong with their actual adapters; neither proposed variant becomes
registered here. Historical artifacts, strict comparisons, imported trust and live
execution authority are unchanged.
