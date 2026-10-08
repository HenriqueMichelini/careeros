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
      `localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_locale', ${JSON.stringify(locale)}); localStorage.setItem('careeros_cv_language', ${JSON.stringify(locale)}); localStorage.removeItem('careeros_cv_v1'); localStorage.removeItem('careeros_cv_preferences_v1'); location.reload()`,
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
    const originalProfile = await evaluate(
      "localStorage.getItem('careeros_repo')",
    )
    const originalChoices = await evaluate(
      "localStorage.getItem('careeros_cv_v1')",
    )
    await evaluate(
      "window.__aiCalls = 0; window.fetch = () => { window.__aiCalls++; throw new Error('AI must not be called') }",
    )
    let previousHeight = 0
    for (const size of [12, 14, 16]) {
      // Exercise the native slider through keyboard input, including its bounds.
      await evaluate(
        "document.querySelector('.cv-font-controls input').focus()",
      )
      await call("Input.dispatchKeyEvent", {
        type: "keyDown",
        key: "Home",
        code: "Home",
        windowsVirtualKeyCode: 36,
      })
      await call("Input.dispatchKeyEvent", {
        type: "keyUp",
        key: "Home",
        code: "Home",
        windowsVirtualKeyCode: 36,
      })
      for (let step = 12; step < size; step++) {
        await call("Input.dispatchKeyEvent", {
          type: "keyDown",
          key: "ArrowRight",
          code: "ArrowRight",
          windowsVirtualKeyCode: 39,
        })
        await call("Input.dispatchKeyEvent", {
          type: "keyUp",
          key: "ArrowRight",
          code: "ArrowRight",
          windowsVirtualKeyCode: 39,
        })
      }
      await until(
        `document.querySelector('.cv-font-controls input').value === '${size}'`,
      )
      assert.equal(
        await evaluate(
          "document.querySelector('.cv-font-controls output').textContent",
        ),
        `${size} px`,
      )
      assert.equal(
        await evaluate(
          "parseFloat(getComputedStyle(document.querySelector('.cv-paper-content')).fontSize)",
        ),
        size,
      )
      const screenType = await evaluate(
        "({body: getComputedStyle(document.querySelector('.cv-paper-content p')).fontSize, heading: getComputedStyle(document.querySelector('.cv-paper-content h3')).fontSize, contact: document.querySelector('.cv-paper-content header ul') && getComputedStyle(document.querySelector('.cv-paper-content header ul')).fontSize})",
      )
      assert.equal(screenType.body, `${size}px`)
      assert.equal(screenType.heading, `${size + 1}px`)
      if (screenType.contact) assert.equal(screenType.contact, `${size - 1}px`)
      assert.equal(await evaluate("window.__aiCalls"), 0)
      assert.equal(
        await evaluate("localStorage.getItem('careeros_repo')"),
        originalProfile,
      )
      assert.equal(
        await evaluate("localStorage.getItem('careeros_cv_v1')"),
        originalChoices,
      )
      assert.equal(
        await evaluate(
          "JSON.parse(localStorage.getItem('careeros_cv_preferences_v1')).fontSize",
        ),
        size,
      )
      const height = await evaluate("document.querySelector('.cv-paper-content').getBoundingClientRect().height")
      assert.ok(height > previousHeight, "font changes must update document height and wrapping immediately")
      previousHeight = height
      assert.ok(await evaluate(`Array.from(document.querySelectorAll('.cv-paper-content [data-cv-block] p')).every(el => getComputedStyle(el).fontSize === '${size}px')`), "entry text also scales")
      await call("Emulation.setEmulatedMedia", { media: "print" })
      assert.equal(
        await evaluate(
          "getComputedStyle(document.querySelector('.cv-paper-content p')).fontSize",
        ),
        screenType.body,
      )
      await call("Emulation.setEmulatedMedia", { media: "" })
      if (size === 16) {
        await call("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: false })
        await until("document.querySelector('.cv-paper').getBoundingClientRect().width <= document.querySelector('.cv-preview-slot').clientWidth + 1")
        assert.ok(await evaluate("document.documentElement.scrollWidth <= innerWidth"), "short CV and typography controls fit390px in either locale")
        if (process.env.CV_PDF_KEEP) {
          writeFileSync(join(work, `font-controls-${fixture.locale}-390.png`), Buffer.from((await call("Page.captureScreenshot", {format: "png"})).data, "base64"))
        }
        await call("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false })
      }
      const name = `${fixture.repo.fullName || "blank"}-${size}`
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
        const urls = execFileSync("pdfinfo", ["-url", file], {
          encoding: "utf8",
        })
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
    // The selected maximum persists across navigation and reload.
    await evaluate(
      "document.querySelectorAll('nav button')[1].click(); document.querySelectorAll('nav button')[2].click()",
    )
    await until(
      "document.querySelector('.cv-font-controls input')?.value === '16'",
    )
    await evaluate("location.reload()")
    await until(
      "document.readyState === 'complete' && !!document.querySelector('header button')",
    )
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('nav button').length === 4")
    await evaluate("document.querySelectorAll('nav button')[2].click()")
    await until(
      "document.querySelector('.cv-font-controls input')?.value === '16'",
    )
    await evaluate("document.querySelector('.cv-font-controls input').focus()")
    await call("Input.dispatchKeyEvent", { type: "keyDown", key: "Home", code: "Home" })
    await call("Input.dispatchKeyEvent", { type: "keyUp", key: "Home", code: "Home" })
    await until(
      "document.querySelector('.cv-font-controls input').value === '12'",
    )
    assert.equal(
      await evaluate("localStorage.getItem('careeros_repo')"),
      originalProfile,
    )
    assert.equal(
      await evaluate("localStorage.getItem('careeros_cv_v1')"),
      originalChoices,
    )
  }

  for (const raw of ['{', '{"fontSize":99}', '{"fontSize":"16"}', '{}']) {
    await evaluate(`localStorage.setItem('careeros_cv_preferences_v1', ${JSON.stringify(raw)}); location.reload()`)
    await until("document.readyState === 'complete' && !!document.querySelector('header button')")
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('nav button').length === 4")
    await evaluate("document.querySelectorAll('nav button')[2].click()")
    await until("document.querySelector('.cv-font-controls input')?.value === '12'")
  }

  await openCv(
    {
      ...blank,
      fullName: "Long Profile",
      careerGoals: "Long text. ".repeat(3000),
    },
    "en",
  )
  await call("Emulation.setDeviceMetricsOverride", {
    width: 390,
    height: 844,
    deviceScaleFactor: 1,
    mobile: false,
  })
  for (const size of [12, 16]) {
    await evaluate(
      `{ const el = document.querySelector('.cv-font-controls input'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, '${size}'); el.dispatchEvent(new Event('input', { bubbles: true })) }`,
    )
    await until(
      "Array.from(document.querySelectorAll('[role=status]')).some(e=>e.textContent.includes('extends about'))",
    )
    assert.ok(
      await evaluate("document.documentElement.scrollWidth <= innerWidth"),
      "font control and overflow must fit 390px",
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
    console.log(
      `Overflow at ${size}px / 390px: clear result; print was not called`,
    )
  }
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
  if (process.env.CV_PDF_KEEP) console.log(`PDF review files: ${work}`)
  else rmSync(work, { recursive: true, force: true })
}
