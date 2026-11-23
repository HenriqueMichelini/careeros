import assert from "node:assert/strict"
import { spawn, spawnSync } from "node:child_process"
import { randomBytes } from "node:crypto"
import { mkdtempSync, rmSync } from "node:fs"
import { createServer } from "node:net"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { pause, withAccountBrowser } from "./account-browser.fixture.mjs"

const dir = mkdtempSync(join(tmpdir(), "pilot-status-"))
const command = join(dir, "pilot-operations-local")
const env = {
  ...process.env,
  GOCACHE: "/tmp/careeros-go-cache",
  CAREEROS_OPERATIONS_JOURNAL_DIRECTORY: dir,
  CAREEROS_OPERATIONS_JOURNAL_KEY: randomBytes(32).toString("hex"),
}
function run(args) {
  const result = spawnSync("rtk", ["proxy", ...args], { env, encoding: "utf8" })
  assert.equal(result.status, 0, result.stderr)
}
async function port() {
  return new Promise(resolve => {
    const server = createServer()
    server.listen(0, "127.0.0.1", () => {
      const p = server.address().port
      server.close(() => resolve(p))
    })
  })
}
let server
try {
  run(["go", "build", "-o", command, "./cmd/pilot-operations-local"])
  const next = "2099-12-25T19:00:00Z"
  run([command, "--synthetic-only", "--action", "publish", "--impact", "invitations_and_inference",
    "--state", "paused", "--next-update", next])
  const statusPort = await port()
  const mainPort = await port()
  const origin = `http://127.0.0.1:${statusPort}`
  server = spawn("rtk", ["proxy", command, "--synthetic-only", "--action", "serve-status",
    "--listen", `127.0.0.1:${statusPort}`], { env, stdio: "inherit" })
  for (let i = 0; i < 100; i++) {
    if (await fetch(`${origin}/status/en`).then(r => r.ok, () => false)) break
    assert.equal(server.exitCode, null)
    await pause(25)
  }
  for (const [locale, contact, feedback, status, impact] of [
    ["en", "Email support", "Share optional feedback", "Service status", "Invitations, activation and AI requests"],
    ["pt-BR", "Enviar e-mail ao suporte", "Enviar feedback opcional", "Status do serviço", "Convites, ativação e solicitações de IA"],
  ]) {
    await withAccountBrowser(async ({ evaluate, wait, call, evidence }) => {
      await wait("document.querySelector('footer ul')")
      const links = await evaluate("Array.from(document.querySelectorAll('footer li a'), a => [a.textContent, a.href])")
      assert.deepEqual(links, [
        [contact, "mailto:support@fixture.invalid"],
        [feedback, `mailto:support@fixture.invalid?subject=${encodeURIComponent(feedback)}`],
        [status, `${origin}/status/${locale}`],
      ])
      assert.equal(await evaluate("document.querySelector('footer').textContent.includes('LEGACY_SENTINEL')"), false)
      const otherLocale = locale === "en" ? "pt-BR" : "en"
      const footerUses = lang => `document.querySelector('footer li:last-child a').href.endsWith('/status/${lang}')`
      await evaluate(`document.documentElement.lang = ${JSON.stringify(otherLocale)}`)
      await wait(footerUses(otherLocale))
      assert.equal(await evaluate("localStorage.getItem('careeros_locale')"), locale)
      await evaluate(`document.documentElement.lang = ${JSON.stringify(locale)}`)
      await wait(footerUses(locale))
      await call("Page.navigate", { url: links[2][1] })
      await wait(`document.querySelector('html[lang="${locale}"] time')`)
      assert.ok((await evaluate("document.querySelector('main').textContent")).includes(impact))
      assert.equal(await evaluate("document.querySelector('main time').dateTime"), next)
      console.log(JSON.stringify({ passed: true, locale, evidence, observed: "React footer and independent Go status page with application API unavailable" }))
    }, {
      path: "/",
      env: {
        VITE_CAREEROS_PILOT_MODE: "1",
        VITE_CAREEROS_PILOT_FAKE_SUPPORT_EMAIL: "support@fixture.invalid",
        VITE_CAREEROS_PILOT_STATUS_ORIGIN: origin,
        CAREEROS_API_TARGET: `http://127.0.0.1:${mainPort}`,
      },
      initialize: `localStorage.setItem("careeros_locale", ${JSON.stringify(locale)})`,
    })
  }
} finally {
  if (server && server.exitCode === null) {
    const exited = new Promise(resolve => server.once("exit", resolve))
    server.kill("SIGTERM")
    await exited
  }
  rmSync(dir, { recursive: true, force: true })
}
