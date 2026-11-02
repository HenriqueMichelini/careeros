import assert from "node:assert/strict"
import { withAccountBrowser } from "./account-browser.fixture.mjs"
for (const pt of [false, true])
  await withAccountBrowser(
    async ({ evaluate, wait, evidence }) => {
      await wait(
        `document.querySelector('[data-pilot-activate]:not(:disabled)')`,
      )
      assert.equal(await evaluate(`pilotScenario.posts.length`), 0)
      await evaluate(
        `pilotScenario.loseAck=true;document.querySelector('[data-pilot-activate]').click()`,
      )
      await wait(`document.querySelector('[data-unsaved]')`)
      assert.equal(await evaluate(`pilotScenario.posts.length`), 1)
      await evaluate(
        `(()=>{const e=document.querySelector('[data-unsaved]');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(e,'UNSAVED_SENTINEL');e.dispatchEvent(new Event('input',{bubbles:true}))})()`,
      )
      await evaluate(
        `pilotScenario.state='retrieval_only';window.dispatchEvent(new Event('focus'))`,
      )
      await wait(
        `document.body.innerText.includes(${JSON.stringify(pt ? "Nova geração por IA" : "New AI generation")})`,
      )
      assert.equal(
        await evaluate(`document.querySelector('[data-unsaved]').value`),
        "UNSAVED_SENTINEL",
      )
      await evaluate(
        `pilotScenario.fail=true;window.dispatchEvent(new Event('focus'))`,
      )
      await wait(
        `document.body.innerText.includes(${JSON.stringify(pt ? "não salvo foi preservado" : "Unsaved work is preserved")})`,
      )
      assert.equal(
        await evaluate(`document.querySelector('[data-unsaved]').value`),
        "UNSAVED_SENTINEL",
      )
      await evaluate(
        `pilotScenario.fail=false;document.querySelector('[data-pilot-refresh]').click()`,
      )
      await wait(
        `document.querySelector('[data-unsaved]').closest('[hidden]')===null`,
      )
      assert.equal(await evaluate(`pilotScenario.posts.length`), 1)
      console.log(
        JSON.stringify({
          locale: pt ? "pt-BR" : "en",
          status: "passed",
          checks: [
            "StrictMode activation",
            "lost response status-only recovery",
            "retrieval-only rights",
            "unsaved work across status failure",
            "no automatic mutation replay",
          ],
          evidence,
        }),
      )
    },
    {
      path: "/tests/pilot-harness.html",
      initialize: pt ? `localStorage.setItem('careeros_locale','pt-BR')` : "",
    },
  )
