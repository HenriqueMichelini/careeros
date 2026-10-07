// Browser regression for raw professional-information review and explicit apply.
// Uses synthetic API responses; no provider request is made.
import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-ingestion-browser-"))
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
  const openProfile = async () => {
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('header nav button').length === 4")
    await evaluate("document.querySelectorAll('header nav button')[1].click()")
    await until("!!document.querySelector('#ingestion-text')")
  }
  const fill = async (value, selector = '#ingestion-text') => {
    await evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, ${JSON.stringify(value)}); el.dispatchEvent(new Event('input', { bubbles: true })); })()`)
    await pause(60)
  }
  const saved = () => evaluate("localStorage.getItem('careeros_repo')")
  const byText = text => `Array.from(document.querySelectorAll('button')).find(el=>el.textContent.trim()===${JSON.stringify(text)})`
  const fixture = {
    claims: [{id:'c1',source:'Ada',text:'Professional facts supplied by Ada',targets:['fullName','email','phone','location','professionalLinks','education','certifications','languages','experience','projects','skills'],question:''},
      {id:'c2',source:'English',text:'English proficiency needs clarification',targets:[],question:'Intermediate or fluent?'}],
    operations: [
      ...Object.entries({fullName:'Ada',email:'ada@example.test',phone:'+55 11 5555',location:'São Paulo',professionalLinks:'https://example.test/ada',skills:'Go'}).map(([target,value])=>({claimId:'c1',target,entryId:'',field:target,action:target==='skills'?'add':'update',value,finding:'addition'})),
      ...Object.entries({education:{degree:'BSc',institution:'North',graduationDate:'2021'},certifications:{name:'Cloud',issuer:'Guild'},languages:{name:'Portuguese',proficiency:'Fluent'},experience:{company:'Aster',title:'Engineer'},projects:{name:'Harbor'}}).flatMap(([target,fields])=>Object.entries(fields).map(([field,value])=>({claimId:'c1',target,entryId:'new:c1',field,action:'add',value,finding:'addition'}))),
    ],unverifiedClaimCount:1,unresolvedClaimIds:[],unplacedOperationCount:0,
  }
  for (const locale of ['en','pt-BR']) for (const width of [1440,390]) for (const populated of [false,true]) {
    await call('Emulation.setDeviceMetricsOverride',{width,height:1000,deviceScaleFactor:1,mobile:false})
    await evaluate(`localStorage.setItem('careeros_locale', ${JSON.stringify(locale)}); localStorage.setItem('careeros_apikey','sk-synthetic-test'); ${populated ? `localStorage.setItem('careeros_repo',${JSON.stringify(JSON.stringify(repo))})` : "localStorage.removeItem('careeros_repo')"}; location.reload()`)
    await until("document.readyState === 'complete' && !!document.querySelector('header button')")
    await openProfile()
    const initial=await saved()
    assert.equal(await evaluate("document.querySelector('#ingestion-text').hasAttribute('maxlength')"),false)
    assert.equal(await evaluate("document.querySelector('#ingestion-limits')"),null)
    await evaluate(`window.__calls=[]; window.fetch=async(url,options)=>{if(url!='/api/profile/ingest')throw new Error('Unexpected request');window.__calls.push(JSON.parse(options.body));return new Response(${JSON.stringify(JSON.stringify(fixture))},{status:200,headers:{'Content-Type':'application/json'}})}`)
    const review=locale==='en'?'Review suggested changes':'Revisar alterações sugeridas'
    const discard=locale==='en'?'Discard':'Descartar'
    const keep=locale==='en'?'Keep editing':'Continuar editando'
    await fill('Unsaved career notes')
    await evaluate(`Array.from(document.querySelectorAll('button')).find(el => !el.closest('dialog') && el.textContent.trim() === ${JSON.stringify(discard)}).click()`)
    await until("document.querySelector('dialog').open")
    assert.equal(await evaluate("document.activeElement.textContent.trim()"),keep)
    await evaluate(`document.querySelector('dialog button').click()`)
    assert.equal(await evaluate("document.querySelector('dialog').open"),false)
    assert.equal(await evaluate("document.querySelector('#ingestion-text').value"),'Unsaved career notes')
    await evaluate(`Array.from(document.querySelectorAll('button')).find(el => !el.closest('dialog') && el.textContent.trim() === ${JSON.stringify(discard)}).click()`)
    await evaluate("document.querySelector('dialog button:last-child').click()")
    assert.equal(await evaluate("document.querySelector('#ingestion-text').value"),'')
    assert.equal(await saved(),initial)
    await fill('ação🙂 \n'.repeat(2500)+'x')
    assert.equal(await evaluate(`${byText(review)}.disabled`),true)
    await fill('Ada English\n'+('messy notes; Go; BSc North 2021; Cloud Guild; English fluent.\n'.repeat(300)))
    await evaluate(`${byText(review)}.click()`)
    const apply=locale==='en'?'Apply approved changes':'Aplicar alterações aprovadas'
    await until(`${byText(apply)} !== undefined`)
    assert.equal(await saved(),initial)
    assert.equal(await evaluate("document.querySelectorAll('article input:checked').length"),0)
    assert.match(await evaluate("document.querySelector('main').textContent"),/Intermediate or fluent/)
    const reject=locale==='en'?'Reject linked changes':'Rejeitar alterações ligadas'
    const approve=locale==='en'?'Approve linked changes':'Aprovar alterações ligadas'
    await evaluate(`${byText(approve)}.click()`)
    await evaluate(`${byText(reject)}.click()`)
    assert.equal(await evaluate(`${byText(apply)}.disabled`),true)
    assert.equal(await saved(),initial)
    await evaluate(`${byText(approve)}.click()`)
    // The first proposal is full name; edit it to exercise controlled editing.
    await fill('Ada Reviewed','article textarea')
    const screen=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true})
    writeFileSync(join(work,`review-${locale}-${width}-${populated}.png`),Buffer.from(screen.data,'base64'))
    assert.ok(await evaluate('document.documentElement.scrollWidth <= innerWidth'), 'horizontal overflow')
    await evaluate(`${byText(apply)}.click()`)
    await until("!Array.from(document.querySelectorAll('button')).some(el=>el.textContent.trim()==="+JSON.stringify(apply)+")")
    const after=JSON.parse(await saved())
    assert.equal(after.fullName,'Ada Reviewed')
    assert.equal(after.email,'ada@example.test')
    assert.equal(after.education.at(-1).institution,'North')
    assert.equal(after.certifications.at(-1).name,'Cloud')
    assert.equal(after.languages.at(-1).name,'Portuguese')
    if(populated) assert.equal(after.experience[0].id,'job')
    await evaluate('location.reload()')
    await until("document.readyState === 'complete' && !!document.querySelector('header button')")
    await openProfile()
    assert.equal(await evaluate("document.querySelector('#contact-fullName').value"),'Ada Reviewed')
    assert.equal(await evaluate("document.querySelector('#ingestion-text').value"),'')
    const skills=locale==='en'?'Skills, tools & tech':'Habilidades e tecnologias'
    const reviewAi=locale==='en'?'Review with AI':'Revisar com IA'
    await evaluate(`${byText(skills)}.click()`)
    assert.ok(await evaluate("document.querySelector('aside').textContent").then(text => text.includes((locale==='en'?'Reviews only ':'Revisa apenas ')+skills)))
    const beforeReview=JSON.parse(await saved())
    await evaluate(`window.fetch=async(url,options)=>{if(url!='/api/profile/review')throw new Error('Unexpected request');const body=JSON.parse(options.body);window.__reviewSection=body.changedSection;return new Response(JSON.stringify({updatedRepository:{...body.repository,skills:'Reviewed skills',careerGoals:'Unrequested model edit'},summary:'Reviewed skills'}),{status:200,headers:{'Content-Type':'application/json'}})}`)
    await evaluate(`${byText(reviewAi)}.click()`)
    await until("JSON.parse(localStorage.getItem('careeros_repo')).skills === 'Reviewed skills'")
    assert.equal(await evaluate('window.__reviewSection'),'skills')
    assert.equal(JSON.parse(await saved()).careerGoals,beforeReview.careerGoals)
    const education=locale==='en'?'Education':'Formação'
    await evaluate(`${byText(education)}.click()`)
    assert.equal(await evaluate(`${byText(reviewAi)} !== undefined`),false)
    console.log(`PASS ingestion ${locale} ${width}px ${populated?'populated':'empty'}`)
  }
  console.log('Screenshots:',work)
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}
