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
    await evaluate(
      `window.__pending.shift().resolve(new Response(${JSON.stringify(JSON.stringify(body))}, { status: ${status}, headers: { 'Content-Type': 'application/json' } }))`,
    )
    await pause(70)
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
  for (const locale of ["en", "pt-BR"]) {
    for (const width of [1440, 390]) {
      await call("Emulation.setDeviceMetricsOverride", {
        width,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false,
      })
      await evaluate(
        `localStorage.setItem('careeros_locale', ${JSON.stringify(locale)}); localStorage.removeItem('careeros_apikey'); localStorage.removeItem('careeros_profile_v2'); localStorage.removeItem('careeros_repo'); location.reload()`,
      )
      await until(
        "document.readyState === 'complete' && !!document.querySelector('header button')",
      )
      await clickApply()
      assert.deepEqual(await states(), ["pending", "pending", "pending"])
      await evaluate(
        `localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_apikey', 'synthetic-test-key'); localStorage.setItem('careeros_typesafe_key', 'synthetic-typesafe'); location.reload()`,
      )
      await until(
        "document.readyState === 'complete' && !!document.querySelector('header button')",
      )
      await clickApply()
      await evaluate(
        `window.__pending = []; window.fetch = (url, options) => { if (!['/api/qualification-gaps', '/api/application-draft'].includes(url)) throw new Error('Unexpected request'); return new Promise((resolve, reject) => window.__pending.push({ url, options, resolve, reject })); }`,
      )
      const decision = (kind, extra = {}) => ({
        decision: {
          version: 1,
          field: "job_posting",
          outcome: { kind, ...extra },
        },
      })
      const feedback = () =>
        evaluate(
          "Array.from(document.querySelectorAll('[role=alert]')).map(el => el.textContent).join(' ')",
        )
      const unchangedProfile = await evaluate(savedProfileExpression)
      await fill("Software Engineer")
      await generate()
      assert.equal(
        await evaluate(
          "window.__pending[0].options.headers['X-TypeSafe-Api-Key']",
        ),
        "synthetic-typesafe",
      )
      await respond(
        decision("request_information", {
          needs: "responsibilities_or_qualifications",
        }),
      )
      assert.ok(
        (await feedback()).includes(
          locale === "en"
            ? "Please add some responsibilities or requirements so CareerOS can tailor your application to this opportunity."
            : "Adicione algumas responsabilidades ou requisitos para que o CareerOS possa adaptar sua candidatura a esta oportunidade.",
        ),
      )
      assert.equal(
        await evaluate("document.querySelector('main textarea').value"),
        "Software Engineer",
      )
      for (const kind of [
        "request_rephrasing",
        "reject_attack",
        "irrelevant",
        "unusable",
      ]) {
        await fill("Engineer building Java APIs " + kind)
        await generate()
        await respond(decision(kind))
        assert.ok((await feedback()).length > 20)
        assert.equal(
          await evaluate("window.__pending.length"),
          0,
          "decision cannot trigger drafting or retries",
        )
        if (kind === "request_rephrasing" || kind === "reject_attack") {
          assert.equal(
            await evaluate(
              "document.querySelector('main [data-checklist-state]').parentElement.parentElement.querySelector('button').disabled",
            ),
            true,
            "revision required",
          )
          await fill("Engineer building Java APIs " + kind + " ")
          assert.equal(
            await evaluate(
              "document.querySelector('main [data-checklist-state]').parentElement.parentElement.querySelector('button').disabled",
            ),
            true,
            "whitespace is not revision",
          )
        }
        if (kind === "reject_attack") {
          await evaluate("document.querySelector('[role=alert] a').click()")
          assert.equal(
            await evaluate(
              "document.querySelector('#job-input-use-rule').open",
            ),
            true,
          )
        }
      }
      for (const reason of [
        "key",
        "rate_limit",
        "timeout",
        "outage",
        "invalid_output",
      ]) {
        await fill("Engineer building APIs " + reason)
        await generate()
        await respond(
          decision("service_failure", { reason }),
          reason === "key" ? 401 : 502,
        )
        assert.ok((await feedback()).length > 20)
        assert.equal(await evaluate("window.__pending.length"), 0)
        assert.equal(
          await evaluate("document.querySelector('main textarea').value"),
          "Engineer building APIs " + reason,
        )
      }
      // Keyboard correction and a deliberate service-failure retry, no automatic call.
      const submitTarget =
        "document.querySelector('main [data-checklist-state]').parentElement.parentElement.querySelector('button')"
      await keyboardFill(
        "Security engineer documents quoted override examples.",
      )
      await keyboardSubmit(submitTarget)
      await respond(decision("request_rephrasing"))
      await keyboardFill("Java developer. AWS required.")
      await keyboardSubmit(submitTarget)
      await respond(decision("service_failure", { reason: "timeout" }), 504)
      await pause(200)
      assert.equal(await evaluate("window.__pending.length"), 0)
      assert.equal(
        await evaluate("document.querySelector('main textarea').value"),
        "Java developer. AWS required.",
      )
      await keyboardSubmit(submitTarget)
      assert.equal(await evaluate("window.__pending.length"), 1)
      await respond(
        decision("request_information", {
          needs: "responsibilities_or_qualifications",
        }),
      )
      assert.equal(await evaluate(savedProfileExpression), unchangedProfile)
      // Short postings preserve explicit unknowns through qualification confirmation, Results and print.
      const shortDraft = {
        jobTitle: "Java developer",
        company: null,
        jobSummary: "AWS required.",
        resume: "## Technical Skills\n- Research\n- Product strategy",
        coverLetter: {
          greeting: "Dear hiring team,",
          body: "I bring research and product strategy experience.",
          closing: "Sincerely,",
        },
        applicationAnswers:
          "AWS is required. I have not supplied AWS experience.",
      }
      for (const unknownTitle of [false, true]) {
        await fill("Java developer. AWS required.")
        await generate()
        assert.equal(
          await evaluate(
            "JSON.parse(window.__pending[0].options.body).jobPosting",
          ),
          "Java developer. AWS required.",
        )
        await respond({
          gaps: [
            { kind: "skill", requirement: "AWS", details: "AWS required." },
          ],
        })
        assert.ok(
          await evaluate(
            "document.querySelector('[role=dialog]').textContent.includes('AWS')",
          ),
        )
        assert.equal(
          await evaluate(
            "document.querySelector('[data-requirement-state]').dataset.requirementState",
          ),
          "not_evidenced",
        )
        assert.ok(
          (
            await evaluate(
              "document.querySelector('[data-requirement-state]').textContent",
            )
          ).includes(
            locale === "en"
              ? "Not evidenced in your Profile"
              : "Sem evidência no seu Perfil",
          ),
        )
        await evaluate(
          "document.querySelector('[data-requirement-state] summary').click()",
        )
        assert.ok(
          (
            await evaluate(
              "document.querySelector('[data-requirement-state]').textContent",
            )
          ).includes("Harbor Works"),
        )
        await evaluate(
          "document.querySelector('[data-requirement-state]').scrollIntoView({ block: 'start' })",
        )
        assert.equal(
          await evaluate(
            "document.querySelector('[role=dialog]').scrollWidth <= document.querySelector('[role=dialog]').clientWidth",
          ),
          true,
        )
        await evaluate("document.querySelector('[role=dialog]').scrollTop = document.querySelector('[data-requirement-state]').offsetTop - document.querySelector('[role=dialog]').offsetTop")
        await pause(250)
        const evidenceShot = await call("Page.captureScreenshot", {
          format: "png",
        })
        writeFileSync(
          join(work, `requirement-evidence-${locale}-${width}.png`),
          Buffer.from(evidenceShot.data, "base64"),
        )
        // A negative answer must be usable without falsely confirming a qualification.
        await evaluate(
          "(() => { const el = document.querySelector('[role=dialog] textarea'); const set = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set; set.call(el, 'I have not used AWS.'); el.dispatchEvent(new Event('input', { bubbles: true })); })()",
        )
        // Keep the qualification unconfirmed; it must not become a candidate fact.
        await evaluate(
          "Array.from(document.querySelectorAll('[role=dialog] button')).at(-2).click()",
        )
        assert.deepEqual(
          await evaluate(
            "JSON.parse(window.__pending[0].options.body).qualificationAnswers",
          ),
          [{ requirement: "AWS", userContext: "I have not used AWS." }],
        )
        assert.deepEqual(
          await evaluate(
            "JSON.parse(window.__pending[0].options.body).confirmedQualifications",
          ),
          [],
        )
        await respond({
          ...shortDraft,
          jobTitle: unknownTitle ? null : shortDraft.jobTitle,
        })
        await until("!!document.querySelector('.results-page')")
        const unknownLabel = locale === "en" ? "Not provided" : "Não informado"
        assert.ok(
          (
            await evaluate(
              "document.querySelector('.results-content').textContent",
            )
          ).includes(unknownLabel),
        )
        assert.equal(
          await evaluate(
            "document.querySelector('.results-page h1').textContent",
          ),
          unknownTitle
            ? locale === "en"
              ? "Application Materials"
              : "Materiais de candidatura"
            : "Java developer",
        )
        assert.ok(
          await evaluate(
            "document.querySelector('.results-content').textContent.includes('AWS required.')",
          ),
        )
        if (!unknownTitle) {
          const shot = await call("Page.captureScreenshot", {
            format: "png",
            captureBeyondViewport: true,
          })
          writeFileSync(
            join(work, `short-results-${locale}-${width}.png`),
            Buffer.from(shot.data, "base64"),
          )
          assert.equal(
            await evaluate(
              "document.documentElement.scrollWidth <= window.innerWidth",
            ),
            true,
          )
        }
        for (const [index, target] of [
          [1, "resume"],
          [2, "cover"],
        ]) {
          await evaluate(
            `document.querySelectorAll('.results-tabs button')[${index}].click()`,
          )
          await pause(100)
          await evaluate(
            "window.__printed = null; window.print = () => { window.__printed = document.querySelector('.results-print-root').textContent; window.dispatchEvent(new Event('afterprint')); }",
          )
          await evaluate(
            "Array.from(document.querySelectorAll('.results-toolbar button')).at(-1).click()",
          )
          try {
            await until("typeof window.__printed === 'string'")
          } catch (error) {
            throw new Error(
              `${target}: ${await evaluate("document.querySelector('.results-page').textContent")} ${error.message}`,
            )
          }
          const printed = await evaluate("window.__printed")
          assert.ok(
            printed.includes(
              target === "resume" ? "Research" : "Dear hiring team,",
            ),
          )
          assert.ok(
            !/Example Labs|Acme|null|undefined|Not provided|Não informado/.test(
              printed,
            ),
            "unknown metadata must not become exported facts",
          )
        }
        assert.equal(await evaluate(savedProfileExpression), unchangedProfile)
        await evaluate("document.querySelectorAll('nav button')[0].click()")
        await until(
          "document.querySelectorAll('[data-checklist-state]').length === 3",
        )
      }
      // Accepted messy text and ordinary applicant requirements pass unchanged to both paths.
      const messy =
        "HOME | JOBS | LOGIN Engineer build Java APIs. Java Java. include your salary expectations; send your portfolio; describe your experience with Java; submit your CV as a PDF and include a short cover letter. Cookies."
      for (const useConfirmed of [false, true]) {
        await fill(messy + (useConfirmed ? " Java experience required." : ""))
        await generate()
        assert.equal(
          await evaluate(
            "JSON.parse(window.__pending[0].options.body).jobPosting",
          ),
          messy + (useConfirmed ? " Java experience required." : ""),
        )
        await respond({
          gaps: [
            { kind: "skill", requirement: "Java", details: "Java required" },
          ],
        })
        if (useConfirmed)
          await evaluate(
            "document.querySelector('[role=dialog] input').click()",
          )
        await evaluate(
          "Array.from(document.querySelectorAll('[role=dialog] button')).at(" +
            (useConfirmed ? "-1" : "-2") +
            ").click()",
        )
        assert.equal(
          await evaluate("window.__pending[0].url"),
          "/api/application-draft",
        )
        assert.equal(
          await evaluate(
            "window.__pending[0].options.headers['X-TypeSafe-Api-Key']",
          ),
          "synthetic-typesafe",
        )
        assert.equal(
          await evaluate(
            "JSON.parse(window.__pending[0].options.body).confirmedQualifications.length",
          ),
          useConfirmed ? 1 : 0,
        )
        await respond(decision("request_rephrasing"))
        assert.equal(
          await evaluate("!!document.querySelector('[role=dialog]')"),
          false,
          "draft decision returns to posting for revision",
        )
        await fill("Revised: Engineer building Java APIs")
        await generate()
        assert.equal(
          await evaluate("window.__pending[0].url"),
          "/api/qualification-gaps",
          "changed input must check qualifications again",
        )
        await respond(
          decision("request_information", {
            needs: "responsibilities_or_qualifications",
          }),
        )
      }
      assert.equal(await evaluate(savedProfileExpression), unchangedProfile)
      await fill(
        "Product lead with research and strategy experience needed for a service team.",
      )
      assert.deepEqual(await states(), ["ready", "ready", "ready"])
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
      assert.deepEqual(await states(), ["ready", "ready", "ready"])
      await respond({
        gaps: [
          {
            kind: "skill",
            requirement: "Synthetic qualification",
            details: "Confirm your actual experience.",
          },
        ],
      })
      assert.deepEqual(await states(), ["ready", "ready", "ready"])
      await evaluate("document.querySelector('[role=dialog] input').click()")
      await evaluate(
        "Array.from(document.querySelectorAll('[role=dialog] button')).at(-1).click()",
      )
      assert.deepEqual(await states(), ["ready", "ready", "ready"])
      await respond({ error: "outage" }, 503)
      assert.deepEqual(await states(), ["ready", "ready", "ready"])
      assert.equal(
        await evaluate(
          "!!document.querySelector('[role=dialog] [role=alert]')",
        ),
        true,
      )
      await evaluate("document.querySelector('[role=dialog] input').click()")
      assert.equal(
        await evaluate(
          "!!document.querySelector('[role=dialog] [role=alert]')",
        ),
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
          "capacity",
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
      assert.deepEqual(await states(), ["ready", "ready", "ready"])
      await generate()
      await evaluate(
        "window.__pending.shift().reject(new DOMException('Synthetic browser timeout', 'TimeoutError'))",
      )
      await until("document.querySelector('[role=alert]') !== null")
      // Exact originals invalidate work even when shared preparation would match.
      const rawPosting = "# Role\nBuild  Java APIs.\nAWS required."
      await fill(rawPosting)
      await generate()
      assert.equal(
        await evaluate(
          "JSON.parse(window.__pending[0].options.body).jobPosting",
        ),
        rawPosting,
      )
      await fill("# Role\nBuild Java APIs.\nAWS required.")
      await respond({ gaps: [] })
      assert.equal(
        await evaluate("window.__pending.length"),
        0,
        "format-equivalent changed original must not draft",
      )
      assert.equal(
        await evaluate("document.querySelector('main textarea').value"),
        "# Role\nBuild Java APIs.\nAWS required.",
      )
      // Failure preserves exact textarea spacing and offers an actionable localized correction.
      await fill(rawPosting)
      await generate()
      await respond({ error: "capacity" }, 502)
      assert.ok(
        (await feedback()).includes(
          locale === "en" ? "Try a smaller portion" : "Tente um trecho menor",
        ),
      )
      assert.equal(
        await evaluate("document.querySelector('main textarea').value"),
        rawPosting,
      )
      assert.equal(await evaluate(savedProfileExpression), unchangedProfile)
      await generate()
      await respond({
        gaps: [{ kind: "skill", requirement: "AWS", details: "AWS required." }],
      })
      await evaluate("document.querySelector('[role=dialog] button').click()")
      assert.equal(
        await evaluate("document.querySelector('main textarea').value"),
        rawPosting,
        "cancellation preserves exact original",
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
      assert.deepEqual(await states(), ["ready", "ready", "ready"])
      // Inspect reusable context before tailoring, including legitimate applicant requests.
      const contextPosting =
        "Java developer. AWS required.\nSend salary expectations and portfolio as PDF."
      await fill(contextPosting)
      await generate()
      assert.equal(
        await evaluate(
          "JSON.parse(window.__pending[0].options.body).understandJob",
        ),
        true,
      )
      await respond({ gaps: [] }, 200, true)
      const review = await evaluate(
        "document.querySelector('[role=dialog]').textContent",
      )
      assert.ok(
        review.includes(
          locale === "en"
            ? "Review job understanding"
            : "Revisar entendimento da vaga",
        ),
      )
      assert.ok(
        review.includes("AWS required.") &&
          review.includes("Send salary expectations and portfolio as PDF."),
      )
      assert.ok(
        review.includes(
          locale === "en"
            ? "Importance not specified"
            : "Importância não especificada",
        ),
      )
      assert.equal(
        await evaluate("window.__pending.length"),
        0,
        "inspection requires no provider call",
      )
      await evaluate(
        "document.querySelector('[role=dialog] details summary').click()",
      )
      assert.equal(
        await evaluate(
          "document.querySelector('[role=dialog] blockquote').textContent",
        ),
        "Java developer",
      )
      assert.equal(
        await evaluate(
          "document.documentElement.scrollWidth <= window.innerWidth",
        ),
        true,
      )
      const reviewShot = await call("Page.captureScreenshot", {
        format: "png",
        captureBeyondViewport: true,
      })
      writeFileSync(
        join(work, `job-context-${locale}-${width}.png`),
        Buffer.from(reviewShot.data, "base64"),
      )
      await evaluate(
        "Array.from(document.querySelectorAll('[role=dialog] button')).at(-2).click()",
      )
      const draftInput = await evaluate(
        "JSON.parse(window.__pending[0].options.body)",
      )
      assert.equal(draftInput.jobPosting, contextPosting)
      assert.deepEqual(draftInput.jobContext, jobContextFixture(contextPosting))
      await respond({ error: "job_context_stale" }, 502)
      assert.equal(
        await evaluate("!!document.querySelector('[role=dialog]')"),
        false,
      )
      assert.ok(
        (await feedback()).includes(
          locale === "en" ? "Analyze again" : "Analise novamente",
        ),
      )
      assert.equal(
        await evaluate("window.__pending.length"),
        0,
        "unverifiable context must not automatically retry",
      )
      assert.equal(
        await evaluate("document.querySelector('main textarea').value"),
        contextPosting,
      )
      await generate()
      await respond({ gaps: [] }, 200, true)
      await evaluate("document.querySelector('[role=dialog] button').click()")
      await fill(contextPosting + " ")
      assert.equal(
        await evaluate("!!document.querySelector('main blockquote')"),
        false,
        "changed originals invalidate inspected context",
      )
      await fill(
        "Harbor Works seeks a Product Lead to lead product work. Research and product strategy required.",
      )
      await generate()
      await respond({ gaps: [] })
      assert.deepEqual(await states(), ["ready", "ready", "ready"])
      await respond(materials)
      await until(
        "document.querySelectorAll('[data-checklist-state]').length === 0",
      )
      assert.equal(
        await evaluate(
          "document.querySelector('.results-page h1').textContent",
        ),
        "Product Lead",
      )
      assert.ok(
        await evaluate(
          "document.querySelector('.results-content').textContent.includes('Harbor Works')",
        ),
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
      if (width === 390) {
        for (const matchState of [
          "supported",
          "partially_supported",
          "not_evidenced",
          "needs_clarification",
        ]) {
          await fill("Java developer. AWS required.")
          await generate()
          const gaps =
            matchState === "supported"
              ? []
              : [
                  {
                    kind: "skill",
                    requirement: "AWS",
                    details: "AWS required.",
                  },
                ]
          await respond({ gaps }, 200, true, {
            state: matchState,
            complete: matchState !== "needs_clarification",
          })
          assert.equal(
            await evaluate(
              "document.querySelector('[data-requirement-state]').dataset.requirementState",
            ),
            matchState,
          )
          if (matchState === "needs_clarification") {
            assert.ok(
              (
                await evaluate(
                  "document.querySelector('[data-requirement-state]').textContent",
                )
              ).includes(
                locale === "en"
                  ? "Incomplete analysis: 1"
                  : "Análise incompleta: 1",
              ),
            )
            assert.ok(
              (
                await evaluate(
                  "document.querySelector('[data-requirement-state]').textContent",
                )
              ).includes("Have you used AWS in a project?"),
            )
          }
          await evaluate(
            "document.querySelector('[role=dialog] button').click()",
          )
        }
        // Approved evidence in every sparse section can start Apply, with all
        // key/posting/in-progress guards still active. No provider is contacted.
        for (const section of [
          "projects",
          "education",
          "certifications",
          "languages",
          "tools",
          "competencies",
          "skills",
          "experience",
        ]) {
          const sparse = Object.fromEntries(
            Object.entries(repo).map(([key, value]) => [
              key,
              Array.isArray(value) ? [] : "",
            ]),
          )
          sparse[section] = repo[section] || "System design"
          await evaluate(`(async () => {
            const { migrateProfile } = await import('/src/lib/profileDocument.ts');
            const doc = migrateProfile(${JSON.stringify(sparse)}, crypto.randomUUID());
            doc.facts.forEach(f => { f.approval = 'approved'; });
            localStorage.setItem('careeros_profile_v2', JSON.stringify(doc));
            location.reload();
          })()`)
          await until(
            "document.readyState === 'complete' && !!document.querySelector('header button')",
          )
          await clickApply()
          await evaluate(
            "window.__pending = []; window.fetch = (url, options) => new Promise(resolve => window.__pending.push({url, options, resolve}))",
          )
          const submit =
            "document.querySelector('main [data-checklist-state]').parentElement.parentElement.querySelector('button')"
          assert.equal(
            await evaluate(`${submit}.disabled`),
            true,
            section + ": posting guard",
          )
          await fill("Java required.")
          assert.equal(
            await evaluate(`${submit}.disabled`),
            false,
            section + ": sparse eligibility",
          )
          await generate()
          assert.equal(
            await evaluate("window.__pending.length"),
            1,
            section + ": qualification request",
          )
          assert.equal(
            await evaluate(`${submit}.disabled`),
            true,
            section + ": in-progress guard",
          )
          const projected = await evaluate(
            "JSON.parse(window.__pending[0].options.body).profileEvidence",
          )
          assert.ok(projected.facts.length > 0, section)
          assert.ok(
            !projected.facts.some((f) =>
              ["email", "currentSalary", "careerGoals"].includes(f.field),
            ),
          )
        }
      }
      console.log(`PASS Apply controlled workflow ${locale} ${width}px`)
    }
  }
  console.log("Screenshots:", work)
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}
