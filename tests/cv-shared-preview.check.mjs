// Browser regression for the Profile CV and an Application Draft resume.
// Uses synthetic API responses; no provider request is made.
import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { execFileSync } from "node:child_process"
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-cv-preview-"))
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
    `localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_apikey', 'synthetic-test-key'); location.reload()`,
  )
  await until(
    "document.readyState === 'complete' && !!document.querySelector('header button')",
  )
  await evaluate("document.querySelector('header button:last-child').click()")
  await until("document.querySelectorAll('nav button').length === 4")
  await evaluate("document.querySelectorAll('nav button')[2].click()")
  await until("!!document.querySelector('.cv-paper-content')")
  const base = await evaluate(
    "Array.from(document.querySelectorAll('.cv-paper-content section h3')).map(el => el.textContent.trim())",
  )
  await evaluate("document.querySelectorAll('nav button')[1].click()")
  await until("!!document.querySelector('aside nav')")
  assert.equal(
    await evaluate("Array.from(document.querySelectorAll('aside nav button')).filter(el => /Skills|Competencies|Tools & Tech/.test(el.textContent)).length"),
    1,
    "Profile should group skills, competencies, tools and technology in one section",
  )
  await evaluate("Array.from(document.querySelectorAll('aside nav button')).find(el => el.textContent.includes('Skills')).click()")
  assert.equal(
    await evaluate("document.querySelectorAll('main textarea').length >= 3"),
    true,
    "grouped Profile section should keep all three editable fields",
  )
  if (process.env.CV_PREVIEW_KEEP) {
    const shot = await call("Page.captureScreenshot", {
      format: "png",
      captureBeyondViewport: true,
    })
    writeFileSync(
      join(work, "profile-cv.png"),
      Buffer.from(shot.data, "base64"),
    )
  }
  await evaluate("document.querySelectorAll('header nav button')[0].click()")
  await until("!!document.querySelector('textarea')")
  await evaluate(
    `window.fetch = async (url) => new Response(JSON.stringify(String(url).includes('qualification-gaps') ? { gaps: [] } : ${JSON.stringify(materials)}), { status: 200, headers: { 'Content-Type': 'application/json' } }); const el = document.querySelector('textarea'); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, 'A long synthetic job posting for a product lead at Harbor Works.'); el.dispatchEvent(new Event('input', { bubbles: true }))`,
  )
  await until(
    "Array.from(document.querySelectorAll('button')).some(el => /Generate/i.test(el.textContent) && !el.disabled)",
  )
  await evaluate(
    "Array.from(document.querySelectorAll('button')).find(el => /Generate/i.test(el.textContent) && !el.disabled).click()",
  )
  await until(
    "document.querySelectorAll('nav button').length === 4 && document.body.textContent.includes('Product Lead')",
  )
  await evaluate(
    "Array.from(document.querySelectorAll('button')).find(el => el.textContent.trim() === 'Résumé').click()",
  )
  const generated = await evaluate(
    "({ paper: !!document.querySelector('.cv-paper'), headings: Array.from(document.querySelectorAll('.cv-paper-content section h3')).map(el => el.textContent.trim()), rawMarkdown: !!document.querySelector('pre') })",
  )
  assert.ok(generated.paper, "Apply résumé should use the CV A4 paper preview")
  assert.equal(
    generated.rawMarkdown,
    false,
    "Apply résumé should render Markdown as document sections",
  )
  assert.deepEqual(
    base,
    generated.headings,
    "both CVs should share section order and headings",
  )
  assert.deepEqual(base, [
    "Professional Summary",
    "Technical Skills",
    "Professional Experience",
    "Education",
    "Certifications",
    "Languages",
  ])
  assert.equal(
    await evaluate(
      "document.querySelectorAll('.cv-paper-content header').length",
    ),
    1,
  )
  assert.equal(
    await evaluate(
      "document.querySelector('.cv-paper-content').textContent.match(/avery@example\\.com/g)?.length",
    ),
    1,
  )
  assert.equal(
    await evaluate("document.querySelector('.cv-paper-content header h2')?.textContent.trim()"),
    "Avery Morgan",
    "Results should recover a generated name when Profile has none",
  )
  assert.ok(
    await evaluate("document.querySelector('.cv-paper-content').textContent.includes('my_variable')"),
    "Markdown rendering must preserve literal professional text",
  )
  await evaluate("{ const el = document.querySelector('.cv-font-controls input'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, '16'); el.dispatchEvent(new Event('input', { bubbles: true })) }")
  await until("getComputedStyle(document.querySelector('.cv-paper-content p')).fontSize === '16px'")
  await evaluate("window.print = () => { window.__printCalled = true }")
  const originalTitle = await evaluate("document.title")
  assert.equal(await evaluate("document.querySelectorAll('.results-page-header button').length"), 1, "New Application should be the only header action")
  assert.equal(await evaluate("document.querySelectorAll('.results-resume-actions button').length"), 1, "résumé should offer PDF instead of copy text")
  assert.equal(await evaluate("getComputedStyle(document.querySelector('.results-resume-actions button')).backgroundColor"), "rgb(200, 16, 46)", "PDF action should use the red accent")
  await evaluate("{ const badge = document.createElement('iframe'); badge.id = 'nl-badge-frame'; badge.srcdoc = '<body>Powered by Netlify</body>'; badge.style.cssText = 'position:fixed;bottom:0;right:0;width:194px;height:64px;border:0'; document.body.append(badge) }")
  await until("document.querySelector('#nl-badge-frame')?.contentDocument?.body?.textContent.includes('Powered by Netlify')")
  await evaluate("document.querySelector('.results-resume-actions button').click()")
  await until("window.__printCalled === true && document.body.classList.contains('results-print-resume')")
  assert.equal(await evaluate("document.querySelectorAll('.results-print-root > .cv-paper').length"), 1, "PDF export should print a standalone résumé")
  assert.equal(await evaluate("getComputedStyle(document.querySelector('.results-print-root .cv-paper-content p')).fontSize"), "16px", "print clone preserves selected type")
  const resumePdf = join(work, "results-resume.pdf")
  writeFileSync(resumePdf, Buffer.from((await call("Page.printToPDF", { printBackground: true, preferCSSPageSize: true })).data, "base64"))
  assert.match(execFileSync("pdfinfo", [resumePdf], { encoding: "utf8" }), /Pages:\s+1\b[\s\S]*Page size:\s+59[45]\.\d+ x 841\.\d+ pts \(A4\)/)
  const resumePdfText = execFileSync("pdftotext", ["-raw", resumePdf, "-"], { encoding: "utf8" })
  assert.match(resumePdfText, /Avery Morgan/i)
  assert.match(resumePdfText, /Professional Summary/i)
  assert.doesNotMatch(resumePdfText, /Generated Application|Cover Letter|Application Q&A/i)
  assert.doesNotMatch(resumePdfText, /Powered by Netlify/i)
  await evaluate("window.dispatchEvent(new Event('afterprint')); window.__printCalled = false")
  assert.equal(await evaluate("document.querySelectorAll('.results-print-root').length"), 0, "PDF export should clean up its print document")
  assert.equal(await evaluate("document.title"), originalTitle, "PDF export should restore the page title")
  await evaluate("Array.from(document.querySelectorAll('button')).find(el => el.textContent.trim() === 'Cover Letter').click()")
  await until("document.querySelector('.results-other-actions')?.textContent.includes('Save as PDF')")
  assert.equal(await evaluate("document.querySelectorAll('.results-other-actions button').length"), 2, "cover letter should offer copy and PDF")
  assert.equal(await evaluate("getComputedStyle(document.querySelector('.results-other-actions button')).backgroundColor"), "rgb(17, 17, 16)", "Copy should have a black background")
  assert.equal(await evaluate("getComputedStyle(document.querySelector('.results-other-actions button')).color"), "rgb(255, 255, 255)", "Copy should have white text")
  assert.equal(await evaluate("getComputedStyle(document.querySelectorAll('.results-other-actions button')[1]).backgroundColor"), "rgb(200, 16, 46)", "cover PDF action should use the red accent")
  if (process.env.CV_PREVIEW_KEEP) {
    const shot = await call("Page.captureScreenshot", { format: "png", captureBeyondViewport: true })
    writeFileSync(join(work, "results-cover-actions.png"), Buffer.from(shot.data, "base64"))
  }
  await evaluate("Array.from(document.querySelectorAll('.results-other-actions button')).find(el => el.textContent.includes('PDF')).click()")
  await until("window.__printCalled === true && document.body.classList.contains('results-print-cover')")
  const coverPdf = join(work, "results-cover.pdf")
  writeFileSync(coverPdf, Buffer.from((await call("Page.printToPDF", { printBackground: true, preferCSSPageSize: true })).data, "base64"))
  assert.match(execFileSync("pdfinfo", [coverPdf], { encoding: "utf8" }), /Pages:\s+1\b[\s\S]*Page size:\s+59[45]\.\d+ x 841\.\d+ pts \(A4\)/)
  const coverPdfText = execFileSync("pdftotext", ["-raw", coverPdf, "-"], { encoding: "utf8" })
  assert.match(coverPdfText, /Dear team,[\s\S]*I led useful service work[\s\S]*Sincerely,/)
  assert.doesNotMatch(coverPdfText, /Professional Summary|Generated Application|Application Q&A/i)
  assert.doesNotMatch(coverPdfText, /Powered by Netlify/i)
  await evaluate("window.dispatchEvent(new Event('afterprint')); window.__printCalled = false")
  await evaluate("document.querySelector('.results-cover-print').textContent = 'Long letter detail. '.repeat(1800)")
  await evaluate("Array.from(document.querySelectorAll('.results-other-actions button')).find(el => el.textContent.includes('PDF')).click()")
  await until("document.querySelector('[role=alert]')?.textContent.includes('cover letter exceeds one A4 page')")
  assert.equal(await evaluate("window.__printCalled"), false, "overflowing cover letter should not open a two-page export")
  await evaluate(`document.querySelector('.results-cover-print').textContent = ${JSON.stringify("Dear team,\n\nI led useful service work at Harbor Works.\n\nSincerely,")}`)
  await evaluate("document.querySelector('#nl-badge-frame')?.remove()")
  await evaluate("Array.from(document.querySelectorAll('button')).find(el => el.textContent.trim() === 'Résumé').click()")
  await until("!!document.querySelector('.cv-paper-content')")
  if (process.env.CV_PREVIEW_KEEP) {
    const shot = await call("Page.captureScreenshot", {
      format: "png",
      captureBeyondViewport: true,
    })
    writeFileSync(join(work, "apply-cv.png"), Buffer.from(shot.data, "base64"))
  }
  await call("Emulation.setDeviceMetricsOverride", {
    width: 390,
    height: 844,
    deviceScaleFactor: 1,
    mobile: false,
  })
  await until("window.innerWidth === 390")
  await until(
    "document.querySelector('.cv-paper').getBoundingClientRect().width <= document.querySelector('.cv-preview-slot').getBoundingClientRect().width + 1",
  )
  if (process.env.CV_PREVIEW_KEEP) {
    const shot = await call("Page.captureScreenshot", {
      format: "png",
      captureBeyondViewport: true,
    })
    writeFileSync(
      join(work, "apply-cv-narrow.png"),
      Buffer.from(shot.data, "base64"),
    )
  }
  const narrow = await evaluate(
    "({ page: document.documentElement.scrollWidth, width: window.innerWidth, paper: document.querySelector('.cv-paper').getBoundingClientRect().width, slot: document.querySelector('.cv-preview-slot').getBoundingClientRect().width, wide: Array.from(document.querySelectorAll('body *')).filter(el => el.getBoundingClientRect().right > innerWidth + 1).slice(0, 8).map(el => [el.tagName, el.className, Math.round(el.getBoundingClientRect().right)]) })",
  )
  assert.equal(
    narrow.page <= narrow.width,
    true,
    `Apply preview must not cause horizontal page scroll at 390px: ${JSON.stringify(narrow)}`,
  )
  assert.equal(
    narrow.paper <= narrow.slot + 1,
    true,
    "A4 preview must scale to the narrow slot",
  )
  assert.equal(await evaluate("document.querySelector('.cv-preview-zoom')"), null)
  await evaluate(
    "{ const select = document.querySelector('select'); select.value = 'pt-BR'; select.dispatchEvent(new Event('change', { bubbles: true })) }",
  )
  await until(
    "document.querySelector('.cv-paper-content section h3')?.textContent.trim() === 'Resumo profissional'",
  )
  const generatedPt = await evaluate(
    "Array.from(document.querySelectorAll('.cv-paper-content section h3')).map(el => el.textContent.trim())",
  )
  assert.ok(
    await evaluate(
      "document.querySelector('.cv-paper-content').textContent.includes('Product leader focused on useful services.')",
    ),
    "locale change must preserve generated wording",
  )
  await evaluate("document.querySelectorAll('nav button')[2].click()")
  await until("!!document.querySelector('.cv-paper-content')")
  const basePt = await evaluate(
    "Array.from(document.querySelectorAll('.cv-paper-content section h3')).map(el => el.textContent.trim())",
  )
  assert.deepEqual(
    basePt,
    generatedPt,
    "Portuguese headings should match without translating professional text",
  )
  assert.ok(await evaluate("document.querySelector('.cv-paper').getBoundingClientRect().width <= document.querySelector('.cv-preview-slot').getBoundingClientRect().width + 1"), "CV preview must fit the narrow slot")
  await evaluate(
    "{ const select = document.querySelector('select'); select.value = 'en'; select.dispatchEvent(new Event('change', { bubbles: true })) }",
  )
  await until("document.querySelector('nav button')?.textContent.trim() === 'Apply'")
  await evaluate("document.querySelectorAll('nav button')[0].click()")
  await until("!!document.querySelector('textarea')")
  // Reset the shared preference before the density fixture mounts a new preview.
  await evaluate("localStorage.setItem('careeros_cv_preferences_v1', JSON.stringify({fontSize: 12}))")
  const denseMaterials = {
    ...materials,
    resume: [
      "# Avery Morgan",
      "avery@example.com | https://example.com/avery",
      "## Professional Summary",
      "Product leader with experience shaping service journeys, collaborating across teams, and improving onboarding for customers.",
      "## Technical Skills",
      "- Research, product strategy, discovery, prioritization, stakeholder alignment, service design, analytics, accessibility, and delivery planning.",
      "- Figma, spreadsheets, reporting tools, prototyping, and cross-functional workshops.",
      "## Professional Experience",
      ...Array.from({ length: 3 }, (_, job) => [
        `### Product Lead · Harbor Works ${job + 1}`,
        "2021 — 2024",
        ...Array.from({ length: 5 }, (_, bullet) => `- Led service research and delivery with partner teams; improved customer journeys and onboarding through evidence-based product decisions ${bullet + 1}.`),
      ]).flat(),
      "## Education",
      "### BSc Design · East College",
      "2018 · Research, design systems, and digital services.",
      "## Certifications",
      "- Research Certificate · Design Guild · 2020",
      "## Languages",
      "- English: Fluent",
    ].join("\n"),
  }
  await evaluate(`window.fetch = async (url) => new Response(JSON.stringify(String(url).includes('qualification-gaps') ? { gaps: [] } : ${JSON.stringify(denseMaterials)}), { status: 200, headers: { 'Content-Type': 'application/json' } })`)
  await evaluate("Array.from(document.querySelectorAll('button')).find(el => /Generate/i.test(el.textContent) && !el.disabled).click()")
  await until("document.querySelector('h1')?.textContent.includes('Product Lead')")
  await evaluate("Array.from(document.querySelectorAll('button')).find(el => el.textContent.trim() === 'Résumé').click()")
  await until("document.querySelector('.cv-paper-content')?.textContent.includes('evidence-based product decisions 5')")
  const denseFit = await evaluate("({ status: document.querySelector('[role=status]')?.textContent, height: Math.round(document.querySelector('.cv-paper-content').getBoundingClientRect().height), page: Math.round(document.querySelector('.cv-page-boundary').getBoundingClientRect().height) })")
  assert.ok(
    denseFit.status?.includes("fits on one A4 page"),
    `a representative dense résumé should fit within one A4 page: ${JSON.stringify(denseFit)}`,
  )
  await evaluate("{ const badge = document.createElement('iframe'); badge.id = 'nl-badge-frame'; badge.srcdoc = '<body>Powered by Netlify</body>'; badge.style.cssText = 'position:fixed;bottom:0;right:0;width:194px;height:64px;border:0'; document.body.append(badge) }")
  await until("document.querySelector('#nl-badge-frame')?.contentDocument?.body?.textContent.includes('Powered by Netlify')")
  await evaluate("window.__printCalled = false; document.querySelector('.results-resume-actions button').click()")
  await until("window.__printCalled === true && document.body.classList.contains('results-print-resume')")
  const densePdf = join(work, "results-dense-resume.pdf")
  writeFileSync(densePdf, Buffer.from((await call("Page.printToPDF", { printBackground: true, preferCSSPageSize: true })).data, "base64"))
  assert.match(execFileSync("pdfinfo", [densePdf], { encoding: "utf8" }), /Pages:\s+1\b/, "dense résumé that fits the preview should print on one page")
  assert.doesNotMatch(execFileSync("pdftotext", ["-raw", densePdf, "-"], { encoding: "utf8" }), /Powered by Netlify/i, "Netlify's injected badge should not appear in the PDF")
  await evaluate("window.dispatchEvent(new Event('afterprint')); window.__printCalled = false")
  await evaluate(`{
    const paper = document.querySelector('.cv-paper')
    const copy = paper.cloneNode(true)
    copy.classList.add('cv-export-measure')
    copy.style.zoom = '1'
    document.body.append(copy)
    const height = copy.querySelector('.cv-paper-content').getBoundingClientRect().height
    copy.remove()
    const content = paper.querySelector('.cv-paper-content')
    content.style.paddingBottom = (parseFloat(getComputedStyle(content).paddingBottom) + (297 * 96 / 25.4 - 10 - height)) + 'px'
  }`)
  await evaluate("document.querySelector('.results-resume-actions button').click()")
  await until("window.__printCalled === true && document.body.classList.contains('results-print-resume')")
  const boundaryPdf = join(work, "results-boundary-resume.pdf")
  writeFileSync(boundaryPdf, Buffer.from((await call("Page.printToPDF", { printBackground: true, preferCSSPageSize: true })).data, "base64"))
  assert.match(execFileSync("pdfinfo", [boundaryPdf], { encoding: "utf8" }), /Pages:\s+1\b/, "résumé near the A4 boundary should still print on one page")
  assert.doesNotMatch(execFileSync("pdftotext", ["-raw", boundaryPdf, "-"], { encoding: "utf8" }), /Powered by Netlify/i)
  await evaluate("window.dispatchEvent(new Event('afterprint')); window.__printCalled = false")
  await evaluate("document.querySelector('#nl-badge-frame')?.remove()")
  await evaluate("document.querySelectorAll('nav button')[0].click()")
  await until("!!document.querySelector('textarea')")
  const longMaterials = {
    ...materials,
    resume: `## Professional Summary\n${"Long draft detail. ".repeat(2500)}END-MARKER`,
  }
  await evaluate(`window.fetch = async (url) => new Response(JSON.stringify(String(url).includes('qualification-gaps') ? { gaps: [] } : ${JSON.stringify(longMaterials)}), { status: 200, headers: { 'Content-Type': 'application/json' } })`)
  await evaluate("Array.from(document.querySelectorAll('button')).find(el => /Generate/i.test(el.textContent) && !el.disabled).click()")
  await until("document.querySelector('h1')?.textContent.includes('Product Lead')")
  await evaluate("Array.from(document.querySelectorAll('button')).find(el => el.textContent.trim() === 'Résumé').click()")
  await until("document.querySelector('[role=status]')?.textContent.includes('extends beyond')")
  assert.ok(await evaluate("document.querySelector('.cv-paper-content').textContent.includes('END-MARKER')"), "overflow must not hide generated text")
  await evaluate("window.__printCalled = false; document.querySelector('.results-resume-actions button').click()")
  await until("document.querySelector('[role=alert]')?.textContent.includes('exceeds one A4 page')")
  assert.equal(await evaluate("window.__printCalled"), false, "overflowing résumé should not open a misleading PDF export")
  assert.ok(await evaluate("document.body.textContent.includes('Add your name and contact details in Profile')"), "a draft without identity should direct the user to Profile")
  await evaluate("Array.from(document.querySelectorAll('button')).find(el => el.textContent.trim() === 'Profile').click()")
  await until("!!document.querySelector('aside nav')")
  console.log(
    "CV and Apply use an A4 document preview with matching section order",
  )
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
  if (process.env.CV_PREVIEW_KEEP) console.log(`Preview screenshots: ${work}`)
  else rmSync(work, { recursive: true, force: true })
}
