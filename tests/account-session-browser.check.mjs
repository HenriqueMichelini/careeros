// Actual React session transitions with independently expected recovery outcomes.
import assert from "node:assert/strict"
import { withAccountBrowser } from "./account-browser.fixture.mjs"

await withAccountBrowser(async ({ evidence, call, evaluate, wait, fill }) => {
  const scenario = process.argv[2] ?? "replacement"
  const sentinel =
    scenario === "replacement"
      ? "REAUTH_UNSAVED_SENTINEL"
      : "EXPIRED_UNSAVED_SENTINEL"
  await fill(sentinel)
  await evaluate(
    `accountScenario.fail=true;document.querySelector('[data-save]').click()`,
  )
  await wait(`document.querySelector('[data-account-save-state="uncertain"]')`)
  const frozenSave = await evaluate(`accountScenario.puts.at(-1)`)
  const count = await evaluate(`accountScenario.puts.length`)
  if (scenario === "expiry") {
    // Let the actual AccountGate expiry timer re-probe an expired session.
    await evaluate(
      `accountScenario.autoExpire=true;accountScenario.expires=new Date(Date.now()+150).toISOString();window.dispatchEvent(new Event('focus'))`,
    )
    await wait(`document.body.innerText.includes('Sign in to create')`)
    assert.equal(
      await evaluate(
        `document.querySelector('[data-workspace-account]')===null`,
      ),
      true,
    )
    assert.equal(
      await evaluate(
        `document.body.innerText.includes(${JSON.stringify(sentinel)})`,
      ),
      false,
    )
    await evaluate(
      `accountScenario.autoExpire=false;accountScenario.account='a';accountScenario.fail=false;accountScenario.expires=new Date(Date.now()+7200000).toISOString();window.dispatchEvent(new Event('focus'))`,
    )
  } else {
    await evaluate(
      `accountScenario.fail=false;accountScenario.expires=new Date(Date.now()+7200000).toISOString();window.dispatchEvent(new Event('focus'))`,
    )
  }
  await wait(
    `document.querySelector('[data-workspace-account="a"]') && document.querySelector('[data-save]:not(:disabled)') && document.querySelector('[data-workspace-account="a"]').closest('[hidden]')===null`,
  )
  assert.equal(
    await evaluate(`document.querySelector('[data-profile-name]').textContent`),
    sentinel,
  )
  assert.equal(
    await evaluate(
      `!![...document.querySelectorAll('button')].find(b=>b.textContent==='Retry the same Save')`,
    ),
    true,
  )
  assert.equal(
    await evaluate(`accountScenario.puts.length`),
    count,
    "reauthentication must not replay a pending Save",
  )
  await evaluate(
    `[...document.querySelectorAll('button')].find(b=>b.textContent==='Retry the same Save').click()`,
  )
  await wait(`document.querySelector('[data-account-save-state="confirmed"]')`)
  assert.equal(
    await evaluate(`accountScenario.puts.at(-1)`),
    frozenSave,
    "recovery must reuse exact request bytes/base/operation identity",
  )
  if (scenario === "lifecycle") {
    await fill("KEEP_A_RECOVERY_SENTINEL")
    await evaluate(
      `accountScenario.fail=true;document.querySelector('[data-save]').click()`,
    )
    await wait(
      `document.querySelector('[data-account-save-state="uncertain"]')`,
    )
    const pendingA = await evaluate(`accountScenario.puts.at(-1)`)
    const putCount = await evaluate(`accountScenario.puts.length`)
    await evaluate(
      `accountScenario.fail=false;accountScenario.account='b';window.dispatchEvent(new Event('focus'))`,
    )
    await wait(
      `document.querySelector('[data-workspace-account="b"]') && document.querySelector('[data-save]:not(:disabled)')`,
    )
    assert.equal(
      await evaluate(
        `document.body.innerText.includes('KEEP_A_RECOVERY_SENTINEL')`,
      ),
      false,
    )
    assert.equal(
      await evaluate(
        `document.querySelector('[data-profile-name]').textContent`,
      ),
      "",
    )
    assert.equal(
      await evaluate(
        `!![...document.querySelectorAll('button')].find(b=>b.textContent==='Retry the same Save')`,
      ),
      false,
    )
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
      "KEEP_A_RECOVERY_SENTINEL",
    )
    assert.equal(await evaluate(`accountScenario.puts.length`), putCount)
    await evaluate(`document.querySelector('[data-sign-out]').click()`)
    await wait(`document.querySelector('[data-pending-logout]')`)
    assert.equal(
      await evaluate(`accountScenario.logoutCalls`),
      0,
      "logout must await a deliberate recovery choice",
    )
    await evaluate(`document.querySelector('[data-logout-cancel]').click()`)
    assert.equal(await evaluate(`accountScenario.logoutCalls`), 0)
    assert.equal(
      await evaluate(
        `document.querySelector('[data-profile-name]').textContent`,
      ),
      "KEEP_A_RECOVERY_SENTINEL",
    )
    await evaluate(`document.querySelector('[data-sign-out]').click()`)
    await wait(`document.querySelector('[data-pending-logout]')`)
    await evaluate(`document.querySelector('[data-logout-keep]').click()`)
    await wait(`document.body.innerText.includes('Sign in to create')`)
    assert.equal(
      await evaluate(
        `document.querySelector('[data-workspace-account]')===null`,
      ),
      true,
    )
    assert.equal(
      await evaluate(
        `document.body.innerText.includes('KEEP_A_RECOVERY_SENTINEL')`,
      ),
      false,
    )
    assert.equal(
      await evaluate(
        `document.querySelector('a[href="/api/auth/google/start"]').target`,
      ),
      "_blank",
    )
    await evaluate(
      `accountScenario.account='a';window.dispatchEvent(new Event('focus'))`,
    )
    await wait(
      `document.querySelector('[data-workspace-account="a"]') && document.querySelector('[data-account-save-state="uncertain"]')`,
    )
    assert.equal(
      await evaluate(
        `JSON.stringify(JSON.parse(sessionStorage.getItem('careeros_pending_profile_v1:a')).pendingSave)`,
      ),
      pendingA,
    )
    // Reconstruct the application, preserving only the synthetic server model
    // and the real tab checkpoint. This represents redirect/reload recovery.
    const serverModel = await evaluate(
      `({account:accountScenario.account,expires:accountScenario.expires,documents:accountScenario.documents,operations:accountScenario.operations,puts:accountScenario.puts,logoutCalls:accountScenario.logoutCalls})`,
    )
    await call("Page.addScriptToEvaluateOnNewDocument", {
      source: `window.accountReloadMarker=true;window.accountServerFixture=${JSON.stringify(serverModel)}`,
    })
    await call("Page.reload")
    await wait(
      `window.accountReloadMarker && document.querySelector('[data-workspace-account="a"]') && document.querySelector('[data-account-save-state="uncertain"]')`,
    )
    assert.equal(
      await evaluate(
        `document.querySelector('[data-profile-name]').textContent`,
      ),
      "KEEP_A_RECOVERY_SENTINEL",
    )
    assert.equal(await evaluate(`accountScenario.puts.length`), putCount)
    assert.equal(
      await evaluate(
        `JSON.stringify(JSON.parse(sessionStorage.getItem('careeros_pending_profile_v1:a')).pendingSave)`,
      ),
      pendingA,
    )
    assert.equal(
      await evaluate(
        `sessionStorage.getItem('careeros_pending_profile_v1:a').includes('synthetic-csrf')`,
      ),
      false,
    )
    await evaluate(`document.querySelector('[data-sign-out]').click()`)
    await wait(`document.querySelector('[data-logout-discard]')`)
    await evaluate(`document.querySelector('[data-logout-discard]').click()`)
    await wait(`document.body.innerText.includes('Sign in to create')`)
    assert.equal(
      await evaluate(`sessionStorage.getItem('careeros_pending_profile_v1:a')`),
      null,
    )
    await evaluate(
      `accountScenario.account='a';window.dispatchEvent(new Event('focus'))`,
    )
    await wait(
      `document.querySelector('[data-workspace-account="a"]') && document.querySelector('[data-save]:not(:disabled)')`,
    )
    assert.equal(
      await evaluate(
        `document.body.innerText.includes('KEEP_A_RECOVERY_SENTINEL')`,
      ),
      false,
    )
    assert.equal(
      await evaluate(`!!document.querySelector('[data-discard-recovery]')`),
      false,
    )
    assert.equal(await evaluate(`accountScenario.puts.length`), putCount)
    assert.equal(await evaluate(`accountScenario.reads.length`), 0)
  }
  console.log(
    JSON.stringify({
      status: "passed",
      scenario,
      evidence,
      checks: [
        "pending wording survives session transition",
        "signed-out/other-account content fenced",
        "no automatic replay",
        "identical recovery request",
        ...(scenario === "lifecycle"
          ? [
              "account switch retains only its own recovery",
              "logout cancel/keep/discard",
              "fresh application reload restores untrusted pending draft only after server read",
              "checkpoint excludes credentials",
            ]
          : []),
      ],
    }),
  )
})
