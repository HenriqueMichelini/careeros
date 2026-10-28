// Shared synthetic-browser fixture. Scenario assertions stay explicit below
// and in each caller; no production transport or live provider is used.
import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"
export const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

async function until(read, message, attempts = 150, interval = 25) {
  const value = await read()
  if (value) return value
  if (attempts <= 1) throw new Error(message)
  await pause(interval)
  return until(read, message, attempts - 1, interval)
}

export async function withAccountBrowser(run, options = {}) {
  const root = new URL("..", import.meta.url).pathname
  const evidence = mkdtempSync(join(tmpdir(), "careeros-account-browser-"))
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
        ...options.env,
        CAREEROS_VITE_CACHE_DIR: join(evidence, "vite-cache"),
      },
      stdio: "ignore",
    },
  )
  const chrome = spawn(
    "/usr/bin/google-chrome",
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
    await until(
      async () => {
        try {
          return (await fetch(`http://127.0.0.1:${port}/`)).ok
        } catch {
          return false
        }
      },
      "Vite start failed",
      101,
      50,
    )
    const debugPort = await until(
      () => {
        try {
          return Number(
            readFileSync(
              join(evidence, "chrome", "DevToolsActivePort"),
              "utf8",
            ).split("\n")[0],
          )
        } catch {
          return null
        }
      },
      "Chrome start failed",
      101,
      50,
    )
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
      if (
        m.method === "Runtime.exceptionThrown" ||
        m.method === "Log.entryAdded"
      )
        browserErrors.push(m)
      if (m.id) {
        const p = pending.get(m.id)
        pending.delete(m.id)
        m.error
          ? p.reject(new Error(JSON.stringify(m.error)))
          : p.resolve(m.result)
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
        throw new Error(
          r.exceptionDetails.exception?.description || r.exceptionDetails.text,
        )
      return r.result.value
    }
    inspect = evaluate
    const wait = (expression) =>
      until(
        () => evaluate(`Boolean(${expression})`),
        "Timed out: " + expression,
      )
    await call("Page.enable")
    await call("Runtime.enable")
    await call("Log.enable")
    await call("Page.addScriptToEvaluateOnNewDocument", {
      source: `localStorage.setItem('careeros_locale','en');localStorage.setItem('careeros_profile_v2','LEGACY_SENTINEL');localStorage.setItem('careeros_repo','LEGACY_SENTINEL');${
        options.path
          ? ""
          : "document.cookie='__Host-careeros_csrf=synthetic; Secure; Path=/'"
      }`,
    })
    if (options.initialize)
      await call("Page.addScriptToEvaluateOnNewDocument", {
        source: options.initialize,
      })
    await options.beforeNavigate?.({ port, evidence })
    await call("Page.navigate", {
      url: `http://127.0.0.1:${port}${options.path ?? "/tests/account-harness.html"}`,
    })
    if (options.path) {
      await run({ evidence, call, evaluate, wait, port })
      return
    }
    await wait(
      `document.querySelector('[data-workspace-account="a"]') && document.querySelector('[data-save]:not(:disabled)')`,
    )
    assert.equal(await evaluate(`accountScenario.reads.length`), 0)
    assert.equal(
      await evaluate(
        `document.querySelector('[data-profile-name]').textContent`,
      ),
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
    await wait(
      `document.querySelector('[data-account-save-state="confirmed"]')`,
    )
    assert.equal(await evaluate(`accountScenario.puts.length`), 1)
    assert.equal(
      await evaluate(
        `document.querySelector('[data-profile-name]').textContent`,
      ),
      "Account A reviewed",
    )
    await fill("Unsaved A wording")
    await evaluate(
      `accountScenario.fail=true;document.querySelector('[data-save]').click()`,
    )
    await wait(
      `document.querySelector('[data-account-save-state="uncertain"]')`,
    )
    const frozen = await evaluate(`accountScenario.puts.at(-1)`)
    await evaluate(`window.dispatchEvent(new Event('focus'))`)
    await wait(
      `document.querySelector('[data-workspace-account="a"]')?.closest('[hidden]')===null && document.querySelector('[data-account-save-state="uncertain"]')`,
    )
    assert.equal(
      await evaluate(
        `document.querySelector('[data-profile-name]').textContent`,
      ),
      "Unsaved A wording",
    )
    await evaluate(
      `accountScenario.fail=false;[...document.querySelectorAll('button')].find(b=>b.textContent==='Retry the same Save').click()`,
    )
    await wait(
      `document.querySelector('[data-account-save-state="confirmed"]')`,
    )
    assert.equal(await evaluate(`accountScenario.puts.at(-1)`), frozen)
    await run({ evidence, call, evaluate, wait, fill })
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
}
