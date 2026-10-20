// Offline co-build correspondence smoke. No server, history, credentials or providers.
import assert from "node:assert/strict"
import { spawnSync } from "node:child_process"
import {
  readFileSync,
  writeFileSync,
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  existsSync,
} from "node:fs"
import { tmpdir } from "node:os"
import { join, dirname } from "node:path"
import { fileURLToPath } from "node:url"

const root = fileURLToPath(new URL("..", import.meta.url))
const binary = join(root, "lab/.bin/lab-api")
const receiptPath = join(root, "lab/.bin/lab-build-receipt.json")
const run = (command, args, cwd = root, timeout = 120_000) => {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    env: {
      ...process.env,
      GOCACHE: process.env.GOCACHE ?? "/tmp/careeros-go-cache",
    },
    maxBuffer: 4 * 1024 * 1024,
    timeout,
  })
  assert.ifError(result.error)
  return result
}
const built = run(process.execPath, ["scripts/lab.mjs", "--build-info"])
assert.equal(built.status, 0, built.stderr)
const info = JSON.parse(built.stdout)
assert.equal(info.build.status, "verified")
assert.equal(info.suites.length, 7)
for (const suite of info.suites) {
  const descriptor = suite.variants[0]
  assert.equal(descriptor.buildManifest.hash, info.build.manifest.hash)
  assert.equal(descriptor.buildManifest.variantId, descriptor.id)
  assert.equal(
    descriptor.buildManifest.correspondence,
    "compiled-identity-only",
  )
}
const originalReceipt = readFileSync(receiptPath)
const scratch = mkdtempSync(join(tmpdir(), "careeros-build-smoke-"))
const copiedBinary = join(scratch, "bin/lab-api")
const copiedReceipt = join(scratch, "bin/lab-build-receipt.json")
const alteredBinary = join(scratch, "bin/lab-api-tampered")
const startupHistory = join(scratch, "startup-history")
mkdirSync(dirname(copiedBinary), { recursive: true })
copyFileSync(binary, copiedBinary)
writeFileSync(copiedReceipt, originalReceipt)
try {
  let observed = run(copiedBinary, ["--build-info"])
  assert.equal(observed.status, 0, observed.stderr)
  assert.equal(JSON.parse(observed.stdout).build.status, "verified")
  const receipt = JSON.parse(originalReceipt)
  const conflicts = {
    version: "unsupported-receipt",
    manifestHash: "a".repeat(64),
    executableHash: "a".repeat(64),
    launcherHash: "a".repeat(64),
    runtime: "different-toolchain",
    command: ["go", "run", "unowned"],
  }
  for (const missing of Object.keys(receipt)) {
    const partial = { ...receipt }
    delete partial[missing]
    writeFileSync(copiedReceipt, JSON.stringify(partial))
    observed = run(copiedBinary, ["--build-info"])
    assert.equal(observed.status, 0, `missing ${missing}: ${observed.stderr}`)
    assert.equal(JSON.parse(observed.stdout).build.status, "unavailable")
    for (const [field, value] of Object.entries(conflicts)) {
      if (field === missing) continue
      for (const conflict of field.endsWith("Hash")
        ? [value, "malformed-digest"]
        : [value]) {
        writeFileSync(
          copiedReceipt,
          JSON.stringify({ ...partial, [field]: conflict }),
        )
        observed = run(copiedBinary, ["--build-info"])
        assert.equal(
          observed.status,
          1,
          `missing ${missing}, conflicting ${field}`,
        )
        assert.equal(JSON.parse(observed.stdout).build.status, "mismatch")
        // A known conflict must stop real startup before history or a listener.
        observed = run(copiedBinary, ["--data-dir", startupHistory], root, 5000)
        assert.equal(
          observed.status,
          1,
          `startup: missing ${missing}, conflicting ${field}`,
        )
        assert.equal(observed.stdout, "")
        assert.equal(existsSync(startupHistory), false)
      }
    }
  }
  writeFileSync(copiedReceipt, originalReceipt)
  copyFileSync(copiedBinary, alteredBinary)
  writeFileSync(
    alteredBinary,
    Buffer.concat([readFileSync(alteredBinary), Buffer.from("tampered")]),
  )
  observed = run(alteredBinary, ["--build-info"])
  assert.equal(observed.status, 1)
  assert.equal(JSON.parse(observed.stdout).build.status, "mismatch")
  for (const name of Object.keys(info.build.manifest.sources)) {
    mkdirSync(dirname(join(scratch, name)), { recursive: true })
    copyFileSync(join(root, name), join(scratch, name))
  }
  writeFileSync(
    join(scratch, "internal/fieldvalidation/questions.json"),
    `{"changed":"template"}`,
  )
  observed = run(copiedBinary, ["--build-info"], scratch)
  assert.equal(observed.status, 1)
  assert.equal(JSON.parse(observed.stdout).build.status, "mismatch")
  rmSync(copiedReceipt)
  observed = run(copiedBinary, ["--build-info"])
  assert.equal(observed.status, 0, observed.stderr)
  assert.equal(JSON.parse(observed.stdout).build.status, "unavailable")
} finally {
  rmSync(scratch, { recursive: true, force: true })
}
console.log(
  "PASS: actual co-build/catalog, partial receipt conflicts reject inspection/startup before history, tampered binary, stale template, absent receipt",
)
