// Browser regression for the Apply checklist and workflow transitions.
// Uses synthetic API responses; no provider request is made.
import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-apply-checklist-"))
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const port = await new Promise((resolve) => {
  const server = createServer()
  server.listen(0, "127.0.0.1", () => {
    const value = server.address().port
    server.close(() => resolve(value))
  })
})
const repo = {
  fullName: "",
  email: "",
  phone: "",
  location: "",
  professionalLinks: "",
  careerGoals: "Product leader focused on useful services.",
  skills: "Research, Product strategy",
  competencies: "",
  tools: "Figma",
  projects: [
    {
      id: "project",
      name: "Service Atlas",
      description: "Mapped service journeys.",
      technologies: "Figma",
      url: "",
      highlights: "Improved task completion.",
    },
  ],
  education: [
    {
      id: "school",
      degree: "BSc Design",
      institution: "East College",
      location: "",
      graduationDate: "2018",
      details: "",
    },
  ],
  certifications: [
    {
      id: "certificate",
      name: "Research Certificate",
      issuer: "Design Guild",
      date: "2020",
      credentialId: "",
      url: "",
    },
  ],
  languages: [{ id: "language", name: "English", proficiency: "Fluent" }],
  employmentStatus: "",
  currentSalary: "",
  desiredSalary: "",
  additionalInfo: "",
  experience: [
    {
      id: "job",
      company: "Harbor Works",
      title: "Product Lead",
      startDate: "2021",
      endDate: "2024",
      current: false,
      location: "",
      description: "Led a customer platform.",
      responsibilities: "",
      achievements: "Improved onboarding.",
    },
  ],
}
const materials = {
  jobTitle: "Product Lead",
  company: "Harbor Works",
  jobSummary: "Lead product work.",
  resume:
    "# Avery Morgan\n\navery@example.com | https://example.com/avery\n\n## Professional Experience\n### Product Lead · Harbor Works\n2021 — 2024\n- Improved onboarding.\n\n## Projects\n### Service Atlas\n- Improved task completion.\n\n## Languages\n- English: Fluent\n\n## Technical Skills\n- Research\n- Product strategy\n\n## Tools & Technology\n- Figma\n\n## Education\n### BSc Design\nEast College · 2018\n\n## Certifications\n- Research Certificate\n\n## Professional Summary\nProduct leader focused on useful services. Built my_variable service.",
  coverLetter: { greeting: "Dear team,", body: "I led useful service work at Harbor Works.", closing: "Sincerely," },
  applicationAnswers: "1. I led a platform.",
}
const vite = spawn(
  join(root, "node_modules/.bin/vite"),
  ["--host", "127.0.0.1", "--port", String(port), "--strictPort"],
  { cwd: root, stdio: "ignore" },
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
    `--user-data-dir=${join(work, "chrome")}`,
    "about:blank",
  ],
  { stdio: "ignore" },
)
let socket
try {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      if ((await fetch(`http://127.0.0.1:${port}/`)).ok) break
    } catch {}
    if (attempt === 99) throw new Error("Vite did not start")
    await pause(100)
  }
  let debugPort
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      debugPort = Number(
        readFileSync(join(work, "chrome", "DevToolsActivePort"), "utf8").split(
          "\n",
        )[0],
      )
      break
    } catch {}
    if (attempt === 99) throw new Error("Chrome did not start")
    await pause(100)
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
  let nextId = 0
  const pending = new Map()
  socket.onmessage = ({ data }) => {
    const message = JSON.parse(data)
    if (!message.id) return
    const callback = pending.get(message.id)
    pending.delete(message.id)
    if (message.error) callback.reject(new Error(message.error.message))
    else callback.resolve(message.result)
  }
  const call = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const id = ++nextId
      pending.set(id, { resolve, reject })
      socket.send(JSON.stringify({ id, method, params }))
    })
  const evaluate = async (expression) => {
    const result = await call("Runtime.evaluate", {
      expression,
      returnByValue: true,
      awaitPromise: true,
    })
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.text)
    return result.result.value
  }
  const until = async (expression) => {
    for (let attempt = 0; attempt < 100; attempt++) {
      if (await evaluate(expression)) return
      await pause(100)
    }
    throw new Error(`Timed out: ${expression}`)
  }
  await call("Page.navigate", { url: `http://127.0.0.1:${port}/` })
  await until(
    "document.readyState === 'complete' && !!document.querySelector('header button')",
  )
  await evaluate(
    `localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_apikey', 'synthetic-test-key'); location.reload()`,
  )
  await until(
    "document.readyState === 'complete' && !!document.querySelector('header button')",
  )
  const clickApply = async () => {
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('nav button').length === 4")
    await evaluate("document.querySelectorAll('nav button')[0].click()")
    await until(
      "document.querySelectorAll('[data-checklist-state]').length === 3",
    )
  }
  const states = () =>
    evaluate(
      "Array.from(document.querySelectorAll('[data-checklist-state]')).map(el => el.dataset.checklistState)",
    )
  const fill = async (text) => {
    await evaluate(
      `(() => { const el = document.querySelector('main textarea'); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, ${JSON.stringify(text)}); el.dispatchEvent(new Event('input', { bubbles: true })); })()`,
    )
    await pause(50)
  }
  const generate = () =>
    evaluate(
      "document.querySelector('main [data-checklist-state]').parentElement.parentElement.querySelector('button').click()",
    )
  const respond = async (body, status = 200) => {
    await evaluate(
      `window.__pending.shift().resolve(new Response(${JSON.stringify(JSON.stringify(body))}, { status: ${status}, headers: { 'Content-Type': 'application/json' } }))`,
    )
    await pause(70)
  }
  for (const locale of ["en", "pt-BR"]) {
    for (const width of [1440, 390]) {
      await call("Emulation.setDeviceMetricsOverride", {
        width,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false,
      })
      await evaluate(
        `localStorage.setItem('careeros_locale', ${JSON.stringify(locale)}); localStorage.removeItem('careeros_apikey'); localStorage.removeItem('careeros_repo'); location.reload()`,
      )
      await until(
        "document.readyState === 'complete' && !!document.querySelector('header button')",
      )
      await clickApply()
      assert.deepEqual(await states(), [
        "pending",
        "pending",
        "pending",
      ])
      await evaluate(
        `localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_apikey', 'synthetic-test-key'); location.reload()`,
      )
      await until(
        "document.readyState === 'complete' && !!document.querySelector('header button')",
      )
      await clickApply()
      await evaluate(
        `window.__pending = []; window.fetch = (url) => { if (!['/api/qualification-gaps', '/api/application-draft'].includes(url)) throw new Error('Unexpected request'); return new Promise((resolve, reject) => window.__pending.push({ url, resolve, reject })); }`,
      )
      await fill(
        "Product lead with research and strategy experience needed for a service team.",
      )
      assert.deepEqual(await states(), [
        "ready",
        "ready",
        "ready",
      ])
      assert.equal(
        await evaluate(
          "document.querySelector('[data-checklist-state]').textContent.includes('" +
            (locale === "en" ? "not yet validated" : "ainda não validada") +
            "')",
        ),
        true,
      )
      assert.equal(
        await evaluate(
          "document.documentElement.scrollWidth <= window.innerWidth",
        ),
        true,
        `${locale} ${width}: no horizontal overflow`,
      )
      assert.equal(
        await evaluate(
          "getComputedStyle(document.querySelector('[data-checklist-state] span')).backgroundColor",
        ),
        "rgb(24, 113, 59)",
      )
      await generate()
      assert.deepEqual(await states(), [
        "ready",
        "ready",
        "ready",
      ])
      await respond({
        gaps: [
          {
            kind: "skill",
            requirement: "Synthetic qualification",
            details: "Confirm your actual experience.",
          },
        ],
      })
      assert.deepEqual(await states(), [
        "ready",
        "ready",
        "ready",
      ])
      await evaluate("document.querySelector('[role=dialog] input').click()")
      await evaluate(
        "Array.from(document.querySelectorAll('[role=dialog] button')).at(-1).click()",
      )
      assert.deepEqual(await states(), [
        "ready",
        "ready",
        "ready",
      ])
      await respond({ error: "outage" }, 503)
      assert.deepEqual(await states(), [
        "ready",
        "ready",
        "ready",
      ])
      assert.equal(
        await evaluate(
          "!!document.querySelector('[role=dialog] [role=alert]')",
        ),
        true,
      )
      await evaluate("document.querySelector('[role=dialog] input').click()")
      assert.equal(
        await evaluate("!!document.querySelector('[role=dialog] [role=alert]')"),
        false,
        "confirmation edits clear draft failure",
      )
      await evaluate("document.querySelector('[role=dialog] button').click()")
      for (const step of ["qualification", "draft"]) {
        for (const code of [
          "key",
          "input",
          "timeout",
          "outage",
          "invalid_output",
          "rate_limit",
        ]) {
          await generate()
          if (step === "draft") await respond({ gaps: [] })
          await respond({ error: code }, code === "key" ? 401 : 400)
          assert.deepEqual(await states(), [
            code === "key" ? "failed" : "ready",
            code === "input" ? "failed" : "ready",
            code === "input" ? "failed" : "ready",
          ])
          assert.equal(
            await evaluate("!!document.querySelector('[role=alert]')"),
            true,
          )
          if (code === "key" || code === "input") {
            assert.equal(
              await evaluate(
                "getComputedStyle(document.querySelector('[data-checklist-state=failed] span')).backgroundColor",
              ),
              "rgb(177, 27, 50)",
            )
          }
        }
      }
      await fill(
        "Changed posting: Product lead with research and strategy experience needed for a service team.",
      )
      assert.deepEqual(await states(), [
        "ready",
        "ready",
        "ready",
      ])
      await generate()
      await evaluate(
        "window.__pending.shift().reject(new DOMException('Synthetic browser timeout', 'TimeoutError'))",
      )
      await until(
        "document.querySelector('[role=alert]') !== null",
      )
      await generate()
      await fill(
        "Edited during request: Product lead with research and strategy experience needed for a service team.",
      )
      await respond({ gaps: [] })
      assert.deepEqual(
        await states(),
        ["ready", "ready", "ready"],
        "stale input response ignored",
      )
      await generate()
      await evaluate("document.querySelectorAll('nav button')[1].click()")
      await respond({ gaps: [] })
      assert.equal(
        await evaluate(
          "document.querySelectorAll('[data-checklist-state]').length",
        ),
        0,
        "stale response cannot navigate",
      )
      await evaluate("document.querySelectorAll('nav button')[0].click()")
      await until(
        "document.querySelectorAll('[data-checklist-state]').length === 3",
      )
      assert.deepEqual(await states(), [
        "ready",
        "ready",
        "ready",
      ])
      await generate()
      await respond({ gaps: [] })
      assert.deepEqual(await states(), [
        "ready",
        "ready",
        "ready",
      ])
      await respond(materials)
      await until(
        "document.querySelectorAll('[data-checklist-state]').length === 0",
      )
      await evaluate("document.querySelectorAll('nav button')[0].click()")
      await until(
        "document.querySelectorAll('[data-checklist-state]').length === 3",
      )
      await generate()
      await respond({ gaps: [] })
      await respond(materials)
      await until(
        "document.querySelectorAll('[data-checklist-state]').length === 0",
      )
      await evaluate("document.querySelectorAll('nav button')[0].click()")
      await until(
        "document.querySelectorAll('[data-checklist-state]').length === 3",
      )
      const screenshot = await call("Page.captureScreenshot", {
        format: "png",
        captureBeyondViewport: true,
      })
      writeFileSync(
        join(work, `apply-${locale}-${width}.png`),
        Buffer.from(screenshot.data, "base64"),
      )
      console.log(`PASS Apply controlled workflow ${locale} ${width}px`)
    }
  }
  console.log("Screenshots:", work)
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}
