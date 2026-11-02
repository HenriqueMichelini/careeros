import { StrictMode, useState } from "react"
import { createRoot } from "react-dom/client"
import PilotAccess from "../src/components/PilotAccess"
import "../src/index.css"
const scenario = {
  state: "invited",
  fail: false,
  loseAck: false,
  posts: [] as string[],
  expires: new Date(Date.now() + 3600000).toISOString(),
}
Object.assign(window, { pilotScenario: scenario })
window.fetch = async (input, init = {}) => {
  if (scenario.fail) throw new Error("synthetic offline")
  if (init.method === "POST") {
    scenario.posts.push(String(init.body))
    scenario.state = "active"
    if (scenario.loseAck) {
      scenario.loseAck = false
      throw new Error("synthetic lost response")
    }
  }
  const active = scenario.state === "active",
    retrieval = scenario.state === "retrieval_only"
  return new Response(
    JSON.stringify({
      accountId: "synthetic-a",
      state: scenario.state,
      canActivate: scenario.state === "invited",
      canDraft: active,
      canStandalone: active,
      canRetrieve: active || retrieval,
      draftAllowance: 5,
      invitationId: "invite",
      ...(active || retrieval
        ? {
            expiresAt: scenario.expires,
            retrieveUntil: new Date(
              Date.parse(scenario.expires) + 30 * 86400000,
            ).toISOString(),
          }
        : {}),
    }),
  )
}
function Workspace() {
  const [value, setValue] = useState("")
  return (
    <input
      data-unsaved
      aria-label="Unsaved work"
      value={value}
      onChange={(e) => setValue(e.target.value)}
    />
  )
}
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <PilotAccess
      accountId="synthetic-a"
      pt={localStorage.getItem("careeros_locale") === "pt-BR"}
    >
      <Workspace />
    </PilotAccess>
  </StrictMode>,
)
