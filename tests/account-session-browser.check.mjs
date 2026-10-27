// Actual React account/session/store behavior with synthetic transport only.
import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"
const root = new URL("..", import.meta.url).pathname
const evidence = mkdtempSync(join(tmpdir(), "careeros-account-browser-"))
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const port = await new Promise((resolve) => {
  const s = createServer()
  s.listen(0, "127.0.0.1", () => {
    const p = s.address().port
    s.close(() => resolve(p))
  })
})
const vite = spawn(
  join(root, "node_modules/.bin/vite"),
  ["--host", "127.0.0.1", "--port", String(port), "--strictPort", "--force"],
  {
    cwd: root,
    env: {
      ...process.env,
      CAREEROS_VITE_CACHE_DIR: join(evidence, "vite-cache"),
    },
    stdio: "ignore",
  },
)
const chrome = spawn(
  "google-chrome",
  [
    "--headless=new",
    "--no-sandbox",
    "--disable-dev-shm-usage",
    "--disable-gpu",
    "--remote-allow-origins=*",
    "--remote-debugging-port=0",
    `--user-data-dir=${join(evidence, "chrome")}`,
    "about:blank",
  ],
  { stdio: "ignore" },
)
let socket, inspect
const browserErrors = []
try {
  for (let i = 0; ; i++) {
    try {
      if ((await fetch(`http://127.0.0.1:${port}/`)).ok) break
    } catch {}
    if (i > 100) throw Error("Vite start failed")
    await pause(50)
  }
  let debugPort
  for (let i = 0; ; i++) {
    try {
      debugPort = Number(
        readFileSync(
          join(evidence, "chrome", "DevToolsActivePort"),
          "utf8",
        ).split("\n")[0],
      )
      break
    } catch {}
    if (i > 100) throw Error("Chrome start failed")
    await pause(50)
  }
  const tabs = await (
    await fetch(`http://127.0.0.1:${debugPort}/json/list`)
  ).json()
  socket = new WebSocket(
    tabs.find((t) => t.type === "page").webSocketDebuggerUrl,
  )
  await new Promise((resolve, reject) => {
    socket.onopen = resolve
    socket.onerror = reject
  })
  let serial = 0
  const pending = new Map()
  socket.onmessage = (event) => {
    const m = JSON.parse(event.data)
    if (m.method === "Runtime.exceptionThrown" || m.method === "Log.entryAdded")
      browserErrors.push(m)
    if (m.id) {
      const p = pending.get(m.id)
      pending.delete(m.id)
      m.error ? p.reject(Error(JSON.stringify(m.error))) : p.resolve(m.result)
    }
  }
  const call = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const id = ++serial
      pending.set(id, { resolve, reject })
      socket.send(JSON.stringify({ id, method, params }))
    })
  const evaluate = async (expression) => {
    const r = await call("Runtime.evaluate", {
      expression,
      returnByValue: true,
      awaitPromise: true,
    })
    if (r.exceptionDetails)
      throw Error(
        r.exceptionDetails.exception?.description || r.exceptionDetails.text,
      )
    return r.result.value
  }
  inspect = evaluate
  const wait = async (expression) => {
    for (let i = 0; i < 150; i++) {
      if (await evaluate(`Boolean(${expression})`)) return
      await pause(25)
    }
    throw Error("Timed out: " + expression)
  }
  await call("Page.enable")
  await call("Runtime.enable")
  await call("Log.enable")
  await call("Page.addScriptToEvaluateOnNewDocument", {
    source: `localStorage.setItem('careeros_locale','en');localStorage.setItem('careeros_profile_v2','LEGACY_SENTINEL');localStorage.setItem('careeros_repo','LEGACY_SENTINEL');document.cookie='__Host-careeros_csrf=synthetic; Secure; Path=/'`,
  })
  await call("Page.navigate", {
    url: `http://127.0.0.1:${port}/tests/account-harness.html`,
  })
  await wait(
    `document.querySelector('[data-workspace-account="a"]') && document.querySelector('[data-save]:not(:disabled)')`,
  )
  assert.equal(await evaluate(`accountScenario.reads.length`), 0)
  assert.equal(
    await evaluate(`document.querySelector('[data-profile-name]').textContent`),
    "",
  )
  const fill = async (value) =>
    evaluate(
      `(()=>{const input=document.querySelector('input');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,${JSON.stringify(value)});input.dispatchEvent(new Event('input',{bubbles:true}))})()`,
    )
  await fill("Account A reviewed")
  await evaluate(
    `accountScenario.loseAck=true;document.querySelector('[data-save]').click()`,
  )
  await wait(`document.querySelector('[data-account-save-state="confirmed"]')`)
  assert.equal(await evaluate(`accountScenario.puts.length`), 1)
  assert.equal(
    await evaluate(`document.querySelector('[data-profile-name]').textContent`),
    "Account A reviewed",
  )
  await fill("Unsaved A wording")
  await evaluate(
    `accountScenario.fail=true;document.querySelector('[data-save]').click()`,
  )
  await wait(`document.querySelector('[data-account-save-state="uncertain"]')`)
  const frozen = await evaluate(`accountScenario.puts.at(-1)`)
  await evaluate(`window.dispatchEvent(new Event('focus'))`)
  await wait(
    `document.querySelector('[data-workspace-account="a"]')?.closest('[hidden]')===null && document.querySelector('[data-account-save-state="uncertain"]')`,
  )
  assert.equal(
    await evaluate(`document.querySelector('[data-profile-name]').textContent`),
    "Unsaved A wording",
  )
  await evaluate(
    `accountScenario.fail=false;[...document.querySelectorAll('button')].find(b=>b.textContent==='Retry the same Save').click()`,
  )
  await wait(`document.querySelector('[data-account-save-state="confirmed"]')`)
  assert.equal(await evaluate(`accountScenario.puts.at(-1)`), frozen)
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
} catch (error) {
  console.error(
    JSON.stringify({
      evidence,
      browserErrors,
      body: inspect ? await inspect("document.body.innerText") : null,
    }),
  )
  throw error
} finally {
  socket?.close()
  vite.kill("SIGTERM")
  chrome.kill("SIGTERM")
}
