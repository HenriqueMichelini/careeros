// Assemble immutable owner receipts. This command never runs providers or tests.
// Usage: node scripts/cv-lab-evidence.mjs OWNER.json BROWSER.json CLIENT.tap BACKEND.jsonl OUTPUT.json
import { readFileSync, writeFileSync } from "node:fs"
import { createHash } from "node:crypto"
const [ownerPath, browserPath, clientPath, backendPath, outputPath, ...extra] =
  process.argv.slice(2)
if (
  !ownerPath ||
  !browserPath ||
  !clientPath ||
  !backendPath ||
  !outputPath ||
  extra.length
)
  throw new Error("Five explicit receipt/output paths required")
const ownerBytes = readFileSync(ownerPath)
const browserBytes = readFileSync(browserPath)
if (ownerBytes.length + browserBytes.length > 8 << 20)
  throw new Error("Receipt bounds exceeded")
const owner = JSON.parse(ownerBytes)
const browser = JSON.parse(browserBytes)
if (
  owner.version !== "cv-lab-evidence-v1" ||
  browser.version !== "cv-lab-browser-v1"
)
  throw new Error("Unsupported owner receipt")
const fixture = JSON.parse(Buffer.from(owner.fixtureBytes, "base64"))
if (owner.observations.length !== fixture.cases.length)
  throw new Error("Incomplete owner observations")
const hash = (raw) => createHash("sha256").update(raw).digest("hex")
owner.ownerReceiptHash = hash(ownerBytes)
const checks = [
  {
    id: "owner-client-contracts",
    language: "en+pt-BR",
    path: clientPath,
    format: "tap",
    sources: [
      "tests/cv-generation.test.mjs",
      "src/lib/cvGeneration.ts",
      "tests/cv-support-fixtures.mjs",
    ],
  },
  {
    id: "owner-backend-contracts",
    language: "en+pt-BR",
    path: backendPath,
    format: "go-test-json",
    sources: [
      "backend/cv-generation/main.go",
      "backend/cv-generation/main_test.go",
    ],
  },
].map(({ path, sources, ...check }) => {
  const raw = readFileSync(path)
  if (raw.length > 1 << 20) throw new Error("Owner check exceeds bounds")
  return {
    ...check,
    originalBytes: raw.toString("base64"),
    hash: hash(raw),
    sourceHashes: Object.fromEntries(
      sources.map((path) => [
        path,
        hash(readFileSync(new URL(`../${path}`, import.meta.url))),
      ]),
    ),
  }
})
owner.checks = checks
owner.browserBytes = browserBytes.toString("base64")
owner.browserHash = hash(browserBytes)
owner.cases = [
  ...fixture.cases,
  ...browser.observations.map((row) => ({
    id: row.id,
    task: "cv_generation",
    language: row.language,
    split: "development",
    sourceId: "cv-owner-browser",
    tags: [row.kind],
    request: row.request ?? { acceptedSources: row.snapshot?.sources ?? [] },
    gold: [],
  })),
]
owner.cases.push(
  ...checks.map((check) => ({
    id: check.id,
    task: "cv_generation",
    language: check.language,
    split: "development",
    sourceId: "cv-owner-contract-tests",
    tags: [check.format],
    request: { sourceHashes: check.sourceHashes },
    gold: [],
  })),
)
writeFileSync(outputPath, JSON.stringify(owner, null, 2) + "\n", { flag: "wx" })
console.log(
  `${owner.cases.length} retained cases; sha256 ${hash(readFileSync(outputPath))}`,
)
