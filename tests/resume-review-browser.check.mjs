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
const work = mkdtempSync(join(tmpdir(), "careeros-resume-review-"))
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
  const button = (label) =>
    `Array.from(document.querySelectorAll('.resume-review button')).find(b => b.textContent.trim() === ${JSON.stringify(label)})`
  await clickApply()
  const originalProfile = await evaluate(savedProfileExpression)
  for (const locale of ["en", "pt-BR"]) {
    for (const width of [1440, 390]) {
      await call("Emulation.setDeviceMetricsOverride", {
        width,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false,
      })
      await evaluate(
        `{ const el=document.querySelector('select');el.value=${JSON.stringify(locale)};el.dispatchEvent(new Event('change',{bubbles:true})) }`,
      )
      await pause(50)
      await evaluate("document.querySelectorAll('nav button')[0].click()")
      await until("!!document.querySelector('main textarea')")
      await evaluate(
        `window.__pending=[];window.fetch=(url,options)=>new Promise((resolve,reject)=>window.__pending.push({url,options,resolve,reject}))`,
      )
      await evaluate(
        `{ const el=document.querySelector('main select');el.value=${JSON.stringify(locale)};el.dispatchEvent(new Event('change',{bubbles:true})) }`,
      )
      await pause(50)
      await fill("Java developer. AWS required.")
      await generate()
      await until("window.__pending.length === 1")
      await respond({ gaps: [] })
      await until(
        "!!document.querySelector('[role=dialog]') || window.__pending.length === 1",
      )
      if (await evaluate("!!document.querySelector('[role=dialog]')")) {
        await evaluate(
          "Array.from(document.querySelectorAll('[role=dialog] button')).at(-2).click()",
        )
      }
      await until("window.__pending.length === 1")
      const input = await evaluate(
        "JSON.parse(window.__pending[0].options.body)",
      )
      assert.equal(input.reviewResume, true)
      const fixture = reviewedDraftFixture(
        {
          ...materials,
          coverLetter:
            locale === "pt-BR"
              ? {
                  greeting: "Prezada equipe,",
                  body: "Eu uso Research.",
                  closing: "Atenciosamente,",
                }
              : materials.coverLetter,
          resume: "## Technical Skills\n- Research\n- Invented result of 40%",
        },
        input,
      )
      fixture.resumeReview.claims[1].state = "unsupported"
      fixture.resumeReview.claims[1].concerns = ["invented_number"]
      const bodyClaim = fixture.artifactReview.claims.find(
        (c) => c.field === "body",
      )
      bodyClaim.state = "unsupported"
      bodyClaim.concerns = ["stronger_claim"]
      const answerClaim = fixture.artifactReview.claims.find(
        (c) => c.field === "applicationAnswers",
      )
      answerClaim.state = "unsupported"
      answerClaim.concerns = ["Required answer omits Java."]
      await respond(fixture)
      await until("!!document.querySelector('.results-tabs')")
      await until("!!document.querySelector('.artifact-review')")
      const artifactButton = (label) =>
        `Array.from(document.querySelectorAll('.artifact-review button')).find(b=>b.textContent.trim()===${JSON.stringify(label)})`
      const acceptArtifacts =
        locale === "en"
          ? "Accept application materials"
          : "Aceitar materiais da candidatura"
      assert.equal(
        await evaluate(artifactButton(acceptArtifacts) + ".disabled"),
        true,
      )
      await evaluate(
        "document.querySelectorAll('.results-tabs button')[2].click()",
      )
      assert.equal(
        await evaluate(
          "Array.from(document.querySelectorAll('.results-other-actions button')).every(b=>b.disabled)",
        ),
        true,
      )
      const correctArtifact =
        locale === "en" ? "Supply correction" : "Fornecer correção"
      const useArtifact =
        locale === "en"
          ? "Use my statement as evidence"
          : "Usar minha afirmação como evidência"
      for (const replacement of [
        locale === "en"
          ? "I use Research for personal projects."
          : "Eu uso Research em projetos pessoais.",
        locale === "en" ? "I use Java." : "Eu uso Java.",
      ]) {
        await evaluate(artifactButton(correctArtifact) + ".click()")
        await evaluate(
          `{const el=document.querySelector('.artifact-review textarea');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,${JSON.stringify(replacement)});el.dispatchEvent(new Event('input',{bubbles:true}))}`,
        )
        await pause(40)
        await evaluate(artifactButton(useArtifact) + ".click()")
      }
      await until(
        artifactButton(acceptArtifacts) +
          " && !" +
          artifactButton(acceptArtifacts) +
          ".disabled",
      )
      await evaluate(artifactButton(acceptArtifacts) + ".click()")
      await evaluate(
        "window.__artifactCopy='';Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async t=>window.__artifactCopy=t}})",
      )
      await evaluate(
        "document.querySelector('.results-other-actions button').click()",
      )
      const completedCover = await evaluate(
        "document.querySelector('.results-cover-print').textContent",
      )
      assert.equal(await evaluate("window.__artifactCopy"), completedCover)
      assert.ok(
        completedCover.includes(
          locale === "en" ? "personal projects" : "projetos pessoais",
        ),
      )
      await evaluate(
        "window.__printed=false;window.print=()=>window.__printed=true;Array.from(document.querySelectorAll('.results-other-actions button')).find(b=>b.textContent.includes('PDF')).click()",
      )
      await until("window.__printed===true")
      assert.equal(
        await evaluate(
          "document.querySelector('.results-print-root').textContent",
        ),
        completedCover,
      )
      const coverPdf = await call("Page.printToPDF", {
        printBackground: true,
        preferCSSPageSize: true,
      })
      writeFileSync(
        join(work, `cover-${locale}-${width}.pdf`),
        Buffer.from(coverPdf.data, "base64"),
      )
      await evaluate("window.dispatchEvent(new Event('afterprint'))")
      const coverShot = await call("Page.captureScreenshot", {
        format: "png",
        captureBeyondViewport: true,
      })
      writeFileSync(
        join(work, `cover-${locale}-${width}.png`),
        Buffer.from(coverShot.data, "base64"),
      )
      await evaluate(
        "document.querySelectorAll('.results-tabs button')[3].click()",
      )
      await evaluate(
        "document.querySelector('.results-other-actions button').click()",
      )
      assert.equal(
        await evaluate("window.__artifactCopy"),
        locale === "en" ? "I use Java." : "Eu uso Java.",
      )
      await evaluate(
        "document.querySelectorAll('.results-tabs button')[1].click()",
      )
      await until("!!document.querySelector('.resume-review')")
      assert.equal(
        await evaluate("!!document.querySelector('.cv-paper')"),
        false,
      )
      assert.equal(
        await evaluate(
          "Array.from(document.querySelectorAll('.results-resume-actions button')).every(b=>b.disabled)",
        ),
        true,
      )
      const pendingShot = await call("Page.captureScreenshot", {
        format: "png",
        captureBeyondViewport: true,
      })
      writeFileSync(
        join(work, `pending-${locale}-${width}.png`),
        Buffer.from(pendingShot.data, "base64"),
      )
      const accept = locale === "en" ? "Accept resume" : "Aceitar currículo"
      const correct =
        locale === "en" ? "Supply correction" : "Fornecer correção"
      const remove = locale === "en" ? "Remove statement" : "Remover afirmação"
      assert.equal(await evaluate(button(accept) + ".disabled"), true)
      await evaluate(
        "document.querySelector('.resume-review details summary').click()",
      )
      assert.ok(
        (
          await evaluate(
            "document.querySelector('.resume-review details').textContent",
          )
        ).includes("Research"),
      )
      await evaluate(
        `Array.from(document.querySelectorAll('.resume-review li > div button')).filter(b=>b.textContent.trim()===${JSON.stringify(correct)})[1].click()`,
      )
      await until("!!document.querySelector('.resume-review textarea')")
      await evaluate(
        `{const el=document.querySelector('.resume-review textarea');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,'I use Research for personal projects.');el.dispatchEvent(new Event('input',{bubbles:true}))}`,
      )
      await pause(50)
      await evaluate(
        button(
          locale === "en"
            ? "Use my statement as evidence"
            : "Usar minha afirmação como evidência",
        ) + ".click()",
      )
      await until(button(accept) + " && !" + button(accept) + ".disabled")
      await evaluate(button(accept) + ".click()")
      await until("!!document.querySelector('.cv-paper')")
      assert.ok(
        (
          await evaluate(
            "document.querySelector('.cv-paper-content').textContent",
          )
        ).includes("I use Research for personal projects."),
      )
      assert.ok(
        !(
          await evaluate(
            "document.querySelector('.cv-paper-content').textContent",
          )
        ).includes("40%"),
      )
      await evaluate(
        "window.__copied='';Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async t=>window.__copied=t}})",
      )
      await evaluate(
        "document.querySelector('.results-resume-actions button').click()",
      )
      await until(
        "window.__copied.includes('I use Research for personal projects.')",
      )
      const before = await evaluate(
        "document.querySelector('.cv-paper-content').textContent",
      )
      await evaluate(
        button(locale === "en" ? "Review wording" : "Revisar texto") +
          ".click()",
      )
      await evaluate(button(remove) + ".click()")
      // Pending edits leave the last accepted content intact; cancellation restores it.
      assert.equal(
        await evaluate(
          "document.querySelector('.cv-paper-content').textContent",
        ),
        before,
      )
      await evaluate(
        button(locale === "en" ? "Cancel" : "Cancelar") + ".click()",
      )
      assert.equal(
        await evaluate(
          "document.querySelector('.cv-paper-content').textContent",
        ),
        before,
      )
      await evaluate(
        "window.__printed=false;window.print=()=>{window.__printed=true}",
      )
      await evaluate(
        "Array.from(document.querySelectorAll('.results-resume-actions button')).find(b=>b.textContent.includes('PDF')).click()",
      )
      await until("window.__printed === true")
      const printed = await evaluate(
        "document.querySelector('.results-print-root').textContent",
      )
      assert.ok(
        printed.includes("I use Research for personal projects.") &&
          !printed.includes("40%"),
      )
      const pdf = await call("Page.printToPDF", {
        printBackground: true,
        preferCSSPageSize: true,
      })
      writeFileSync(
        join(work, `resume-${locale}-${width}.pdf`),
        Buffer.from(pdf.data, "base64"),
      )
      await evaluate("window.dispatchEvent(new Event('afterprint'))")
      assert.equal(
        await evaluate(
          "document.documentElement.scrollWidth <= window.innerWidth",
        ),
        true,
      )
      const shot = await call("Page.captureScreenshot", {
        format: "png",
        captureBeyondViewport: true,
      })
      writeFileSync(
        join(work, `review-${locale}-${width}.png`),
        Buffer.from(shot.data, "base64"),
      )
      assert.equal(await evaluate(savedProfileExpression), originalProfile)
      // A failed generation keeps the previous accepted resume accessible.
      await evaluate("document.querySelectorAll('nav button')[0].click()")
      await until("!!document.querySelector('main textarea')")
      await generate()
      await until("window.__pending.length === 1")
      await respond({ gaps: [] })
      await until("window.__pending.length === 1")
      await respond({ error: "invalid_output" }, 502)
      await until("!!document.querySelector('[role=alert]')")
      await evaluate("document.querySelectorAll('nav button')[3].click()")
      await until("!!document.querySelector('.results-tabs')")
      await evaluate(
        "document.querySelectorAll('.results-tabs button')[1].click()",
      )
      await until("!!document.querySelector('.cv-paper')")
      assert.equal(
        await evaluate(
          "document.querySelector('.cv-paper-content').textContent",
        ),
        before,
      )
      assert.equal(
        await evaluate("window.__pending.length"),
        0,
        "failure must not retry automatically",
      )
      // A successful replacement stays a draft; cancellation restores the
      // complete previous accepted result, not just the resume string.
      await evaluate("document.querySelectorAll('nav button')[0].click()")
      await until("!!document.querySelector('main textarea')")
      await generate()
      await until("window.__pending.length === 1")
      await respond({ gaps: [] })
      await until("window.__pending.length === 1")
      await respond({
        ...materials,
        jobTitle: "Replacement draft",
        resume: "## Technical Skills\n- Research",
      })
      await until("!!document.querySelector('.results-tabs')")
      await evaluate(
        "document.querySelectorAll('.results-tabs button')[1].click()",
      )
      await until("!!document.querySelector('.resume-review')")
      const restore =
        locale === "en"
          ? "Restore previous accepted draft"
          : "Restaurar rascunho aceito anterior"
      assert.equal(
        await evaluate(
          `Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()===${JSON.stringify(restore)})`,
        ),
        true,
      )
      await evaluate(
        `Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(restore)}).click()`,
      )
      await until("!!document.querySelector('.cv-paper')")
      assert.equal(
        await evaluate(
          "document.querySelector('.cv-paper-content').textContent",
        ),
        before,
      )
      assert.ok(
        !(await evaluate("document.querySelector('h1').textContent")).includes(
          "Replacement draft",
        ),
      )
    }
  }
  // Actual accepted-content overflow is measured before opening the print dialog.
  await evaluate("document.querySelectorAll('nav button')[0].click()")
  await until("!!document.querySelector('main textarea')")
  await generate()
  await until("window.__pending.length === 1")
  await respond({ gaps: [] })
  await until("window.__pending.length === 1")
  await respond({
    ...materials,
    resume:
      "## Professional Summary\n" +
      "Long draft detail. ".repeat(450) +
      "END-MARKER",
  })
  await until("!!document.querySelector('.results-tabs')")
  await evaluate(
    "Array.from(document.querySelectorAll('.artifact-review button')).find(b=>b.textContent.trim()==='Aceitar materiais da candidatura').click()",
  )
  await evaluate("document.querySelectorAll('.results-tabs button')[1].click()")
  await until("!!document.querySelector('.resume-review')")
  await evaluate(button("Aceitar currículo") + ".click()")
  await until("!!document.querySelector('.cv-paper-content')")
  assert.ok(
    (
      await evaluate("document.querySelector('.cv-paper-content').textContent")
    ).includes("END-MARKER"),
  )
  await evaluate(
    "window.__printed=false;Array.from(document.querySelectorAll('.results-resume-actions button')).find(b=>b.textContent.includes('PDF')).click()",
  )
  await until(
    "!!document.querySelector('.results-toolbar-advice [role=alert]')",
  )
  assert.equal(
    await evaluate("window.__printed"),
    false,
    "overflowing accepted content must not open print",
  )
  // Stale Profile keeps the source snapshot but blocks copy and PDF.
  await evaluate("document.querySelectorAll('nav button')[1].click()")
  await until("!!document.querySelector('aside nav')")
  await evaluate("document.querySelector('aside nav button').click()")
  await until("!!document.querySelector('main input')")
  await evaluate(
    "{const el=document.querySelector('main input');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(el,'Updated identity');el.dispatchEvent(new Event('input',{bubbles:true}))}",
  )
  await pause(100)
  await evaluate("document.querySelectorAll('nav button')[3].click()")
  await until("!!document.querySelector('.results-tabs')")
  await evaluate("document.querySelectorAll('.results-tabs button')[1].click()")
  await until("!!document.querySelector('.resume-review [role=alert]')")
  assert.equal(
    await evaluate(
      "Array.from(document.querySelectorAll('.results-resume-actions button')).every(b=>b.disabled)",
    ),
    true,
  )
  assert.ok(
    (
      await evaluate("document.querySelector('.cv-paper-content').textContent")
    ).includes("END-MARKER"),
  )
  await evaluate("document.querySelectorAll('.results-tabs button')[2].click()")
  await until("!!document.querySelector('.artifact-review [role=alert]')")
  assert.equal(
    await evaluate(
      "Array.from(document.querySelectorAll('.results-other-actions button')).every(b=>b.disabled)",
    ),
    true,
  )
  assert.ok(
    (
      await evaluate(
        "document.querySelector('.results-cover-print').textContent",
      )
    ).includes("I led useful service work"),
  )
  console.log(
    `Resume and application review browser checks passed (EN/PT, 390/1440px). Artifacts: ${work}`,
  )
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}
