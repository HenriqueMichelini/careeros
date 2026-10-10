import { savedProfileExpression } from "./profile-browser-storage.mjs"
// Browser PDF integration check. Run with: npm run test:cv-pdf
// Requires Google Chrome, pdfinfo, and pdftotext on PATH.
import assert from "node:assert/strict"
import { spawn, execFileSync } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-cv-generation-"))
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
  const openCv = async (repo, locale) => {
    await call("Page.navigate", { url: `http://127.0.0.1:${port}/` })
    await until(
      "document.readyState === 'complete' && !!document.querySelector('header button')",
    )
    await evaluate(
      `localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_locale', ${JSON.stringify(locale)}); localStorage.setItem('careeros_cv_language', ${JSON.stringify(locale)}); localStorage.removeItem('careeros_curated_cv_v1'); localStorage.setItem('careeros_apikey','sk-synthetic'); localStorage.removeItem('careeros_cv_v1'); localStorage.removeItem('careeros_cv_preferences_v1'); location.reload()`,
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

  const click = async (text) =>
    evaluate(
      `(() => { const button=Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(text)}); if(!button)throw new Error('Missing button '+${JSON.stringify(text)}+'; '+document.body.innerText.slice(0,3000)); button.click(); })()`,
    )
  const editSummary = async (text) =>
    evaluate(
      `{const el=document.querySelector('textarea');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,${JSON.stringify(text)});el.dispatchEvent(new Event('input',{bubbles:true}))}`,
    )
  const controlled = async () =>
    evaluate(
      `window.__calls=0;window.fetch=async(url,options)=>{window.__calls++;window.__outbound=JSON.parse(options.body);const facts=window.__outbound.facts;const summary=facts.find(f=>f.field==='description');const selected=facts.filter(f=>['skills','title','company','startDate','endDate','description','achievements','degree','institution','graduationDate','name','highlights'].includes(f.field)).map(f=>f.id);return new Response(JSON.stringify({summary:[{sourceId:summary.id,text:summary.text}],selected}),{status:200,headers:{'Content-Type':'application/json'}})}`,
    )
  for (const fixture of fixtures) {
    const repo = {
      ...fixture.repo,
      currentSalary: "SECRET-SALARY",
      desiredSalary: "SECRET-TARGET",
      employmentStatus: "SECRET-STATUS",
      additionalInfo: "SECRET-HEALTH",
      projects: [
        {
          id: "PRIVATE-PROJECT-ID",
          name:
            fixture.locale === "en" ? "Community portal" : "Portal comunitário",
          description: "",
          technologies: "",
          url: "https://secret.example",
          highlights:
            fixture.locale === "en"
              ? "Improved access to services."
              : "Melhorou o acesso aos serviços.",
        },
      ],
    }
    await openCv(repo, fixture.locale)
    const generate = fixture.locale === "en" ? "Generate CV" : "Gerar currículo"
    const accept =
      fixture.locale === "en"
        ? "Accept and replace CV"
        : "Aceitar e substituir CV"
    await controlled()
    const original = await evaluate(savedProfileExpression)
    await click(generate)
    await until("!!document.querySelector('[data-generated-summary]')")
    const outbound = await evaluate("JSON.stringify(window.__outbound)")
    for (const secret of [
      "SECRET-",
      "PRIVATE-PROJECT-ID",
      "secret.example",
      repo.email,
      repo.careerGoals,
    ].filter(Boolean))
      assert.ok(!outbound.includes(secret), secret)
    assert.equal(
      await evaluate("localStorage.getItem('careeros_curated_cv_v1')"),
      null,
      "proposal must not apply itself",
    )
    await click(accept)
    await until("!!localStorage.getItem('careeros_curated_cv_v1')")
    assert.equal(await evaluate(savedProfileExpression), original)
    await editSummary(
      fixture.locale === "en"
        ? "Edited professional summary."
        : "Resumo profissional editado.",
    )
    await until(
      "localStorage.getItem('careeros_cv_v1').includes('edit') || localStorage.getItem('careeros_cv_v1').includes('Edited')",
    )
    const edited = await evaluate("localStorage.getItem('careeros_cv_v1')")
    await click(generate)
    await until("!!document.querySelector('[data-generated-summary]')")
    assert.equal(
      await evaluate("localStorage.getItem('careeros_cv_v1')"),
      edited,
      "regeneration must preserve manual edits until acceptance",
    )
    await click(
      fixture.locale === "en"
        ? "Cancel / discard proposal"
        : "Cancelar / descartar proposta",
    )
    assert.equal(
      await evaluate("localStorage.getItem('careeros_cv_v1')"),
      edited,
    )
    await call("Emulation.setDeviceMetricsOverride", {
      width: 390,
      height: 844,
      deviceScaleFactor: 1,
      mobile: false,
    })
    await until("document.documentElement.scrollWidth<=innerWidth")
    assert.ok(
      await evaluate("document.documentElement.scrollWidth<=innerWidth"),
      "390px horizontal fit",
    )
    await call("Page.captureScreenshot", {
      format: "png",
      captureBeyondViewport: true,
    }).then((r) =>
      writeFileSync(
        join(work, fixture.locale + "-390.png"),
        Buffer.from(r.data, "base64"),
      ),
    )
    await call("Emulation.setDeviceMetricsOverride", {
      width: 1440,
      height: 1000,
      deviceScaleFactor: 1,
      mobile: false,
    })
    await evaluate("document.fonts.ready")
    const pdf = await call("Page.printToPDF", {
      printBackground: true,
      preferCSSPageSize: true,
    })
    const path = join(work, fixture.locale + ".pdf")
    writeFileSync(path, Buffer.from(pdf.data, "base64"))
    const info = execFileSync("pdfinfo", [path], { encoding: "utf8" })
    assert.match(info, /Pages:\s+1/)
    const text = execFileSync("pdftotext", ["-raw", path, "-"], {
      encoding: "utf8",
    })
    assert.ok(text.toLowerCase().includes(repo.fullName.toLowerCase()))
    assert.ok(
      text.includes(
        fixture.locale === "en"
          ? "Edited professional summary."
          : "Resumo profissional editado.",
      ),
    )
    execFileSync("pdftoppm", [
      "-f",
      "1",
      "-singlefile",
      "-scale-to",
      "1200",
      "-png",
      path,
      join(work, fixture.locale),
    ])
    await evaluate("location.reload()")
    await until("!!document.querySelector('header button')")
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('nav button').length===4")
    await evaluate("document.querySelectorAll('nav button')[2].click()")
    await until("!!document.querySelector('.cv-paper-content')")
    assert.equal(
      await evaluate("localStorage.getItem('careeros_cv_v1')"),
      edited,
      "reload preserves accepted edits",
    )
    const acceptedRaw = await evaluate(
      "localStorage.getItem('careeros_curated_cv_v1')",
    )
    const accepted = JSON.parse(acceptedRaw)
    assert.equal(accepted.version, 2)
    assert.ok(
      accepted.sources.every(
        (f) => f.reference?.id === f.id && f.reference.revision > 0,
      ),
    )
    await evaluate(
      `(() => { const key='careeros_profile_v2'; const doc=JSON.parse(localStorage.getItem(key)); const fact=doc.facts.find(f=>f.id===${JSON.stringify(accepted.sources[0].id)});fact.revision++;fact.value='Changed source';doc.revision++;localStorage.setItem(key,JSON.stringify(doc));location.reload(); })()`,
    )
    await until("!!document.querySelector('header button')")
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('nav button').length===4")
    await evaluate("document.querySelectorAll('nav button')[2].click()")
    await until("!!document.querySelector('[data-cv-saved-support]')")
    assert.ok(
      await evaluate(
        "!!document.querySelector('[data-cv-saved-support] [role=status]')",
      ),
      "changed Profile support is visible",
    )
    assert.equal(
      await evaluate("localStorage.getItem('careeros_curated_cv_v1')"),
      acceptedRaw,
      "live edit preserves accepted snapshot and manual edits",
    )
    // A persisted v1 CV remains independent: never map positional IDs to live facts.
    await evaluate(
      `(() => {const saved=JSON.parse(localStorage.getItem('careeros_curated_cv_v1'));saved.version=1;delete saved.sourceProfileId;delete saved.context;delete saved.summarySources;saved.sources.forEach((f,i)=>{f.id='f'+i;delete f.reference;delete f.owner;delete f.evidence});localStorage.setItem('careeros_curated_cv_v1',JSON.stringify(saved));location.reload()})()`,
    )
    await until("!!document.querySelector('header button')")
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('nav button').length===4")
    await evaluate("document.querySelectorAll('nav button')[2].click()")
    await until("!!document.querySelector('[data-cv-saved-support]')")
    assert.equal(
      await evaluate(
        "!!document.querySelector('[data-cv-saved-support] [role=status]')",
      ),
      false,
    )
    assert.equal(
      await evaluate("document.querySelector('textarea').value"),
      fixture.locale === "en"
        ? "Edited professional summary."
        : "Resumo profissional editado.",
    )
    console.log(
      fixture.locale +
        ": staged/replaced/edited/regenerated/discarded/reloaded; 390px and extractable one-page A4 controlled PDF",
    )
  }
  await openCv(fixtures[0].repo, "en")
  await controlled()
  await editSummary("Keep current manual wording.")
  await evaluate(
    `window.fetch=()=>new Promise(resolve=>window.__resolve=resolve)`,
  )
  await click("Generate CV")
  await until("!!window.__resolve")
  await editSummary("Newer edit wins.")
  await evaluate(
    `window.__resolve(new Response(JSON.stringify({summary:[],selected:[]})))`,
  )
  await pause(100)
  assert.equal(
    await evaluate("!!document.querySelector('[data-generated-summary]')"),
    false,
  )
  assert.ok(
    (await evaluate("localStorage.getItem('careeros_cv_v1')")).includes(
      "Newer edit wins.",
    ),
  )
  for (const code of ["rate_limit", "timeout", "invalid_output", "outage"]) {
    await evaluate(
      `window.fetch=async()=>new Response(JSON.stringify({error:${JSON.stringify(code)}}),{status:502})`,
    )
    await click("Generate CV")
    await until("!!document.querySelector('[data-cv-generation] [role=alert]')")
    assert.ok(
      (await evaluate("localStorage.getItem('careeros_cv_v1')")).includes(
        "Newer edit wins.",
      ),
    )
  }
  // A large source whose distinctive impact is at the end of the input.
  const heavy = {
    ...fixtures[0].repo,
    skills: "Research, Research, Design, Irrelevant filler",
    experience: [
      ...Array.from({ length: 18 }, (_, i) => ({
        ...fixtures[0].repo.experience[0],
        id: `old${i}`,
        title: "Assistant",
        company: `Old employer ${i}`,
        description: "Filed routine reports.",
        achievements: "",
        responsibilities: "Filed routine reports.\nFiled routine reports.",
      })),
      {
        ...fixtures[0].repo.experience[0],
        id: "last-impact",
        title: "Product Lead",
        company: "Impact Studio",
        description:
          "Product leader designing useful services. Worked on services and more services. Designed customer-facing services using research. Led useful service design.",
        achievements: "Reduced onboarding time by 30%.",
        responsibilities: "Repeated routine report.\nRepeated routine report.",
      },
    ],
    projects: [
      {
        id: "complement",
        name: "Accessible portal",
        description: "",
        technologies: "",
        url: "",
        highlights: "Expanded access for local communities.",
      },
    ],
  }
  await openCv(heavy, "en")
  await evaluate(
    `window.fetch=async(url,options)=>{const facts=JSON.parse(options.body).facts;const summary=facts.find(f=>f.text.startsWith('Product leader designing useful services.'));const role=facts.find(f=>f.text==='Impact Studio');const ids=facts.filter(f=>f.entryId===role.entryId && f.field!=='responsibilities' || f.section==='education' || f.section==='projects' || f.text==='Research' || f.text==='Design').map(f=>f.id);return new Response(JSON.stringify({summary:[{sourceId:summary.id,text:'Product leader using research to design customer-facing services.'}],selected:ids,wording:{[summary.id]:'Designed customer-facing services using research.'}}),{status:200})}`,
  )
  await click("Generate CV")
  await until("!!document.querySelector('[data-generated-summary]')")
  await click("Accept and replace CV")
  await until("!!localStorage.getItem('careeros_curated_cv_v1')")
  const snapshot = await evaluate(
    "JSON.parse(localStorage.getItem('careeros_curated_cv_v1'))",
  )
  assert.equal(snapshot.repository.experience.length, 1)
  assert.equal(snapshot.repository.experience[0].company, "Impact Studio")
  assert.equal(snapshot.repository.experience[0].responsibilities, "")
  assert.equal(snapshot.repository.projects.length, 1)
  assert.equal(
    snapshot.summary,
    "Product leader using research to design customer-facing services.",
  )
  assert.equal(
    snapshot.repository.experience[0].description,
    "Designed customer-facing services using research.",
  )
  assert.ok(
    snapshot.sources.some((f) => f.text.includes("services and more services")),
    "original source remains inspectable",
  )
  assert.ok(
    await evaluate(
      "!Array.from(document.querySelectorAll('.cv-controls [role=status]')).some(e=>e.textContent.includes('extends about'))",
    ),
  )
  // Simulate failed legacy mirror write: atomic accepted snapshot contains the new choices.
  await controlled()
  await click("Generate CV")
  await until("!!document.querySelector('[data-generated-summary]')")
  const atomicSummary = await evaluate(
    "document.querySelector('[data-generated-summary]').textContent",
  )
  await evaluate(
    `window.__originalSet=Storage.prototype.setItem;Storage.prototype.setItem=function(key,value){if(key==='careeros_cv_v1')throw new DOMException('quota','QuotaExceededError');return window.__originalSet.call(this,key,value)}`,
  )
  await click("Accept and replace CV")
  await pause(100)
  assert.equal(
    await evaluate(
      "JSON.parse(localStorage.getItem('careeros_curated_cv_v1')).choices.summary",
    ),
    atomicSummary,
  )
  await evaluate(
    `Storage.prototype.setItem=window.__originalSet;location.reload()`,
  )
  await until("!!document.querySelector('header button')")
  await evaluate("document.querySelector('header button:last-child').click()")
  await until("document.querySelectorAll('nav button').length===4")
  await evaluate("document.querySelectorAll('nav button')[2].click()")
  await until("!!document.querySelector('.cv-paper-content')")
  assert.equal(
    await evaluate("document.querySelector('textarea').value"),
    atomicSummary,
    "atomic choices win over failed old mirror",
  )
  await controlled()
  await click("Generate CV")
  await until("!!document.querySelector('[data-generated-summary]')")
  const beforeFailedSave = await evaluate(
    "localStorage.getItem('careeros_curated_cv_v1')",
  )
  await evaluate(
    `Storage.prototype.setItem=function(key,value){if(key==='careeros_curated_cv_v1')throw new DOMException('quota','QuotaExceededError');return window.__originalSet.call(this,key,value)}`,
  )
  await click("Accept and replace CV")
  await until("!!document.querySelector('[data-cv-generation] [role=alert]')")
  assert.equal(
    await evaluate("localStorage.getItem('careeros_curated_cv_v1')"),
    beforeFailedSave,
    "failed primary write preserves previous CV",
  )
  assert.ok(
    await evaluate("!!document.querySelector('[data-generated-summary]')"),
    "proposal remains reviewable after failed save",
  )
  await evaluate(`Storage.prototype.setItem=window.__originalSet`)
  await click("Cancel / discard proposal")

  assert.equal(
    JSON.parse(await evaluate(savedProfileExpression)).experience.length,
    19,
  )
  await evaluate(
    `window.fetch=()=>new Promise(resolve=>window.__resolveCancel=resolve)`,
  )
  await click("Generate CV")
  await until("!!window.__resolveCancel")
  await click("Cancel / discard proposal")
  await evaluate("window.__resolveCancel(new Response('{}'))")
  await pause(100)
  assert.equal(
    await evaluate("!!document.querySelector('[data-generated-summary]')"),
    false,
  )
  // Selecting genuinely long facts exposes measured overflow, never clips them.
  const long = {
    ...heavy,
    experience: [
      {
        ...heavy.experience.at(-1),
        description: "Detailed supported professional statement. ".repeat(200),
      },
    ],
  }
  await openCv(long, "en")
  await controlled()
  await click("Generate CV")
  await until("!!document.querySelector('[data-generated-summary]')")
  await click("Accept and replace CV")
  await until(
    "Array.from(document.querySelectorAll('.cv-controls [role=status]')).some(e=>e.textContent.includes('extends about'))",
  )
  assert.equal(
    await evaluate(
      "JSON.parse(localStorage.getItem('careeros_curated_cv_v1')).repository.experience[0].description.length",
    ),
    long.experience[0].description.length,
  )
  console.log(
    "Heavy 19-role source: later impact selected with complementary project, duplicates omitted, source intact; cancelled response ignored; actual oversized selected content surfaces overflow",
  )
  // Same source in each mode: controlled fixture responses establish propagation,
  // accepted volume, fit, editing and persistence; they do not establish provider quality.
  for (const fixture of fixtures) {
    for (const [size, profile] of [
      ["short", fixture.repo],
      ["heavy", { ...heavy, fullName: fixture.repo.fullName }],
    ]) {
      await openCv(profile, fixture.locale)
      const originalProfile = await evaluate(savedProfileExpression)
      const lengths = []
      const generate =
        fixture.locale === "en" ? "Generate CV" : "Gerar currículo"
      const accept =
        fixture.locale === "en"
          ? "Accept and replace CV"
          : "Aceitar e substituir CV"
      await evaluate(`window.__calls=0;window.fetch=async(url,options)=>{
        window.__calls++;window.__outbound=JSON.parse(options.body);
        const {facts,density}=window.__outbound;
        const source=facts.find(f=>f.field==='description');
        const primary=facts.filter(f=>f.entryId===source.entryId);
        const anchors=['title','company','startDate','endDate','current'];
        const selected=density==='compact' ? primary.filter(f=>anchors.includes(f.field)||f.field==='description') :
          density==='balanced' ? facts.filter(f=>f.entryId===source.entryId || f.section==='skills') : facts;
        return new Response(JSON.stringify({summary:[{sourceId:source.id,text:source.text}],selected:selected.map(f=>f.id)}),{status:200})
      }`)
      for (const density of ["compact", "balanced", "detailed"]) {
        const before = await evaluate(
          "document.querySelector('.cv-paper-content').textContent",
        )
        const calls = await evaluate("window.__calls")
        await evaluate(
          `document.querySelector('[data-cv-density] input[value="${density}"]').click()`,
        )
        await pause(50)
        assert.equal(
          await evaluate("window.__calls"),
          calls,
          "density selection makes no request",
        )
        assert.equal(
          await evaluate(
            "document.querySelector('.cv-paper-content').textContent",
          ),
          before,
          "selection preserves current content",
        )
        if (density !== "compact")
          assert.ok(
            await evaluate(
              "document.querySelector('[data-cv-density-status]').textContent.includes('Pending') || document.querySelector('[data-cv-density-status]').textContent.includes('pendente')",
            ),
          )
        await click(generate)
        await until("!!document.querySelector('[data-generated-summary]')")
        assert.equal(await evaluate("window.__outbound.density"), density)
        await click(accept)
        await until(
          `JSON.parse(localStorage.getItem('careeros_curated_cv_v1')).density==='${density}'`,
        )
        const text = await evaluate(
          "document.querySelector('.cv-paper-content').textContent",
        )
        lengths.push(text.length)
        assert.equal(await evaluate(savedProfileExpression), originalProfile)
        await call("Emulation.setDeviceMetricsOverride", {
          width: 390,
          height: 844,
          deviceScaleFactor: 1,
          mobile: false,
        })
        await pause(50)
        assert.ok(
          await evaluate("document.documentElement.scrollWidth<=innerWidth"),
        )
        assert.equal(
          await evaluate(
            "document.querySelectorAll('[data-cv-density] input').length",
          ),
          3,
        )
        await call("Page.captureScreenshot", {
          format: "png",
          captureBeyondViewport: true,
        }).then((r) =>
          writeFileSync(
            join(work, `${fixture.locale}-${size}-${density}-390.png`),
            Buffer.from(r.data, "base64"),
          ),
        )
        await call("Emulation.setDeviceMetricsOverride", {
          width: 1440,
          height: 1000,
          deviceScaleFactor: 1,
          mobile: false,
        })
        await pause(50)
        await call("Page.captureScreenshot", {
          format: "png",
          captureBeyondViewport: true,
        }).then((r) =>
          writeFileSync(
            join(work, `${fixture.locale}-${size}-${density}-desktop.png`),
            Buffer.from(r.data, "base64"),
          ),
        )
        const overflow = await evaluate(
          "Array.from(document.querySelectorAll('.cv-controls [role=status]')).some(e=>e.textContent.includes('extends about') || e.textContent.includes('continua por cerca'))",
        )
        if (size === "heavy" && density === "detailed") {
          await evaluate("window.__prints=0;window.print=()=>window.__prints++")
          await click(
            fixture.locale === "en" ? "Save as PDF" : "Salvar como PDF",
          )
          await pause(50)
          assert.equal(
            await evaluate("window.__prints"),
            0,
            "ordinary app export blocks overflowing content",
          )
        }
        // Direct CDP printing intentionally bypasses the app's overflow gate for inspection.
        const pdf = await call("Page.printToPDF", {
          printBackground: true,
          preferCSSPageSize: true,
        })
        const pdfPath = join(work, `${fixture.locale}-${size}-${density}.pdf`)
        writeFileSync(pdfPath, Buffer.from(pdf.data, "base64"))
        const info = execFileSync("pdfinfo", [pdfPath], { encoding: "utf8" })
        const pages = Number(info.match(/Pages:\s+(\d+)/)[1])
        assert.match(info, /Page size:[^\n]+\(A4\)/)
        const pdfText = execFileSync("pdftotext", ["-raw", pdfPath, "-"], {
          encoding: "utf8",
        })
        assert.ok(
          pdfText.toLowerCase().includes(profile.fullName.toLowerCase()),
        )
        if (size === "short") assert.equal(pages, 1)
        if (size === "heavy" && density === "detailed") {
          assert.ok(pages > 1)
          assert.ok(overflow, "detailed overflow is visible")
        }
        console.log(
          `${fixture.locale}/${size}/${density}: ${text.length} preview chars, ${pages} A4 pages, overflow=${overflow}`,
        )
      }
      assert.ok(
        lengths[0] < lengths[1] && lengths[1] < lengths[2],
        `distinct content volumes: ${lengths}`,
      )
      // Font changes preserve density; density changes preserve font and manual edits.
      await evaluate(
        `{const el=document.querySelector('input[type=range]');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(el,'15');el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}))}`,
      )
      await editSummary("Manual wording stays until explicit replacement.")
      await pause(50)
      const edited = await evaluate("localStorage.getItem('careeros_cv_v1')")
      const saved = await evaluate(
        "localStorage.getItem('careeros_curated_cv_v1')",
      )
      await evaluate(
        `document.querySelector('[data-cv-density] input[value=compact]').click()`,
      )
      assert.equal(
        await evaluate("localStorage.getItem('careeros_cv_v1')"),
        edited,
      )
      assert.equal(
        await evaluate(
          "JSON.parse(localStorage.getItem('careeros_cv_preferences_v1')).fontSize",
        ),
        15,
      )
      await click(generate)
      await until("!!document.querySelector('[data-generated-summary]')")
      await evaluate(
        `document.querySelector('[data-cv-density] input[value=balanced]').click()`,
      )
      await until("!document.querySelector('[data-generated-summary]')")
      assert.equal(
        await evaluate("localStorage.getItem('careeros_curated_cv_v1')"),
        saved,
      )
      const calls = await evaluate("window.__calls")
      await evaluate(
        `window.fetch=async()=>{window.__calls++;return new Response(JSON.stringify({error:'rate_limit'}),{status:429})}`,
      )
      await click(generate)
      await until(
        "!!document.querySelector('[data-cv-generation] [role=alert]')",
      )
      await pause(100)
      assert.equal(
        await evaluate("window.__calls"),
        calls + 1,
        "no automatic retry",
      )
      await click(generate)
      await pause(50)
      assert.equal(
        await evaluate("window.__calls"),
        calls + 2,
        "deliberate retry",
      )
      await evaluate("document.querySelectorAll('nav button')[0].click()")
      await evaluate("document.querySelectorAll('nav button')[2].click()")
      await until("!!document.querySelector('.cv-paper-content')")
      assert.equal(
        await evaluate(
          "document.querySelector('[data-cv-density] input:checked').value",
        ),
        "balanced",
      )
      await evaluate("location.reload()")
      await until("!!document.querySelector('header button')")
      await evaluate(
        "document.querySelector('header button:last-child').click()",
      )
      await until("document.querySelectorAll('nav button').length===4")
      await evaluate("document.querySelectorAll('nav button')[2].click()")
      await until("!!document.querySelector('.cv-paper-content')")
      assert.equal(
        await evaluate(
          "document.querySelector('[data-cv-density] input:checked').value",
        ),
        "balanced",
      )
      assert.equal(
        await evaluate("document.querySelector('input[type=range]').value"),
        "15",
      )
      assert.equal(
        await evaluate("document.querySelector('textarea').value"),
        "Manual wording stays until explicit replacement.",
      )
      assert.equal(await evaluate(savedProfileExpression), originalProfile)
    }
  }
  await openCv({ ...blank, education: fixtures[0].repo.education }, "en")
  await evaluate(
    `window.fetch=async(url,options)=>{const facts=JSON.parse(options.body).facts;const degree=facts.find(f=>f.field==='degree');return new Response(JSON.stringify({summary:[{sourceId:degree.id,text:'Education includes a BSc Design.'}],selected:facts.map(f=>f.id)}),{status:200})}`,
  )
  await click("Generate CV")
  await until("!!document.querySelector('[data-generated-summary]')")
  await click("Accept and replace CV")
  await until("!!localStorage.getItem('careeros_curated_cv_v1')")
  assert.equal(
    await evaluate(
      "document.querySelectorAll('.cv-paper-content [data-cv-sample]').length",
    ),
    0,
    "sparse generated CV must not introduce sample experience, skills or name",
  )
  await openCv(blank, "en")
  assert.ok(
    await evaluate(
      "Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()==='Generate CV').disabled",
    ),
  )
  console.log(
    "Empty guidance; stale edit and provider error paths preserve valid CV; no real provider calls",
  )
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
  console.log(`Controlled CV generation review files: ${work}`)
}
