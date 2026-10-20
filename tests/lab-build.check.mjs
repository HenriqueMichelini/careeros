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
} from "node:fs"
import { tmpdir } from "node:os"
import { join, dirname } from "node:path"
import { fileURLToPath } from "node:url"

const root = fileURLToPath(new URL("..", import.meta.url))
const binary = join(root, "lab/.bin/lab-api")
const receiptPath = join(root, "lab/.bin/lab-build-receipt.json")
const run = (command, args, cwd = root) => {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    env: {
      ...process.env,
      GOCACHE: process.env.GOCACHE ?? "/tmp/careeros-go-cache",
    },
    maxBuffer: 4 * 1024 * 1024,
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
const alteredBinary = join(root, "lab/.bin/lab-api-tampered")
try {
  const receipt = JSON.parse(originalReceipt)
  receipt.executableHash = "a".repeat(64)
  writeFileSync(receiptPath, JSON.stringify(receipt))
  let observed = run(binary, ["--build-info"])
  assert.equal(observed.status, 1)
  assert.equal(JSON.parse(observed.stdout).build.status, "mismatch")
  writeFileSync(receiptPath, originalReceipt)
  copyFileSync(binary, alteredBinary)
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
  observed = run(binary, ["--build-info"], scratch)
  assert.equal(observed.status, 1)
  assert.equal(JSON.parse(observed.stdout).build.status, "mismatch")
  rmSync(receiptPath)
  observed = run(binary, ["--build-info"])
  assert.equal(observed.status, 0, observed.stderr)
  assert.equal(JSON.parse(observed.stdout).build.status, "unavailable")
} finally {
  writeFileSync(receiptPath, originalReceipt)
  rmSync(alteredBinary, { force: true })
  rmSync(scratch, { recursive: true, force: true })
}
console.log(
  "PASS: actual co-built manifest/catalog, tampered receipt/binary, stale template, missing receipt",
)
