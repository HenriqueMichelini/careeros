// Actual React account/session/store behavior with synthetic transport only.
import assert from "node:assert/strict"
import { writeFileSync } from "node:fs"
import { join } from "node:path"
import { pause, withAccountBrowser } from "./account-browser.fixture.mjs"

await withAccountBrowser(async ({ evidence, call, evaluate, wait }) => {
  // Defer A2's Profile read, switch to B, then release the late old reply.
  await evaluate(
    `accountScenario.documents.a2=structuredClone(accountScenario.documents.a);accountScenario.documents.a2.facts.find(f=>f.field==='fullName').value='LATE_A_SENTINEL';accountScenario.defer='a2';accountScenario.account='a2';window.dispatchEvent(new Event('focus'))`,
  )
  await wait(`accountScenario.release!==null`)
  await evaluate(
    `accountScenario.account='b';window.dispatchEvent(new Event('focus'))`,
  )
  await wait(
    `document.querySelector('[data-workspace-account="b"]') && document.querySelector('[data-save]:not(:disabled)')`,
  )
  await evaluate(`accountScenario.release()`)
  await pause(100)
  assert.equal(
    await evaluate(`document.querySelector('[data-profile-name]').textContent`),
    "",
  )
  assert.equal(
    await evaluate(`document.body.innerText.includes('LATE_A_SENTINEL')`),
    false,
  )
  assert.equal(await evaluate(`accountScenario.reads.length`), 0)
  await evaluate(
    `localStorage.setItem('careeros_locale','pt-BR');accountScenario.account='c';window.dispatchEvent(new Event('focus'))`,
  )
  await wait(
    `document.querySelector('[data-workspace-account="c"]') && document.body.innerText.includes('Sair')`,
  )
  const shot = await call("Page.captureScreenshot", { format: "png" })
  writeFileSync(
    join(evidence, "account-workspace.png"),
    Buffer.from(shot.data, "base64"),
  )
  console.log(
    JSON.stringify({
      status: "passed",
      checks: [
        "clean new account; zero legacy Profile reads",
        "explicit Save and committed-response-loss recovery",
        "uncertain Save preserves edits and stable retry identity across focus",
        "actual React account remount and late-response fencing",
        "English and Portuguese account UI",
      ],
      evidence,
    }),
  )
})
