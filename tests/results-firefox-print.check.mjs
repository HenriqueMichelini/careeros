// Firefox regression for the Results résumé print layout. Uses synthetic data.
import assert from "node:assert/strict"
import { spawn, execFileSync } from "node:child_process"
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-firefox-print-"))
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const port = await new Promise((resolve) => {
  const server = createServer()
  server.listen(0, "127.0.0.1", () => {
    const value = server.address().port
    server.close(() => resolve(value))
  })
})
const firefoxPort = await new Promise((resolve) => {
  const server = createServer()
  server.listen(0, "127.0.0.1", () => {
    const value = server.address().port
    server.close(() => resolve(value))
  })
})
const profile = join(work, "firefox")
mkdirSync(profile)
const output = join(work, "resume.pdf")
writeFileSync(
  join(profile, "user.js"),
  `user_pref("print.always_print_silent", true);\nuser_pref("print_printer", "Mozilla Save to PDF");\nuser_pref("print.printer_Mozilla_Save_to_PDF.print_to_file", true);\nuser_pref("print.printer_Mozilla_Save_to_PDF.print_to_filename", ${JSON.stringify(output)});\nuser_pref("print.print_headerleft", "&T");\nuser_pref("print.print_headerright", "&U");\nuser_pref("print.print_footerleft", "&PT");\nuser_pref("print.print_footerright", "&D");\n`,
)
const vite = spawn(
  join(root, "node_modules/.bin/vite"),
  ["--host", "127.0.0.1", "--port", String(port), "--strictPort"],
  { cwd: root, stdio: "ignore" },
)
const firefox = spawn(
  "firefox",
  [
    "--headless",
    "--no-remote",
    "--profile",
    profile,
    "--remote-debugging-port",
    String(firefoxPort),
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
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      socket = new WebSocket(`ws://127.0.0.1:${firefoxPort}/session`)
      await new Promise((resolve, reject) => {
        socket.onopen = resolve
        socket.onerror = reject
      })
      break
    } catch {
      socket?.close()
      if (attempt === 99) throw new Error("Firefox BiDi did not start")
      await pause(100)
    }
  }
  let nextId = 0
  const pending = new Map()
  socket.onmessage = ({ data }) => {
    const message = JSON.parse(data)
    if (!message.id) return
    const callback = pending.get(message.id)
    pending.delete(message.id)
    if (message.type === "error")
      callback.reject(new Error(`${message.error}: ${message.message}`))
    else callback.resolve(message.result)
  }
  const call = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const id = ++nextId
      pending.set(id, { resolve, reject })
      socket.send(JSON.stringify({ id, method, params }))
    })
  await call("session.new", {
    capabilities: { alwaysMatch: { browserName: "firefox" } },
  })
  const tree = await call("browsingContext.getTree")
  const context = tree.contexts[0].context
  const evaluate = async (expression) => {
    const result = await call("script.evaluate", {
      expression,
      target: { context },
      awaitPromise: true,
    })
    if (result.type === "exception")
      throw new Error(result.exceptionDetails.text)
    return result.result.value
  }
  const until = async (expression) => {
    for (let attempt = 0; attempt < 100; attempt++) {
      if (await evaluate(expression)) return
      await pause(100)
    }
    throw new Error(
      `Timed out: ${expression}; ${await evaluate("JSON.stringify({ url: location.href, buttons: Array.from(document.querySelectorAll('button')).map(el => el.textContent.trim()).slice(0, 20), text: document.body.textContent.slice(0, 300) })")}`,
    )
  }
  await call("browsingContext.navigate", {
    context,
    url: `http://127.0.0.1:${port}/`,
    wait: "complete",
  })
  await until("!!document.querySelector('header button')")
  const repo = {
    fullName: "Avery Morgan",
    email: "avery@example.com",
    phone: "",
    location: "",
    professionalLinks: "",
    careerGoals: "Product leader focused on useful services.",
    skills: "Research, Product strategy",
    competencies: "",
    tools: "Figma",
    projects: [],
    education: [],
    certifications: [],
    languages: [],
    employmentStatus: "",
    currentSalary: "",
    desiredSalary: "",
    additionalInfo: "",
    experience: [],
  }
  await evaluate(
    `localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_apikey', 'synthetic-test-key')`,
  )
  await call("browsingContext.navigate", {
    context,
    url: `http://127.0.0.1:${port}/`,
    wait: "complete",
  })
  await until("!!document.querySelector('header button')")
  await evaluate("document.querySelector('header button:last-child').click()")
  await until("document.querySelectorAll('nav button').length === 4")
  await evaluate(
    "window.fetch = async (url) => new Response(JSON.stringify(String(url).includes('qualification-gaps') ? { gaps: [] } : {jobTitle:'Product Lead',company:'Harbor Works',jobSummary:'Lead product work.',resume:'# Avery Morgan\\n\\navery@example.com\\n\\n## Professional Summary\\nProduct leader focused on useful services.\\n\\n## Technical Skills\\n- Research\\n- Figma\\n\\n## Professional Experience\\n### Product Lead · Harbor Works\\n2021 — 2024\\n- Improved onboarding.\\n\\n## Education\\n### BSc Design\\nEast College · 2018\\n\\n## Certifications\\n- Research Certificate\\n\\n## Languages\\n- English: Fluent',coverLetter:'Dear team,\\n\\nI led useful service work.\\n\\nSincerely,\\nAvery Morgan',applicationAnswers:'1. I led a platform.'}), { status: 200, headers: { 'Content-Type': 'application/json' } }); const el = document.querySelector('textarea'); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, 'A long synthetic job posting for a product lead at Harbor Works.'); el.dispatchEvent(new Event('input', { bubbles: true }))",
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
  await until("!!document.querySelector('.cv-paper-content')")
  await evaluate(
    "document.querySelector('.cv-paper-content').insertAdjacentHTML('beforeend', Array.from({length: 12}, (_, i) => `<p class=\\\"text-[11px] leading-[1.4]\\\">Additional synthetic accomplishment ${i + 1}: improved service quality and delivery through careful engineering.</p>`).join(''))",
  )
  await evaluate(
    "{ const badge = document.createElement('iframe'); badge.id = 'nl-badge-frame'; badge.srcdoc = '<body>Powered by Netlify</body>'; badge.style.cssText = 'position:fixed;bottom:0;right:0;width:194px;height:64px;border:0'; document.body.append(badge) }",
  )
  await until(
    "document.querySelector('#nl-badge-frame')?.contentDocument?.body?.textContent.includes('Powered by Netlify')",
  )
  await evaluate(
    "document.querySelector('.results-resume-actions button').click()",
  )
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      if (readFileSync(output).length > 1000) break
    } catch {}
    if (attempt === 99)
      throw new Error("Firefox silent print did not create a PDF")
    await pause(100)
  }
  const info = execFileSync("pdfinfo", [output], { encoding: "utf8" })
  const pages = Number(info.match(/Pages:\s+(\d+)/)?.[1])
  console.log(`Firefox Save to PDF pages: ${pages}`)
  assert.equal(
    pages,
    1,
    "Firefox should print the Results résumé on one A4 page",
  )
  assert.match(
    info,
    /Page size:\s+59[4-6](?:\.\d+)? x 84[12](?:\.\d+)? pts \(A4\)/,
  )
  const pdfText = execFileSync("pdftotext", ["-raw", output, "-"], {
    encoding: "utf8",
  })
  assert.match(pdfText, /AVERY MORGAN[\s\S]*PROFESSIONAL SUMMARY/)
  assert.match(pdfText, /Additional synthetic accomplishment 12/)
  assert.equal(
    pdfText.match(/AVERY MORGAN/g)?.length,
    1,
    "the résumé should print only once",
  )
  assert.doesNotMatch(
    pdfText,
    /Powered by Netlify|Generated Application|Cover Letter/i,
  )
  await call("session.end")
} finally {
  socket?.close()
  firefox.kill()
  vite.kill()
  if (!process.env.CV_PREVIEW_KEEP)
    rmSync(work, { recursive: true, force: true })
}
