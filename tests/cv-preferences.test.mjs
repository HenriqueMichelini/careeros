import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync, mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
const path = join(mkdtempSync(join(tmpdir(), "cv-preferences-")), "prefs.mjs")
const source = readFileSync(
  new URL("../src/lib/cvPreferences.ts", import.meta.url),
  "utf8",
).replace(
  'from "react"',
  `from ${JSON.stringify(import.meta.resolve("react"))}`,
)
writeFileSync(
  path,
  ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText,
)
const { parseCvDensity, parseCvFontSize } = await import(path)
test("legacy and invalid density preferences default to balanced independently of font size", () => {
  for (const raw of [
    null,
    "{",
    "null",
    "{}",
    '{"fontSize":15}',
    '{"density":"invalid"}',
    '{"density":42}',
  ])
    assert.equal(parseCvDensity(raw), "balanced")
  for (const density of ["compact", "balanced", "detailed"]) {
    const raw = JSON.stringify({ density, fontSize: 15 })
    assert.equal(parseCvDensity(raw), density)
    assert.equal(parseCvFontSize(raw), 15)
  }
  assert.equal(parseCvFontSize('{"density":"detailed"}'), 14)
})
