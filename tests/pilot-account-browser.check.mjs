// Combined actual AccountGate + PilotAccess + StoreProvider behavior.
// The transport is synthetic; PostgreSQL protocol evidence stays independent.
import assert from "node:assert/strict"
import { withAccountBrowser } from "./account-browser.fixture.mjs"

for (const pt of [false, true]) {
  await withAccountBrowser(
    async ({ evaluate, wait, evidence }) => {
      await wait(
        `document.querySelector('[data-pilot-activate]:not(:disabled)')`,
      )
      assert.equal(await evaluate(`accountScenario.pilotPosts.length`), 0)
      assert.equal(await evaluate(`accountScenario.puts.length`), 0)
      assert.equal(await evaluate(`accountScenario.mutations.length`), 0)
      assert.equal(
        await evaluate(`document.querySelector('[data-workspace-account]')`),
        null,
      )
      await evaluate(
        `accountScenario.pilotLoseAck=true;document.querySelector('[data-pilot-activate]').click()`,
      )
      await wait(
        `document.querySelector('[data-workspace-account="a"]') && document.querySelector('[data-save]:not(:disabled)')`,
      )
      const activation = await evaluate(`accountScenario.pilotPosts[0]`)
      const expiresAt = await evaluate(
        `document.querySelector("time").dateTime`,
      )
      assert.equal(await evaluate(`accountScenario.pilotPosts.length`), 1)
      assert.equal(JSON.parse(activation).accountId, "a")
      const sentinel = pt
        ? "PILOTO_PT_TRABALHO_NAO_SALVO"
        : "PILOT_EN_UNSAVED_WORK"
      await evaluate(
        `(()=>{const input=document.querySelector('input');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,${JSON.stringify(sentinel)});input.dispatchEvent(new Event('input',{bubbles:true}))})()`,
      )
      await evaluate(
        `accountScenario.fail=true;document.querySelector('[data-save]').click()`,
      )
      await wait(
        `document.querySelector('[data-account-save-state="uncertain"]')`,
      )
      const save = await evaluate(`accountScenario.puts[0]`)
      assert.equal(
        await evaluate(
          `document.querySelector('[data-profile-name]').textContent`,
        ),
        sentinel,
      )
      // Exercise the real AccountGate expiry timer, not only a focus event.
      await evaluate(
        `accountScenario.autoExpire=true;accountScenario.expires=new Date(Date.now()+150).toISOString();window.dispatchEvent(new Event('focus'))`,
      )
      await wait(`document.querySelector('a[href="/api/auth/google/start"]')`)
      assert.equal(
        await evaluate(`document.querySelector('[data-workspace-account]')`),
        null,
      )
      assert.equal(
        await evaluate(
          `document.body.innerText.includes(${JSON.stringify(sentinel)})`,
        ),
        false,
      )
      assert.equal(
        await evaluate(
          `document.querySelector('a[href="/api/auth/google/start"]').target`,
        ),
        "_blank",
      )
      assert.equal(await evaluate(`accountScenario.pilotPosts.length`), 1)
      assert.equal(await evaluate(`accountScenario.puts.length`), 1)
      assert.equal(await evaluate(`accountScenario.mutations.length`), 2)
      // Reauthenticate the same account: recover its active grant and pending
      // Profile wording with status/owner reads only, without another activation.
      await evaluate(
        `accountScenario.autoExpire=false;accountScenario.fail=false;accountScenario.expires=new Date(Date.now()+3600000).toISOString();window.dispatchEvent(new Event('focus'))`,
      )
      await wait(
        `document.querySelector('[data-workspace-account="a"]') && document.querySelector('[data-account-save-state="uncertain"]') && document.querySelector('[data-workspace-account="a"]').closest('[hidden]')===null`,
      )
      assert.equal(
        await evaluate(
          `document.querySelector('[data-profile-name]').textContent`,
        ),
        sentinel,
      )
      assert.equal(
        await evaluate(
          `document.body.innerText.includes(${JSON.stringify(pt ? "acesso piloto de 14 dias está ativo" : "14-day pilot access is active")})`,
        ),
        true,
      )
      assert.equal(await evaluate(`accountScenario.pilotPosts.length`), 1)
      assert.equal(await evaluate(`accountScenario.puts.length`), 1)
      assert.equal(await evaluate(`accountScenario.mutations.length`), 2)
      assert.equal(await evaluate(`accountScenario.pilotPosts[0]`), activation)
      assert.equal(
        await evaluate(`document.querySelector("time").dateTime`),
        expiresAt,
      )
      // A different authenticated account is still unactivated and cannot see
      // A's workspace or recovery. Focus and sign-in never activate B.
      await evaluate(
        `accountScenario.account='b';window.dispatchEvent(new Event('focus'))`,
      )
      await wait(
        `document.querySelector('[data-pilot-activate]:not(:disabled)')`,
      )
      assert.equal(
        await evaluate(`document.querySelector('[data-workspace-account]')`),
        null,
      )
      assert.equal(
        await evaluate(
          `document.body.innerText.includes(${JSON.stringify(sentinel)})`,
        ),
        false,
      )
      assert.equal(await evaluate(`accountScenario.pilotPosts.length`), 1)
      assert.equal(await evaluate(`accountScenario.puts.length`), 1)
      assert.equal(await evaluate(`accountScenario.mutations.length`), 2)
      // Returning to A restores only A's pending Save. Only an explicit Retry
      // submits the original bytes/base/operation identity exactly once.
      await evaluate(
        `accountScenario.account='a';window.dispatchEvent(new Event('focus'))`,
      )
      await wait(
        `document.querySelector('[data-workspace-account="a"]') && document.querySelector('[data-account-save-state="uncertain"]')`,
      )
      assert.equal(
        await evaluate(
          `document.querySelector('[data-profile-name]').textContent`,
        ),
        sentinel,
      )
      assert.equal(await evaluate(`accountScenario.pilotPosts.length`), 1)
      assert.equal(await evaluate(`accountScenario.puts.length`), 1)
      assert.equal(await evaluate(`accountScenario.mutations.length`), 2)
      await evaluate(
        `[...document.querySelectorAll('button')].find(b=>b.textContent===${JSON.stringify(pt ? "Tentar o mesmo salvamento" : "Retry the same Save")}).click()`,
      )
      await wait(
        `document.querySelector('[data-account-save-state="confirmed"]')`,
      )
      assert.equal(await evaluate(`accountScenario.puts.length`), 2)
      assert.equal(await evaluate(`accountScenario.mutations.length`), 3)
      assert.equal(await evaluate(`accountScenario.puts[1]`), save)
      assert.equal(await evaluate(`accountScenario.pilotPosts.length`), 1)
      assert.equal(await evaluate(`accountScenario.reads.length`), 0)
      console.log(
        JSON.stringify({
          status: "passed",
          locale: pt ? "pt-BR" : "en",
          evidence,
          checks: [
            "combined AccountGate/PilotAccess/StoreProvider in StrictMode",
            "lost activation response recovered by status only",
            "real session expiry timer and same-account reauthentication",
            "unsaved Profile recovery retained",
            "other-account invitation cannot expose or activate A",
            "no automatic activation or Save replay",
            "explicit Retry preserves exact Save identity",
            "zero legacy Profile reads",
          ],
        }),
      )
    },
    {
      path: "/tests/account-harness.html",
      initialize: `localStorage.setItem('careeros_locale',${JSON.stringify(pt ? "pt-BR" : "en")});window.accountServerFixture={pilot:true}`,
    },
  )
}
