import { savedProfileExpression } from "./profile-browser-storage.mjs"
import { keyboardFlow } from "./keyboard-flow.mjs"
// Browser regression for raw professional-information review and explicit apply.
// Uses synthetic API responses; no provider request is made.
import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-section-browser-"))
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
      userGesture: true,
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
    throw new Error(
      `Timed out: ${expression}; ${await evaluate("JSON.stringify({text:document.body.innerText.slice(0,1500),injected:typeof window.__originalStorageSet,canonical:localStorage.getItem('careeros_profile_v2')!==null})")}`,
    )
  }
  await call("Page.enable")
  await call("Page.navigate", { url: `http://127.0.0.1:${port}/` })
  await until(
    "document.readyState === 'complete' && !!document.querySelector('header button')",
  )
  await evaluate(
    `localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_apikey', 'synthetic-test-key'); localStorage.setItem('careeros_typesafe_key','synthetic-typesafe'); location.reload()`,
  )
  await until(
    "document.readyState === 'complete' && !!document.querySelector('header button')",
  )
  const openProfile = async () => {
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('header nav button').length === 4")
    await evaluate("document.querySelectorAll('header nav button')[1].click()")
    await until("!!document.querySelector('#ingestion-text')")
  }
  const fill = async (value, selector = "#ingestion-text") => {
    await evaluate(
      `(() => { const el = document.querySelector(${JSON.stringify(selector)}); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, ${JSON.stringify(value)}); el.dispatchEvent(new Event('input', { bubbles: true })); })()`,
    )
    await pause(60)
  }
  const saved = () => evaluate(savedProfileExpression)
  const byText = (text) =>
    `Array.from(document.querySelectorAll('button')).find(el=>el.textContent.trim()===${JSON.stringify(text)})`
  const { keyboardFill, keyboardSubmit } = keyboardFlow({
    call,
    evaluate,
    pause,
    selector: "#ingestion-text",
  })

  const mock = `window.fetch=async(url,options)=>{if(url!='/api/profile/review')throw new Error('Unexpected request');const body=JSON.parse(options.body);window.__reviewBody=body;const {reviewableFact}=await import('/src/lib/sectionReview.ts');const result={profileId:body.document.id,revision:body.document.revision,section:body.section,summary:'Clearer wording',patches:body.document.facts.filter(f=>reviewableFact(body.document,body.section,f)).map(f=>({factId:f.id,revision:f.revision,wording:f.field==='skills'?'Research and product strategy':String(f.value),supporting:[{id:f.id,revision:f.revision}]}))};if(window.__delay)await new Promise(resolve=>window.__releaseReview=resolve);return new Response(JSON.stringify(result),{status:200})}`
  for (const locale of ["en", "pt-BR"])
    for (const width of [1440, 390]) {
      await call("Emulation.setDeviceMetricsOverride", {
        width,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false,
      })
      const review = locale === "en" ? "Review with AI" : "Revisar com IA"
      const accept =
        locale === "en"
          ? "Accept and save changes"
          : "Aceitar e salvar alterações"
      const reject = locale === "en" ? "Reject proposal" : "Rejeitar proposta"
      const skills =
        locale === "en" ? "Skills, tools & tech" : "Habilidades e tecnologias"
      const label =
        locale === "en"
          ? "Review Profile Proposal"
          : "Revisar proposta do Perfil"
      const panel = `document.querySelector('section[aria-label="${label}"]')`
      const reset = async () => {
        await evaluate(
          `localStorage.setItem('careeros_locale',${JSON.stringify(locale)});localStorage.removeItem('careeros_profile_v2');localStorage.setItem('careeros_repo',${JSON.stringify(JSON.stringify(repo))});location.reload()`,
        )
        await pause(250)
        await until(
          "document.readyState === 'complete' && !!document.querySelector('header button:last-child')",
        )
        await openProfile()
        await evaluate(`${byText(skills)}.click()`)
        await evaluate(mock)
      }
      await reset()
      const original = await evaluate(
        "localStorage.getItem('careeros_profile_v2')",
      )
      await evaluate(`${byText(review)}.click()`)
      await until(`${panel} !== null`)
      assert.equal(
        await evaluate("localStorage.getItem('careeros_profile_v2')"),
        original,
      )
      assert.equal(
        await evaluate(
          'JSON.stringify(window.__reviewBody).includes("careerGoals")',
        ),
        false,
      )
      assert.equal(
        await evaluate(
          'JSON.stringify(window.__reviewBody).includes("fullName")',
        ),
        false,
      )
      assert.ok(
        await evaluate("document.documentElement.scrollWidth <= innerWidth"),
      )
      const image = await call("Page.captureScreenshot", {
        format: "png",
        captureBeyondViewport: true,
      })
      writeFileSync(
        join(work, `section-${locale}-${width}.png`),
        Buffer.from(image.data, "base64"),
      )
      await evaluate(`${byText(reject)}.click()`)
      assert.equal(
        await evaluate("localStorage.getItem('careeros_profile_v2')"),
        original,
      )
      await evaluate(`${byText(review)}.click()`)
      await until(`${panel} !== null`)
      await fill(
        "User-authored product strategy",
        `section[aria-label="${label}"] textarea`,
      )
      await evaluate(`${byText(accept)}.click()`)
      await until(`${panel} === null`)
      assert.equal(
        JSON.parse(await saved()).skills,
        "User-authored product strategy",
      )
      assert.equal(
        await evaluate(
          "JSON.parse(localStorage.getItem('careeros_profile_v2')).facts.find(f=>f.field==='skills').origin.original",
        ),
        "user",
      )
      assert.equal(JSON.parse(await saved()).careerGoals, repo.careerGoals)
      // Revisions to source metadata invalidate a pending proposal before apply.
      await reset()
      await evaluate(`${byText(review)}.click()`)
      await until(`${panel} !== null`)
      await evaluate(
        `window.__peer=window.open('about:blank','section-peer');const doc=JSON.parse(localStorage.getItem('careeros_profile_v2'));doc.revision++;for(const f of doc.facts)if(f.owner.id===doc.id)f.owner.revision=doc.revision;window.__peer.localStorage.setItem('careeros_profile_v2',JSON.stringify(doc));`,
      )
      await until(`${byText(accept)}.disabled`)
      assert.ok(await evaluate(`${panel} !== null`))
      await evaluate("window.__peer.close()")
      // A quota error keeps both the saved Profile and the edited proposal.
      await reset()
      const quotaOriginal = await evaluate(
        "localStorage.getItem('careeros_profile_v2')",
      )
      await evaluate(`${byText(review)}.click()`)
      await until(`${panel} !== null`)
      await fill(
        "Draft retained after quota",
        `section[aria-label="${label}"] textarea`,
      )
      await evaluate(
        `window.__realSet=Storage.prototype.setItem;Storage.prototype.setItem=function(key,value){if(key==='careeros_profile_v2')throw new DOMException('quota','QuotaExceededError');return window.__realSet.call(this,key,value)}`,
      )
      await evaluate(`${byText(accept)}.click()`)
      await until("!!document.querySelector('[role=alert]')")
      assert.equal(
        await evaluate("localStorage.getItem('careeros_profile_v2')"),
        quotaOriginal,
      )
      assert.equal(
        await evaluate(`${panel}.querySelector('textarea').value`),
        "Draft retained after quota",
      )
      await evaluate("Storage.prototype.setItem=window.__realSet")
      // A completion after cancellation or navigation never installs a proposal.
      await reset()
      await evaluate("window.__delay=true")
      await evaluate(`${byText(review)}.click()`)
      await until("typeof window.__releaseReview === 'function'")
      const cancel =
        locale === "en" ? "Cancel and discard" : "Cancelar e descartar"
      await evaluate(`${byText(cancel)}.click()`)
      await evaluate("window.__releaseReview()")
      await pause(100)
      assert.equal(await evaluate(`${panel}`), null)
      await evaluate("window.__delay=true")
      await evaluate(`${byText(review)}.click()`)
      await until("typeof window.__releaseReview === 'function'")
      await evaluate(
        `${byText(locale === "en" ? "Experience" : "Experiência")}.click()`,
      )
      await evaluate("window.__releaseReview()")
      await pause(100)
      assert.equal(await evaluate(`${panel}`), null)
      console.log(
        `PASS section rewrite review/edit/reject/apply/quota/cross-tab/cancel/stale ${locale} ${width}px`,
      )
    }
  console.log("Screenshots:", work)
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}
