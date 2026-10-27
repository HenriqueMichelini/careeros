// Browser-only synthetic transport for the actual AccountGate + StoreProvider.
// PostgreSQL behavior is independently exercised by Go integration tests.
import { useState } from "react"
import { createRoot } from "react-dom/client"
import AccountGate from "../src/components/AccountGate"
import { useStore } from "../src/lib/store"
import type { ProfileDocument } from "../src/lib/profileDocument"
import "../src/index.css"

type Scenario = {
  account: string
  expires: string
  fail: boolean
  loseAck: boolean
  defer: string
  release: (() => void) | null
  reads: string[]
  puts: string[]
  logoutCalls: number
  autoExpire: boolean
  documents: Record<string, ProfileDocument>
  operations: Record<string, unknown>
}
const scenario: Scenario = {
  account: "a",
  expires: new Date(Date.now() + 3600_000).toISOString(),
  fail: false,
  loseAck: false,
  defer: "",
  release: null,
  reads: [],
  puts: [],
  logoutCalls: 0,
  autoExpire: false,
  documents: {},
  operations: {},
  ...(window as Window & { accountServerFixture?: Partial<Scenario> })
    .accountServerFixture,
}
Object.assign(window, { accountScenario: scenario })
const get = Storage.prototype.getItem
Storage.prototype.getItem = function (key) {
  if (key === "careeros_repo" || key === "careeros_profile_v2")
    scenario.reads.push(key)
  return get.call(this, key)
}
const response = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status })
window.fetch = async (input, init = {}) => {
  const path = String(input)
  const owner = scenario.account
  if (path === "/api/auth/session") {
    const authenticated =
      !!owner &&
      !(scenario.autoExpire && Date.now() >= Date.parse(scenario.expires))
    return response({
      authenticated,
      account: authenticated ? { id: owner } : undefined,
      expiresAt: scenario.expires,
    })
  }
  if (path === "/api/auth/logout") {
    scenario.logoutCalls++
    scenario.account = ""
    return new Response(null, { status: 204 })
  }
  const headers = new Headers(init.headers)
  if (headers.get("X-CareerOS-Account") !== owner)
    return response({ error: "account_changed" }, 409)
  if (init.method === "PUT") {
    scenario.puts.push(String(init.body))
    if (scenario.fail) return response({ error: "storage_unavailable" }, 503)
    const command = JSON.parse(String(init.body))
    scenario.documents[owner] = command.document
    scenario.operations[owner + ":" + command.operationId] = {
      operationId: command.operationId,
      profileId: command.document.id,
      committedRevision: command.document.revision,
    }
    if (scenario.loseAck) {
      scenario.loseAck = false
      throw new Error("synthetic committed response loss")
    }
    return response({
      accountId: owner,
      document: scenario.documents[owner],
      outcome: scenario.operations[owner + ":" + command.operationId],
    })
  }
  if (scenario.fail) return response({ error: "storage_unavailable" }, 503)
  if (path.startsWith("/api/profile/operations/")) {
    const outcome = scenario.operations[owner + ":" + path.split("/").at(-1)]
    return outcome
      ? response({
          accountId: owner,
          document: scenario.documents[owner],
          outcome,
        })
      : response({ error: "not_found" }, 404)
  }
  const body = { accountId: owner, document: scenario.documents[owner] ?? null }
  if (scenario.defer === owner) {
    scenario.defer = ""
    await new Promise<void>((resolve) => {
      scenario.release = resolve
    })
  }
  return response(body)
}
function Workspace() {
  const {
    state,
    accountId,
    profileDocument,
    editCanonicalProfile,
    profileSaving,
  } = useStore()
  const [value, setValue] = useState("")
  return (
    <section data-workspace-account={accountId}>
      <p data-profile-name>{state.repository.fullName}</p>
      <input
        aria-label="Synthetic name"
        value={value}
        onChange={(e) => setValue(e.target.value)}
      />
      <button
        data-save
        disabled={profileSaving || !profileDocument}
        onClick={() =>
          profileDocument &&
          void editCanonicalProfile({
            type: "field",
            ownerId: profileDocument.id,
            field: "fullName",
            value,
          })
        }
      >
        Save
      </button>
    </section>
  )
}
createRoot(document.getElementById("root")!).render(
  <AccountGate>
    <Workspace />
  </AccountGate>,
)
