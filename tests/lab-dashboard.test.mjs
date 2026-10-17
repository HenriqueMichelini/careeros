import test from "node:test"
import assert from "node:assert/strict"
import { execFileSync } from "node:child_process"
import { readFileSync } from "node:fs"

test("standalone Lab build bundles local assets without production analytics or Profile storage access", () => {
  execFileSync("npm", ["run", "build:lab"], { stdio: "pipe" })
  const script = readFileSync("lab/web/lab.js", "utf8")
  const style = readFileSync("lab/web/lab.css", "utf8")
  assert.ok(script.length > 1000 && style.length > 100)
  for (const forbidden of [
    "figma.com",
    "careeros_profile_v2",
    "careeros_apikey",
    "localStorage",
    "sessionStorage",
    "fonts.googleapis.com",
  ])
    assert.equal(script.includes(forbidden), false, forbidden)
  assert.equal(/@import\s+url\(\s*https?:/.test(style), false)
})
