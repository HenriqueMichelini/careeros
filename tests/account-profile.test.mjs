import { workflowModuleURL } from "./workflow-module.mjs"
import test from "node:test"
import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const temp = mkdtempSync(join(tmpdir(), "careeros-account-client-"))
writeFileSync(
  join(temp, "contract.mjs"),
  `export default ${readFileSync(new URL("../internal/profiledocument/contract.json", import.meta.url), "utf8")}`,
)
for (const name of ["profileDocument", "profileStorage", "accountProfile"]) {
  let source = ts.transpileModule(
    readFileSync(new URL(`../src/lib/${name}.ts`, import.meta.url), "utf8"),
    {
      compilerOptions: {
        module: ts.ModuleKind.ESNext,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText.replaceAll('"./workflowRequest"', JSON.stringify(workflowModuleURL))
  for (const dependency of ["profileDocument", "profileStorage"])
    source = source.replaceAll(`"./${dependency}"`, `"./${dependency}.mjs"`)
  source = source.replaceAll(
    '"../../internal/profiledocument/contract.json"',
    '"./contract.mjs"',
  )
  writeFileSync(join(temp, `${name}.mjs`), source)
}
const { AccountProfile, applyAccountRetention } = await import(
  join(temp, "accountProfile.mjs")
)
const {
  migrateProfile,
  emptyProfileView,
  editProfile,
  validateProfileDocument,
} = await import(join(temp, "profileDocument.mjs"))
globalThis.document = { cookie: "__Host-careeros_csrf=synthetic-csrf" }
const reply = (body, status = 200) =>
  new Response(JSON.stringify(body), { status })
const candidate = (doc) =>
  editProfile(doc, {
    type: "field",
    ownerId: doc.id,
    field: "skills",
    value: "Go, ação\0😀",
  })

test("new account opens clean without touching legacy Profile storage, and sends explicit Save + CSRF", async () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage")
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    get() {
      throw new Error("legacy read")
    },
  })
  try {
    const calls = []
    const client = new AccountProfile(
      "account-a",
      () => {},
      async (path, init) => {
        calls.push({ path, init })
        if (init.method !== "PUT")
          return reply({ accountId: "account-a", document: null })
        const body = JSON.parse(init.body)
        return reply({
          accountId: "account-a",
          document: body.document,
          outcome: {
            operationId: body.operationId,
            profileId: body.document.id,
            committedRevision: body.document.revision,
          },
        })
      },
    )
    const doc = await client.open()
    assert.deepEqual(doc.facts.map((f) => f.value).filter(Boolean), [])
    assert.equal(calls.length, 1)
    const saved = await client.save(doc, candidate(doc))
    assert.equal(client.state, "confirmed")
    assert.equal(saved.revision, 2)
    const put = calls[1]
    assert.equal(JSON.parse(put.init.body).expectedRevision, 0)
    assert.equal(put.init.headers["X-CareerOS-Account"], "account-a")
    assert.equal(put.init.headers["X-CSRF-Token"], "synthetic-csrf")
    assert.equal(put.init.credentials, "same-origin")
  } finally {
    if (original) Object.defineProperty(globalThis, "localStorage", original)
    else delete globalThis.localStorage
  }
})

test("commit response loss recovers historical outcome while adopting current newer Profile", async () => {
  const initial = migrateProfile(emptyProfileView(), "p")
  let pending,
    puts = 0
  const client = new AccountProfile(
    "a",
    () => {},
    async (path, init) => {
      if (init.method === "PUT") {
        puts++
        pending = JSON.parse(init.body)
        throw new Error("committed response lost")
      }
      if (path === "/api/profile/current")
        return reply({ accountId: "a", document: initial })
      const newer = editProfile(pending.document, {
        type: "field",
        ownerId: "p",
        field: "tools",
        value: "Current newer edit",
      })
      return reply({
        accountId: "a",
        document: newer,
        outcome: {
          operationId: pending.operationId,
          profileId: "p",
          committedRevision: 2,
        },
      })
    },
  )
  const doc = await client.open(),
    saved = await client.save(doc, candidate(doc))
  assert.equal(puts, 1)
  assert.equal(saved.revision, 3)
  assert.equal(client.historicalRevision, 2)
  assert.equal(client.state, "confirmed")
})

