// Loopback-only browser fixture. No application keys or provider traffic.
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, rmSync } from "node:fs"
import { createServer } from "node:http"
import { tmpdir } from "node:os"
import { join } from "node:path"

export async function inspectionBrowserFixture() {
  const work = mkdtempSync(join(tmpdir(), "careeros-inspection-browser-"))
  const requests = []
  let authenticated = true
  let reply = (_req, res) => {
    res.writeHead(404, { "Content-Type": "text/html" })
    res.end("Page not found")
  }
  const servers = [false, true].map((auth) =>
    createServer(async (req, res) => {
      const chunks = []
      for await (const chunk of req) chunks.push(chunk)
      requests.push({
        auth,
        method: req.method,
        url: req.url,
        body: Buffer.concat(chunks),
      })
      if (req.url === "/app") {
        if (!authenticated) {
          res.writeHead(302, { Location: authOrigin + "/auth" })
          res.end()
          return
        }
        res.writeHead(200, {
          "Content-Type": "text/html",
          "Cache-Control": "no-store",
        })
        res.end(
          '<!doctype html><div id="root"><h1>CareerOS controlled fixture</h1></div>',
        )
      } else if (auth) {
        res.writeHead(404, { "Content-Type": "text/html" })
        res.end("Team authentication / Page not found")
      } else reply(req, res)
    }),
  )
  await Promise.all(
    servers.map(
      (server) =>
        new Promise((resolve) => server.listen(0, "127.0.0.1", resolve)),
    ),
  )
  const [origin, authOrigin] = servers.map(
    (server) => `http://127.0.0.1:${server.address().port}`,
  )
  const chrome = spawn(
    "/usr/bin/google-chrome",
    [
      "--headless=new",
      "--no-sandbox",
      "--disable-dev-shm-usage",
      "--disable-gpu",
      "--disable-background-networking",
      "--disable-component-update",
      "--no-first-run",
      "--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE 127.0.0.1",
      "--remote-allow-origins=*",
      "--remote-debugging-port=0",
      `--user-data-dir=${work}`,
      "about:blank",
    ],
    { stdio: "ignore" },
  )
  let socket
  const pending = new Map(),
    listeners = new Map()
  const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
  try {
    let debugPort
    for (let attempt = 0; attempt < 100; attempt++) {
      try {
        debugPort = Number(
          readFileSync(join(work, "DevToolsActivePort"), "utf8").split("\n")[0],
        )
        break
      } catch {}
      if (attempt === 99) throw Error("Controlled Chrome did not start")
      await pause(50)
    }
    const tabs = await (
      await fetch(`http://127.0.0.1:${debugPort}/json/list`)
    ).json()
    socket = new WebSocket(
      tabs.find((tab) => tab.type === "page").webSocketDebuggerUrl,
    )
    await new Promise((resolve, reject) => {
      socket.onopen = resolve
      socket.onerror = reject
    })
    socket.onmessage = ({ data }) => {
      const message = JSON.parse(data)
      if (message.id) {
        const entry = pending.get(message.id)
        if (!entry) return
        pending.delete(message.id)
        clearTimeout(entry.timer)
        if (message.error) entry.reject(Error(message.error.message))
        else entry.resolve(message.result)
      } else
        for (const listener of listeners.get(message.method) ?? [])
          listener(message.params)
    }
    let nextID = 0
    const call = (method, params = {}) =>
      new Promise((resolve, reject) => {
        const id = ++nextID
        const timer = setTimeout(() => {
          pending.delete(id)
          reject(Error(`CDP timeout: ${method}`))
        }, 10000)
        pending.set(id, { resolve, reject, timer })
        socket.send(JSON.stringify({ id, method, params }))
      })
    const onEvent = (method, listener) => {
      if (!listeners.has(method)) listeners.set(method, new Set())
      listeners.get(method).add(listener)
      return () => listeners.get(method).delete(listener)
    }
    const evaluate = async (expression) => {
      const result = await call("Runtime.evaluate", {
        expression,
        awaitPromise: true,
        returnByValue: true,
      })
      if (result.exceptionDetails) throw Error(result.exceptionDetails.text)
      return result.result.value
    }
    const navigate = async (url) => {
      await call("Page.navigate", { url })
      for (let attempt = 0; attempt < 100; attempt++) {
        if (
          await evaluate(
            `location.href === ${JSON.stringify(url)} && document.readyState === 'complete'`,
          )
        )
          return
        await pause(20)
      }
      throw Error("Controlled navigation did not complete")
    }
    return {
      call,
      onEvent,
      evaluate,
      navigate,
      origin,
      authOrigin,
      requests,
      respond: (handler) => {
        reply = handler
      },
      setAuthenticated: (value) => {
        authenticated = value
      },
      close: async () => {
        socket.close()
        for (const entry of pending.values()) {
          clearTimeout(entry.timer)
          entry.reject(Error("Browser closed"))
        }
        chrome.kill()
        await new Promise((resolve) => chrome.once("exit", resolve))
        await Promise.all(
          servers.map(
            (server) =>
              new Promise((resolve) => {
                server.closeAllConnections()
                server.close(resolve)
              }),
          ),
        )
        rmSync(work, { recursive: true, force: true })
      },
    }
  } catch (error) {
    socket?.close()
    chrome.kill()
    for (const server of servers) server.close()
    throw error
  }
}
