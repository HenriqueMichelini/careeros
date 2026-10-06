// Browser PDF integration check. Run with: npm run test:cv-pdf
// Requires Google Chrome, pdfinfo, and pdftotext on PATH.
import assert from "node:assert/strict"
import { spawn, execFileSync } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-cv-pdf-"))
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const port = await new Promise((resolve) => {
  const server = createServer()
  server.listen(0, "127.0.0.1", () => {
    const value = server.address().port
    server.close(() => resolve(value))
  })
})

const blank = {
  fullName: "",
  email: "",
  phone: "",
  location: "",
  professionalLinks: "",
  careerGoals: "",
  skills: "",
  competencies: "",
  experience: [],
  tools: "",
  projects: [],
  education: [],
  certifications: [],
  languages: [],
  employmentStatus: "",
  currentSalary: "",
  desiredSalary: "",
  additionalInfo: "",
}
const fixtures = [
  {
    locale: "en",
    repo: {
      ...blank,
      fullName: "Avery Morgan",
      email: "avery@example.com",
      professionalLinks: "https://example.com/avery",
      careerGoals: "Product leader focused on useful services.",
      skills: "Research, Product strategy",
      experience: [
        {
          id: "en-job",
          title: "Product Lead",
          company: "Harbor Works",
          startDate: "2021",
          endDate: "2024",
          current: false,
          location: "Boston",
          description: "Led a customer platform.",
          responsibilities: "",
          achievements: "Improved onboarding.",
        },
      ],
      education: [
        {
          id: "en-school",
          degree: "BSc Design",
          institution: "East College",
          location: "",
          graduationDate: "2018",
          details: "",
        },
      ],
    },
    order: [
      "Avery Morgan",
      "Professional Summary",
      "Product leader",
      "Technical Skills",
      "Research",
      "Professional Experience",
      "Product Lead",
      "Education",
      "BSc Design",
    ],
  },
  {
    locale: "pt-BR",
    repo: {
      ...blank,
      fullName: "Marina Alves",
      email: "marina@example.com",
      professionalLinks: "linkedin.com/in/marina-alves",
      careerGoals: "Profissional de produto com foco em serviços úteis.",
      skills: "Pesquisa, Estratégia de produto",
      experience: [
        {
          id: "pt-job",
          title: "Líder de produto",
          company: "Porto Digital",
          startDate: "2020",
          endDate: "2024",
          current: false,
          location: "Recife",
          description: "Liderou uma plataforma de clientes.",
          responsibilities: "",
          achievements: "Melhorou a integração.",
        },
      ],
      education: [
        {
          id: "pt-school",
          degree: "Bacharelado em Design",
          institution: "Universidade do Recife",
          location: "",
          graduationDate: "2017",
          details: "",
        },
      ],
    },
    order: [
      "Marina Alves",
      "Resumo profissional",
      "Profissional de produto",
      "Habilidades técnicas",
      "Pesquisa",
      "Experiência profissional",
      "Líder de produto",
      "Formação",
      "Bacharelado em Design",
    ],
  },
]

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
  const openCv = async (repo, locale) => {
    await call("Page.navigate", { url: `http://127.0.0.1:${port}/` })
    await until(
      "document.readyState === 'complete' && !!document.querySelector('header button')",
    )
    await evaluate(
      `localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_locale', ${JSON.stringify(locale)}); localStorage.removeItem('careeros_cv_v1'); location.reload()`,
    )
    await until(
      "document.readyState === 'complete' && !!document.querySelector('header button')",
    )
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('nav button').length === 4")
    await evaluate("document.querySelectorAll('nav button')[2].click()")
    await until("!!document.querySelector('.cv-paper-content')")
    await evaluate("document.fonts.ready")
  }

  for (const fixture of [
    ...fixtures,
    { locale: "en", repo: blank, order: [] },
  ]) {
    await openCv(fixture.repo, fixture.locale)
    const name = fixture.repo.fullName || "blank"
    if (fixture.repo.fullName) {
      await evaluate("window.print = () => { window.__printCalled = true }")
      await evaluate(
        "Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('PDF')).click()",
      )
      await until("window.__printCalled === true")
    }
    const pdf = await call("Page.printToPDF", {
      printBackground: true,
      preferCSSPageSize: true,
    })
    const file = join(work, `${name.replaceAll(" ", "-")}.pdf`)
    writeFileSync(file, Buffer.from(pdf.data, "base64"))
    const info = execFileSync("pdfinfo", [file], { encoding: "utf8" })
    assert.match(info, /Pages:\s+1\b/, `${name}: should be one page`)
    assert.match(
      info,
      /Page size:\s+59[45]\.\d+ x 841\.\d+ pts \(A4\)/,
      `${name}: should be A4`,
    )
    const text = execFileSync("pdftotext", ["-raw", file, "-"], {
      encoding: "utf8",
    })
    const comparable = text.toLocaleLowerCase(fixture.locale)
    let previous = -1
    for (const fragment of fixture.order) {
      const index = comparable.indexOf(
        fragment.toLocaleLowerCase(fixture.locale),
        previous + 1,
      )
      assert.ok(
        index > previous,
        `${name}: ${fragment} missing or out of order in PDF: ${text.slice(0, 400)}`,
      )
      previous = index
    }
    assert.doesNotMatch(
      text,
      /Northstar Labs|Your name|Seu nome|Sample content|Conteúdo de exemplo|Strategic professional|Profissional estratégico/,
    )
    if (!fixture.repo.fullName)
      assert.equal(
        text.trim(),
        "",
        "empty Profile must export no preview samples",
      )
    if (fixture.repo.professionalLinks) {
      const urls = execFileSync("pdfinfo", ["-url", file], { encoding: "utf8" })
      assert.match(
        urls,
        /https?:\/\//,
        `${name}: professional link should remain active`,
      )
    }
    console.log(
      `${name}: one A4 page; ordered selectable text; no sample facts`,
    )
  }

  await openCv(
    {
      ...blank,
      fullName: "Long Profile",
      careerGoals: "Long text. ".repeat(3000),
    },
    "en",
  )
  await evaluate("window.print = () => { window.__printCalled = true }")
  await evaluate(
    "Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('Export A4 PDF')).click()",
  )
  await until("!!document.querySelector('[role=alert]')")
  assert.equal(
    await evaluate("window.__printCalled === true"),
    false,
    "overflow must block export",
  )
  console.log("Overflow: clear result; print was not called")
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
  if (process.env.CV_PDF_KEEP) console.log(`PDF review files: ${work}`)
  else rmSync(work, { recursive: true, force: true })
}