test("unavailable storage stays uncertain, retains exact operation, and explicit retry reuses it", async () => {
  const initial = migrateProfile(emptyProfileView(), "p")
  const puts = []
  let available = false
  const client = new AccountProfile(
    "a",
    () => {},
    async (path, init) => {
      if (init.method === "PUT") {
        puts.push(init.body)
        if (!available) throw new Error("network lost")
        const p = JSON.parse(init.body)
        return reply({
          accountId: "a",
          document: p.document,
          outcome: {
            operationId: p.operationId,
            profileId: "p",
            committedRevision: 2,
          },
        })
      }
      if (path === "/api/profile/current")
        return reply({ accountId: "a", document: initial })
      return reply({ error: "storage_unavailable" }, 503)
    },
  )
  const doc = await client.open()
  await assert.rejects(client.save(doc, candidate(doc)), { code: "uncertain" })
  assert.equal(client.state, "uncertain")
  assert.equal(client.recovery().document.revision, 2)
  await assert.rejects(client.save(doc, candidate(doc)), { code: "uncertain" })
  available = true
  assert.equal((await client.retry()).revision, 2)
  assert.equal(puts.length, 2)
  assert.equal(puts[0], puts[1])
})

test("new stale saves fail truthfully without browser fallback", async () => {
  const doc = migrateProfile(emptyProfileView(), "p")
  const client = new AccountProfile(
    "a",
    () => {},
    async (path, init) =>
      init.method === "PUT"
        ? reply({ error: "stale" }, 409)
        : reply({ accountId: "a", document: doc }),
  )
  await client.open()
  await assert.rejects(client.save(doc, candidate(doc)), { code: "stale" })
  assert.equal(client.state, "failed")
  assert.equal(client.recovery().document.revision, 2)
})

test("account-switch disposal rejects late A replies and cannot contaminate account B", async () => {
  let release
  const a = new AccountProfile(
    "a",
    () => {},
    () =>
      new Promise((resolve) => {
        release = resolve
      }),
  )
  const late = a.open()
  a.dispose()
  const doc = migrateProfile({ skills: "A sentinel" }, "p")
  release(reply({ accountId: "a", document: doc }))
  await assert.rejects(late, { code: "account_changed" })
  const b = new AccountProfile(
    "b",
    () => {},
    async () => reply({ accountId: "b", document: null }),
  )
  assert.ok(!(await b.open()).facts.some((f) => f.value === "A sentinel"))
  const confused = new AccountProfile(
    "a",
    () => {},
    async () => reply({ accountId: "b", document: doc }),
  )
  await assert.rejects(confused.open(), { code: "account_changed" })
})

function recoveryStorage() {
  const data = new Map()
  return {
    data,
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => data.set(key, value),
    removeItem: (key) => data.delete(key),
  }
}

test("tab recovery survives reconstruction, stays account-scoped and reuses the exact operation", async () => {
  const storage = recoveryStorage()
  const doc = migrateProfile(emptyProfileView(), "pending-profile")
  const puts = []
  let available = false
  const transport = async (path, init) => {
    if (init.method === "PUT") {
      puts.push(init.body)
      if (!available) throw new Error("lost acknowledgment")
      const p = JSON.parse(init.body)
      return reply({
        accountId: "a",
        document: p.document,
        outcome: {
          operationId: p.operationId,
          profileId: p.document.id,
          committedRevision: p.document.revision,
        },
      })
    }
    if (path === "/api/profile/current")
      return reply({ accountId: "a", document: doc })
    return reply({ error: "not_found" }, 404)
  }
  const original = new AccountProfile("a", () => {}, transport, storage)
  await original.open()
  await assert.rejects(original.save(doc, candidate(doc)), {
    code: "uncertain",
  })
  const frozen = puts[0],
    key = "careeros_pending_profile_v1:a"
  assert.equal(
    JSON.stringify(JSON.parse(storage.data.get(key)).pendingSave),
    frozen,
  )
  assert.ok(!storage.data.get(key).includes("synthetic-csrf"))
  original.dispose()
  const recovered = new AccountProfile("a", () => {}, transport, storage)
  assert.equal(recovered.state, "uncertain")
  assert.equal(
    (await recovered.open()).revision,
    1,
    "read authority remains the current server document",
  )
  assert.equal(puts.length, 1, "construct/read must not upload recovery")
  assert.equal(JSON.stringify(recovered.recovery()), frozen)
  const b = new AccountProfile(
    "b",
    () => {},
    async () => reply({ accountId: "b", document: null }),
    storage,
  )
  assert.equal(b.recovery(), null)
  assert.ok(!(await b.open()).facts.some((f) => f.value === "Go, ação\0😀"))
  await assert.rejects(recovered.refresh(), { code: "not_found" })
  assert.equal(
    JSON.stringify(recovered.recovery()),
    frozen,
    "missing outcome must not discard pending work",
  )
  available = true
  await recovered.retry()
  assert.equal(puts[1], frozen)
  assert.equal(storage.data.has(key), false)
})

