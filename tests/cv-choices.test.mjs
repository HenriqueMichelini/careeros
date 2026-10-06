import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync, mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const source = readFileSync(
  new URL("../src/lib/cv.ts", import.meta.url),
  "utf8",
)
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText
const path = join(mkdtempSync(join(tmpdir(), "careeros-cv-")), "cv.mjs")
writeFileSync(path, compiled)
const { parseCvChoices, entryKey, bulletKey, professionalLinkHref } = await import(path)

test("CV choices round trip independently and legacy or damaged data loads safely", () => {
  const choices = {
    hiddenSections: ["skills"],
    hiddenEntries: [entryKey("experience", "e1")],
    hiddenBullets: [
      bulletKey("experience", "e1", "achievements", 0, "Built search"),
    ],
    summary: "CV summary",
    bulletWording: { example: "Clearer bullet" },
  }
  assert.deepEqual(parseCvChoices(JSON.stringify(choices)), choices)
  assert.deepEqual(parseCvChoices(null), parseCvChoices("{"))
  assert.deepEqual(
    parseCvChoices(
      JSON.stringify({
        hiddenSections: ["not-a-section"],
        hiddenEntries: [42],
        bulletWording: { one: 42 },
        summary: 17,
      }),
    ),
    {
      hiddenSections: [],
      hiddenEntries: [],
      hiddenBullets: [],
      summary: null,
      bulletWording: {},
    },
  )
})

test("a changed Profile bullet cannot inherit an old CV choice", () => {
  assert.notEqual(
    bulletKey("projects", "p1", "highlights", 0, "Original fact"),
    bulletKey("projects", "p1", "highlights", 0, "Updated fact"),
  )
})

test("long CV wording is restored without truncation", () => {
  const longText = "Detailed CV wording. ".repeat(250)
  const choices = {
    hiddenSections: [],
    hiddenEntries: [],
    hiddenBullets: [],
    summary: longText,
    bulletWording: { example: longText },
  }
  assert.deepEqual(parseCvChoices(JSON.stringify(choices)), choices)
})

test("professional links become safe PDF destinations", () => {
  assert.equal(professionalLinkHref("linkedin.com/in/example"), "https://linkedin.com/in/example")
  assert.equal(professionalLinkHref("https://example.com/work"), "https://example.com/work")
  assert.equal(professionalLinkHref("javascript:alert(1)"), null)
  assert.equal(professionalLinkHref("https://user:pass@example.com"), null)
})
