import { reviewedDraftFixture } from "./reviewed-resume-fixture.mjs"
import { jobContextFixture } from "./job-context-fixtures.mjs"
import { savedProfileExpression } from "./profile-browser-storage.mjs"
import { keyboardFlow } from "./keyboard-flow.mjs"
// Browser regression for the Apply checklist and workflow transitions.
// Uses synthetic API responses; no provider request is made.
import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-application-reuse-"))
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
  coverLetter: {
    greeting: "Dear team,",
    body: "I led useful service work at Harbor Works.",
    closing: "Sincerely,",
  },
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
    if (result.exceptionDetails)
      throw new Error(
        result.exceptionDetails.exception?.description ||
          result.exceptionDetails.text,
      )
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
    `localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_apikey', 'synthetic-test-key'); localStorage.setItem('careeros_typesafe_key', 'synthetic-typesafe'); location.reload()`,
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
  const generate = async () => {
    await until(
      "!!document.querySelector('main textarea') && !!document.querySelector('main [data-checklist-state]')",
    )
    return evaluate(
      "document.querySelector('main [data-checklist-state]').parentElement.parentElement.querySelector('button').click()",
    )
  }
  const { keyboardFill, keyboardSubmit } = keyboardFlow({
    call,
    evaluate,
    pause,
    selector: "main textarea",
  })
  const respond = async (
    body,
    status = 200,
    inspectOnly = false,
    fixtureState = null,
  ) => {
    if (body.gaps && !body.jobContext) {
      const posting = await evaluate(
        "JSON.parse(window.__pending[0].options.body).jobPosting",
      )
      body = { ...body, jobContext: jobContextFixture(posting) }
    }
    if (body.gaps && !body.matches && body.jobContext) {
      const doc = await evaluate(
        "JSON.parse(window.__pending[0].options.body).profileEvidence",
      )
      body.matches = body.jobContext.job.qualifications.map(
        (requirement, index) => ({
          requirementIndex: index,
          requirement,
          state:
            fixtureState?.state ??
            (body.gaps.length ? "not_evidenced" : "supported"),
          factIds:
            fixtureState?.state === "partially_supported" || !body.gaps.length
              ? [doc.facts[0].id]
              : [],
          explanation: "Controlled support explanation.",
          question:
            fixtureState?.state === "needs_clarification"
              ? "Have you used AWS in a project?"
              : "",
          complete: fixtureState?.complete ?? true,
          excluded: fixtureState?.complete === false ? 1 : 0,
          facts: doc.facts
            .slice(fixtureState?.complete === false ? 1 : 0)
            .map((f) => ({
              ...f,
              section:
                doc.entities.find((e) => e.id === f.owner.id)?.kind ??
                "profile",
              evidence: [],
            })),
        }),
      )
    }
    if (body.resume && !body.resumeReview && status === 200)
      body = reviewedDraftFixture(
        body,
        await evaluate("JSON.parse(window.__pending[0].options.body)"),
      )
    await evaluate(
      `window.__pending.shift().resolve(new Response(${JSON.stringify(JSON.stringify(body))}, { status: ${status}, headers: { 'Content-Type': 'application/json' } }))`,
    )
    await pause(70)
    if (body.artifactReview && status === 200) {
      await until("!!document.querySelector('.artifact-review')")
      await evaluate(
        "Array.from(document.querySelectorAll('.artifact-review button')).find(b=>/^(Accept application materials|Aceitar materiais da candidatura)$/.test(b.textContent.trim())).click()",
      )
    }
    if (
      body.gaps?.length === 0 &&
      !inspectOnly &&
      (await evaluate("!!document.querySelector('[role=dialog]')"))
    ) {
      await evaluate(
        "Array.from(document.querySelectorAll('[role=dialog] button')).at(-2).click()",
      )
      await pause(70)
    }
  }
  for (const [locale, width] of [
    ["en", 1440],
    ["pt-BR", 390],
  ]) {
    await call("Emulation.setDeviceMetricsOverride", {
      width,
      height: 1000,
      deviceScaleFactor: 1,
      mobile: false,
    })
    await evaluate(
      `localStorage.setItem('careeros_locale', ${JSON.stringify(locale)}); localStorage.setItem('careeros_cv_language', ${JSON.stringify(locale)}); localStorage.setItem('careeros_apikey','synthetic-test-key'); localStorage.setItem('careeros_typesafe_key','synthetic-typesafe'); localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); location.reload()`,
    )
    await until(
      "document.readyState === 'complete' && !!document.querySelector('header button')",
    )
    await clickApply()
    // The synthetic transport deliberately ignores abort, exposing late completions.
    await evaluate(
      `window.__pending = []; window.__requests = []; window.fetch = (url, options) => new Promise((resolve, reject) => { const r = {url,options,resolve,reject}; window.__requests.push(r); window.__pending.push(r); })`,
    )
    const posting = "Java developer. AWS required."
    const gaps = [
      { kind: "skill", requirement: "AWS", details: "AWS required." },
    ]
    const payload = () =>
      evaluate("JSON.parse(window.__pending[0].options.body)")
    const requestCount = (url) =>
      evaluate(
        `window.__requests.filter(r=>r.url===${JSON.stringify(url)}).length`,
      )
    const draftClick = async () => {
      await evaluate(
        "Array.from(document.querySelectorAll('[role=dialog] button')).at(-2).click()",
      )
      await until("window.__pending.length === 1")
      assert.equal(
        await evaluate("window.__pending[0].url"),
        "/api/application-draft",
      )
    }
    const dismiss = async () => {
      await evaluate("document.querySelector('[role=dialog] button').click()")
      await pause(60)
    }
    const setNote = async (note) => {
      await evaluate(
        `(() => { const e=document.querySelector('[role=dialog] textarea'); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(e, ${JSON.stringify(note)}); e.dispatchEvent(new Event('input',{bubbles:true})); })()`,
      )
      await pause(60)
    }
    const saved = await evaluate(savedProfileExpression)
    await fill(posting)
    await generate()
    await respond({ gaps }, 200, true)
    await setNote("I have not used AWS.")
    await draftClick()
    await respond({ error: "outage" }, 502)
    assert.equal(await requestCount("/api/qualification-gaps"), 1)
    await dismiss()
    await generate()
    await until("!!document.querySelector('[role=dialog]')")
    assert.equal(
      await evaluate("window.__pending.length"),
      0,
      "unchanged retry must reuse qualification analysis",
    )
    assert.equal(
      await evaluate("document.querySelector('[role=dialog] textarea').value"),
      "I have not used AWS.",
    )
    await setNote("I use AWS only for personal study.")
    await draftClick()
    assert.deepEqual((await payload()).qualificationAnswers, [
      { requirement: "AWS", userContext: "I use AWS only for personal study." },
    ])
    await evaluate("document.querySelector('[role=dialog] button').click()")
    assert.equal(
      await evaluate("window.__pending[0].options.signal.aborted"),
      true,
      "Cancel must abort draft helper",
    )
    const lateInput = await payload()
    const late = reviewedDraftFixture(materials, lateInput)
    await evaluate(
      `window.__pending.shift().resolve(new Response(${JSON.stringify(JSON.stringify(late))},{status:200}))`,
    )
    await pause(100)
    assert.equal(
      await evaluate("!!document.querySelector('.results-page')"),
      false,
      "late successful draft cannot navigate",
    )
    assert.equal(await evaluate(savedProfileExpression), saved)
    await draftClick()
    await respond(materials)
    await until("!!document.querySelector('.results-page')")
    const currentResults = await evaluate(
      "document.querySelector('.results-content').textContent",
    )
    assert.equal(await requestCount("/api/qualification-gaps"), 1)
    await evaluate("document.querySelectorAll('header nav button')[0].click()")
    await generate()
    await draftClick()
    await respond({ error: "outage" }, 502)
    await evaluate("document.querySelectorAll('header nav button')[3].click()")
    await until("!!document.querySelector('.results-page')")
    assert.equal(
      await evaluate("document.querySelector('.results-content').textContent"),
      currentResults,
      "failed retry preserves previous valid results",
    )
    assert.equal(
      await evaluate(
        "document.querySelectorAll('[data-checklist-state]').length",
      ),
      0,
      "inactive Apply must have no controls",
    )
    // New Application resets reuse even for identical posting text.
    await evaluate(
      "Array.from(document.querySelectorAll('.results-page button')).find(b=>/^(New Application|Nova candidatura)$/i.test(b.textContent.trim())).click()",
    )
    await until("!!document.querySelector('main textarea')")
    assert.equal(
      await evaluate("document.querySelector('main textarea').value"),
      "",
    )
    await fill(posting)
    await generate()
    assert.equal((await payload()).jobContext, undefined)
    await respond({ gaps }, 200, true)
    // Language is a draft dependency; it does not alter posting-language analysis.
    await dismiss()
    await evaluate(
      `(() => { const e=document.querySelector('main select'); e.value=${JSON.stringify(locale === "en" ? "pt-BR" : "en")}; e.dispatchEvent(new Event('change',{bubbles:true})); })()`,
    )
    await pause(60)
    await generate()
    assert.equal(await evaluate("window.__pending.length"), 0)
    await draftClick()
    assert.equal((await payload()).cvLanguage, locale === "en" ? "pt-BR" : "en")
    await respond({ error: "outage" }, 502)
    await dismiss()
    // A Profile fact revision invalidates matching, but not independent job context.
    const previousContext = await evaluate(
      "window.__requests.filter(r=>r.url==='/api/application-draft').at(-1).options.body",
    )
    await evaluate("document.querySelectorAll('header nav button')[1].click()")
    await until("!!document.querySelector('aside nav button')")
    await evaluate("document.querySelectorAll('aside nav button')[2].click()")
    await until(
      "document.querySelector('[data-profile-content]')?.dataset.profileContent === 'skills'",
    )
    await evaluate(
      "(() => { const e=Array.from(document.querySelectorAll('[data-profile-content] textarea')).find(e=>!e.closest('[data-fact-details]')); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(e,'Research, Product strategy, AWS'); e.dispatchEvent(new Event('input',{bubbles:true})); })()",
    )
    await pause(120)
    await evaluate("document.querySelectorAll('header nav button')[0].click()")
    await generate()
    assert.deepEqual(
      (await payload()).jobContext,
      JSON.parse(previousContext).jobContext,
    )
    assert.ok(
      (await payload()).profileEvidence.facts.some(
        (f) => f.value === "Research, Product strategy, AWS",
      ),
    )
    await respond({ gaps }, 200, true)
    await draftClick()
    await respond({ error: "job_context_stale" }, 409)
    assert.equal(
      await evaluate("window.__pending.length"),
      0,
      "stale artifacts cannot trigger repair",
    )
    assert.equal(
      await evaluate("document.querySelector('main textarea').value"),
      posting,
    )
    await evaluate(
      "Array.from(document.querySelectorAll('main button')).find(b=>/^(Discard analysis and analyze again|Descartar análise e analisar novamente)$/.test(b.textContent.trim())).click()",
    )
    await until("window.__pending.length === 1")
    assert.equal(
      (await payload()).jobContext,
      undefined,
      "explicit reanalysis must discard unverifiable context",
    )
    await respond({ error: "outage" }, 502, true)
    await generate()
    assert.equal(
      (await payload()).jobContext,
      undefined,
      "failed fresh extraction must not resurrect the discarded artifact",
    )
    await respond({ gaps }, 200, true)
    await dismiss()
    // A new exact original invalidates all analysis and aborts pending extraction.
    await fill(posting + " ")
    await generate()
    assert.equal((await payload()).jobContext, undefined)
    await fill(posting + "\nBuild Java APIs.")
    assert.equal(
      await evaluate("window.__pending[0].options.signal.aborted"),
      true,
    )
    await respond({ gaps }, 200, true)
    assert.equal(
      await evaluate("!!document.querySelector('[role=dialog]')"),
      false,
    )
    await generate()
    await evaluate("document.querySelectorAll('header nav button')[1].click()")
    assert.equal(
      await evaluate("window.__pending[0].options.signal.aborted"),
      true,
      "navigation aborts qualification request",
    )
    await respond({ gaps }, 200, true)
    assert.equal(
      await evaluate(
        "document.querySelectorAll('[data-checklist-state]').length",
      ),
      0,
    )
    await evaluate("document.querySelectorAll('header nav button')[0].click()")
    await generate()
    await evaluate(
      "Array.from(document.querySelectorAll('main button')).find(b=>/^(Cancel request|Cancelar solicitação)$/.test(b.textContent.trim())).click()",
    )
    assert.equal(
      await evaluate("window.__pending[0].options.signal.aborted"),
      true,
    )
    await respond({ gaps }, 200, true)
    assert.equal(
      await evaluate("!!document.querySelector('[role=dialog]')"),
      false,
    )
    // A completed job stage returned with a later failure survives deliberate retry.
    await generate()
    const partialPosting = (await payload()).jobPosting
    await respond(
      { error: "outage", jobContext: jobContextFixture(partialPosting) },
      502,
      true,
    )
    await generate()
    assert.deepEqual(
      (await payload()).jobContext,
      jobContextFixture(partialPosting),
    )
    await respond({ gaps }, 200, true)
    await dismiss()
    // Changing user credentials affects requests, while valid analysis stays reusable.
    await generate()
    await draftClick()
    await evaluate(
      "(() => { const e=document.querySelector('#job-typesafe-key'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(e,'changed-synthetic-typesafe'); e.dispatchEvent(new Event('input',{bubbles:true})); })()",
    )
    await pause(70)
    assert.equal(
      await evaluate("window.__pending[0].options.signal.aborted"),
      true,
    )
    await respond({ error: "key" }, 401)
    await draftClick()
    assert.equal(
      await evaluate(
        "window.__pending[0].options.headers['X-TypeSafe-Api-Key']",
      ),
      "changed-synthetic-typesafe",
    )
    await respond({ error: "outage" }, 502)
    await dismiss()
    await evaluate(
      "Array.from(document.querySelectorAll('header button')).find(b=>b.textContent.includes('API')).click()",
    )
    await until("!!document.querySelector('header input[type=password]')")
    await evaluate(
      "(() => { const e=document.querySelector('header input[type=password]'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(e,'changed-synthetic-openai'); e.dispatchEvent(new Event('input',{bubbles:true})); })()",
    )
    await pause(60)
    await evaluate(
      "Array.from(document.querySelectorAll('header button')).find(b=>/^(Save|Salvar)$/.test(b.textContent.trim())).click()",
    )
    await pause(60)
    const beforeKeyRetry = await requestCount("/api/qualification-gaps")
    await generate()
    await draftClick()
    assert.equal(
      await evaluate("window.__pending[0].options.headers['X-OpenAI-Api-Key']"),
      "changed-synthetic-openai",
    )
    assert.equal(await requestCount("/api/qualification-gaps"), beforeKeyRetry)
    await respond({ error: "outage" }, 502)
    await dismiss()
    assert.equal(
      await evaluate("document.documentElement.scrollWidth <= innerWidth"),
      true,
    )
    const shot = await call("Page.captureScreenshot", { format: "png" })
    writeFileSync(
      join(work, `reuse-${locale}-${width}.png`),
      Buffer.from(shot.data, "base64"),
    )
    console.log(`PASS session reuse/cancellation ${locale} ${width}px`)
  }
  console.log("Evidence:", work)
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}