test("late replies from a disposed lease cannot clear a newer recovery after same-account resume", async () => {
  const storage = recoveryStorage(),
    doc = migrateProfile(emptyProfileView(), "lease-profile")
  const requests = []
  const client = new AccountProfile(
    "a",
    () => {},
    async (path, init) => {
      if (init.method !== "PUT") return reply({ accountId: "a", document: doc })
      return new Promise((resolve) =>
        requests.push({ resolve, body: JSON.parse(init.body) }),
      )
    },
    storage,
  )
  await client.open()
  const former = client.save(doc, candidate(doc)).then(
    () => null,
    (error) => error,
  )
  client.dispose()
  client.resume()
  client.discardRecovery()
  const nextDoc = editProfile(doc, {
    type: "field",
    ownerId: doc.id,
    field: "skills",
    value: "new pending wording",
  })
  const next = client.save(doc, nextDoc)
  const currentID = client.recovery().operationId
  const complete = (request) =>
    request.resolve(
      reply({
        accountId: "a",
        document: request.body.document,
        outcome: {
          operationId: request.body.operationId,
          profileId: doc.id,
          committedRevision: 2,
        },
      }),
    )
  complete(requests[0])
  assert.equal((await former).code, "account_changed")
  assert.equal(client.recovery().operationId, currentID)
  assert.equal(
    JSON.parse(storage.data.get("careeros_pending_profile_v1:a")).pendingSave
      .operationId,
    currentID,
  )
  complete(requests[1])
  await next
  assert.equal(client.recovery(), null)
})

test("checkpoint failure retains memory recovery, server outage never supplies browser authority, discard is deliberate", async () => {
  const doc = migrateProfile(emptyProfileView(), "storage-profile")
  let failRead = false,
    failRemove = false
  const storage = recoveryStorage()
  const originalSet = storage.setItem,
    originalRemove = storage.removeItem
  storage.setItem = () => {
    throw new Error("quota")
  }
  storage.removeItem = (key) => {
    if (failRemove) throw new Error("blocked")
    originalRemove(key)
  }
  const client = new AccountProfile(
    "a",
    () => {},
    async (path, init) => {
      if (init.method === "PUT" || path.includes("operations") || failRead)
        return reply({ error: "storage_unavailable" }, 503)
      return reply({ accountId: "a", document: doc })
    },
    storage,
  )
  await client.open()
  await assert.rejects(client.save(doc, candidate(doc)), { code: "uncertain" })
  const frozen = client.recovery()
  assert.equal(client.checkpointStored, false)
  client.dispose()
  client.resume()
  failRead = true
  await assert.rejects(client.open(), { code: "storage" })
  assert.deepEqual(client.recovery(), frozen)
  failRemove = true
  assert.throws(() => client.discardRecovery(), { code: "storage" })
  assert.deepEqual(client.recovery(), frozen)
  failRemove = false
  storage.setItem = originalSet
  client.discardRecovery()
  assert.equal(client.recovery(), null)
})

test("account retention removes a deleted fact's shared excerpt and invalidates retained support", () => {
  const before = JSON.parse(
    readFileSync(
      new URL("../internal/profiledocument/fixtures.json", import.meta.url),
      "utf8",
    ),
  )[0].document
  const retained = structuredClone(before.facts[0])
  retained.id = "retained"
  retained.field = "tools"
  retained.value = "Go"
  retained.normalization.canonical = null
  before.facts.push(retained)
  const shared = structuredClone(before.links[0])
  shared.id = "shared"
  shared.from.id = "retained"
  before.links.push(shared)
  const next = structuredClone(before)
  next.facts = next.facts.filter((f) => f.id === "retained")
  for (const l of next.links) if (l.from.id === "skill") l.state = "invalidated"
  const minimized = applyAccountRetention(before, next)
  assert.equal(minimized.evidence.length, 0)
  assert.equal(minimized.facts[0].support, "invalidated")
  assert.ok(
    minimized.links.every(
      (l) => l.kind !== "supports" || l.state === "invalidated",
    ),
  )
  assert.equal(before.evidence.length, 1)
  assert.ok(validateProfileDocument(minimized))
})
