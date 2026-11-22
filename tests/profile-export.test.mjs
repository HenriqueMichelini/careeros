import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, mkdtempSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
const dir = mkdtempSync(join(tmpdir(), "profile-export-client-"))
const source = readFileSync(
  new URL("../src/lib/profileExport.ts", import.meta.url),
  "utf8",
)
writeFileSync(
  join(dir, "client.mjs"),
  ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText,
)
const { requestProfileExport, ProfileExportError } = await import(
  join(dir, "client.mjs")
)
const profile = readFileSync(
  new URL(
    "../internal/accountapi/testdata/profile-export-v1.json",
    import.meta.url,
  ),
  "utf8",
)
const wire =
  '{"format":"careeros-profile-export","version":1,"profile":' + profile + "}"
test("download preserves exact response bytes and requests only authenticated retained export", async () => {
  let seen
  const result = await requestProfileExport(
    "owner",
    new AbortController().signal,
    async (path, init) => {
      seen = { path, init }
      return new Response(wire, {
        headers: { "Content-Type": "application/json" },
      })
    },
  )
  assert.equal(await result.text(), wire)
  assert.equal(seen.path, "/api/profile/export")
  assert.equal(seen.init.credentials, "same-origin")
  assert.equal(seen.init.cache, "no-store")
  assert.equal(seen.init.headers["X-CareerOS-Account"], "owner")
  assert.equal(seen.init.method, "GET")
  assert.equal(seen.init.body, undefined)
})
test("errors expose fixed metadata codes and unknown versions never download", async () => {
  for (const [status, code] of [
    [401, "authentication"],
    [403, "access"],
    [404, "missing"],
    [409, "account_changed"],
    [503, "unavailable"],
  ]) {
    await assert.rejects(
      requestProfileExport(
        "owner",
        new AbortController().signal,
        async () => new Response("PRIVATE_ERROR_CONTENT", { status }),
      ),
      (e) =>
        e instanceof ProfileExportError &&
        e.code === code &&
        !String(e).includes("PRIVATE"),
    )
  }
  for (const raw of [
    wire.replace('"version":1', '"version":2'),
    wire.replace('"version": 2', '"version": 99'),
    "{}",
  ]) {
    await assert.rejects(
      requestProfileExport(
        "owner",
        new AbortController().signal,
        async () =>
          new Response(raw, {
            headers: { "Content-Type": "application/json" },
          }),
      ),
      (e) => e.code === "format",
    )
  }
})
