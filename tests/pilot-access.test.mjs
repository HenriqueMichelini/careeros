import test from "node:test"
import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
import { workflowModuleURL } from "./workflow-module.mjs"
const temp = mkdtempSync(join(tmpdir(), "careeros-pilot-client-"))
writeFileSync(
  join(temp, "pilot.mjs"),
  ts
    .transpileModule(
      readFileSync(
        new URL("../src/lib/pilotAccess.ts", import.meta.url),
        "utf8",
      ),
      {
        compilerOptions: {
          module: ts.ModuleKind.ESNext,
          target: ts.ScriptTarget.ES2022,
        },
      },
    )
    .outputText.replaceAll(
      '"./workflowRequest"',
      JSON.stringify(workflowModuleURL),
    ),
)
const { PilotAccessClient } = await import(join(temp, "pilot.mjs"))
globalThis.document = { cookie: "__Host-careeros_csrf=synthetic-csrf" }
const invited = {
  accountId: "a",
  state: "invited",
  canActivate: true,
  canDraft: false,
  canStandalone: false,
  canRetrieve: false,
  draftAllowance: 5,
  invitationId: "invite",
}
const active = {
  ...invited,
  state: "active",
  canActivate: false,
  canDraft: true,
  canStandalone: true,
  canRetrieve: true,
  expiresAt: "2030-01-15T00:00:00Z",
  retrieveUntil: "2030-02-14T00:00:00Z",
}
const reply = (value, status = 200) =>
  new Response(JSON.stringify(value), { status })
test("lost activation response reads status and never resubmits on refresh or reauthentication", async () => {
  const calls = []
  let activated = false
  const client = new PilotAccessClient("a", async (path, init) => {
    calls.push({ path, init })
    if (init.method === "POST") {
      activated = true
      throw new Error("lost response")
    }
    return reply(activated ? active : invited)
  })
  await client.refresh()
  assert.equal(calls.length, 1)
  assert.equal(client.status.canActivate, true)
  await client.activate()
  assert.equal(client.status.state, "active")
  assert.equal(calls.filter((c) => c.init.method === "POST").length, 1)
  await client.refresh()
  assert.equal(calls.filter((c) => c.init.method === "POST").length, 1)
  const post = calls.find((c) => c.init.method === "POST")
  assert.equal(post.init.headers["X-CareerOS-Account"], "a")
  assert.equal(post.init.headers["X-CSRF-Token"], "synthetic-csrf")
  assert.equal(JSON.parse(post.init.body).confirm, true)
})
test("uncertain activation retains its exact request for explicit retry, refresh is read-only", async () => {
  const bodies = []
  let online = false
  const client = new PilotAccessClient("a", async (path, init) => {
    if (init.method === "POST") {
      bodies.push(init.body)
      if (!online) throw new Error("offline")
      return reply(active)
    }
    if (bodies.length && !online) throw new Error("offline")
    return reply(invited)
  })
  await client.refresh()
  await assert.rejects(client.activate())
  await assert.rejects(client.refresh())
  assert.equal(bodies.length, 1)
  online = true
  await client.refresh()
  assert.equal(bodies.length, 1)
  await client.activate()
  assert.equal(bodies[0], bodies[1])
})
test("late account response is fenced and does not expose another account", async () => {
  let finish
  const client = new PilotAccessClient(
    "a",
    () => new Promise((resolve) => (finish = resolve)),
  )
  const pending = client.refresh()
  client.dispose()
  finish(reply(active))
  await assert.rejects(pending)
  const changed = new PilotAccessClient("a", async () =>
    reply({ ...active, accountId: "b" }),
  )
  await assert.rejects(changed.refresh())
  assert.equal(changed.status, null)
})
