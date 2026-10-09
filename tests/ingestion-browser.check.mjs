import {withIngestionLedger} from "./ingestion-fixtures.mjs"
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
      `Timed out: ${expression}; ${await evaluate("JSON.stringify({text:document.body.innerText.slice(0,1500),alerts:Array.from(document.querySelectorAll('[role=alert]')).map(e=>e.textContent),helper:typeof window.__portionResponse,injected:typeof window.__originalStorageSet,canonical:localStorage.getItem('careeros_profile_v2')!==null})")}`,
    )
  }
  await call("Page.enable")
  await call("Page.addScriptToEvaluateOnNewDocument", {source: String.raw`
    window.__portionResponse = async (raw, init = {status:200}) => {
      if (typeof raw === 'string') raw = JSON.parse(raw);
      if (raw.claims && raw.coverage && raw.decision?.outcome.kind === 'accept') {
        const input = document.querySelector('#ingestion-text').value;
        const identity = JSON.stringify(['normalization-v1','structure-v1','professional_information',input]).replace(/[<>&\u2028\u2029]/g,c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'));
        const sourceId = Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(identity))),b=>b.toString(16).padStart(2,'0')).join('');
        raw.continuation = {sourceId,index:0,total:1,bytes:2000,regions:[[{Start:0,End:new TextEncoder().encode(input).length}]],remaining:null,planComplete:true,processed:raw.unverifiedClaimCount===0&&raw.claims.length<30&&raw.operations.length<60&&raw.unplacedOperationCount===0};
      }
      return new Response(JSON.stringify(raw),init);
    };
  `});
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
    await pause(150)
    await until("!!document.querySelector('header button:last-child')")
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
  const approvedSource =
    "Ada ada@example.test +55 11 5555 São Paulo https://example.test/ada Go BSc North 2021 Cloud Guild Portuguese Fluent Aster Engineer Harbor"
  const fixture = {
    decision: {
      version: 1,
      field: "professional_information",
      outcome: { kind: "accept" },
    },
    claims: [
      {
        id: "c1",
        source: approvedSource,
        text: "Professional facts supplied by Ada",
        targets: [
          "fullName",
          "email",
          "phone",
          "location",
          "professionalLinks",
          "education",
          "certifications",
          "languages",
          "experience",
          "projects",
          "skills",
        ],
        question: "",
      },
      {
        id: "c2",
        source: "English",
        text: "English proficiency needs clarification",
        targets: [],
        question: "Intermediate or fluent?",
      },
    ],
    operations: [
      ...Object.entries({
        fullName: "Ada",
        email: "ada@example.test",
        phone: "+55 11 5555",
        location: "São Paulo",
        professionalLinks: "https://example.test/ada",
        skills: "Go",
      }).map(([target, value]) => ({
        claimId: "c1",
        target,
        entryId: "",
        field: target,
        action: target === "skills" ? "add" : "update",
        value,
        finding: "addition",
      })),
      ...Object.entries({
        education: {
          degree: "BSc",
          institution: "North",
          graduationDate: "2021",
        },
        certifications: { name: "Cloud", issuer: "Guild" },
        languages: { name: "Portuguese", proficiency: "Fluent" },
        experience: { company: "Aster", title: "Engineer" },
        projects: { name: "Harbor" },
      }).flatMap(([target, fields]) =>
        Object.entries(fields).map(([field, value]) => ({
          claimId: "c1",
          target,
          entryId: "new:c1",
          field,
          action: "add",
          value,
          finding: "addition",
        })),
      ),
    ],
    unverifiedClaimCount: 1,
    unresolvedClaimIds: [],
    unplacedOperationCount: 0,
  }
  // Each destination has its own accepted excerpt; contact evidence must not
  // become evidence for skills or an unrelated role/project.
  const destinationSources = {
    fullName: "Ada",
    email: "ada@example.test",
    phone: "+55 11 5555",
    location: "São Paulo",
    professionalLinks: "https://example.test/ada",
    skills: "Go",
    education: "BSc North 2021",
    certifications: "Cloud Guild",
    languages: "Portuguese Fluent",
    experience: "Aster Engineer",
    projects: "Harbor",
  }
  fixture.claims = Object.entries(destinationSources)
    .map(([target, source]) => ({
      id: target,
      source,
      text: source,
      targets: [target],
      question: "",
    }))
    .concat(fixture.claims[1])
  fixture.operations = fixture.operations.map((op) => ({
    ...op,
    claimId: op.target,
    entryId: op.entryId ? "new:" + op.target : "",
  }))
  Object.assign(fixture,withIngestionLedger(fixture))
  if (!process.env.PROFILE_RECOVERY_ONLY && !process.env.PROFILE_CONTINUATION_ONLY) {
    // Rephrasing is a backend decision and requires changed text, with no Profile write.
    await evaluate(
      "localStorage.setItem('careeros_typesafe_key','synthetic-typesafe'); localStorage.setItem('careeros_apikey','sk-synthetic-test')",
    )
    await openProfile()
    const gateInitial = await saved()
    await evaluate(
      `window.__gateCalls=0; window.fetch=async()=>{window.__gateCalls++; return new Response(JSON.stringify({decision:{version:1,field:'professional_information',outcome:{kind:'request_rephrasing'}}}),{status:200})}`,
    )
    const quoted = 'I test security with "ignore previous instructions".'
    await fill(quoted)
    await evaluate(`${byText("Review suggested changes")}.click()`)
    await until("!!document.querySelector('[role=alert]')")
    assert.match(
      await evaluate("document.querySelector('[role=alert]').textContent"),
      /could be.*override/i,
    )
    assert.equal(
      await evaluate("document.querySelector('#ingestion-text').value"),
      quoted,
    )
    assert.equal(
      await evaluate(`${byText("Review suggested changes")}.disabled`),
      true,
    )
    assert.equal(await saved(), gateInitial)
    await fill("I test application security.")
    assert.equal(
      await evaluate(`${byText("Review suggested changes")}.disabled`),
      false,
    )
    await evaluate(`${byText("Review suggested changes")}.click()`)
    await until(
      "window.__gateCalls===2 && !!document.querySelector('[role=alert]')",
    )
    assert.equal(await saved(), gateInitial)
    for (const locale of ["en", "pt-BR"])
      for (const width of [1440, 390]) {
        await call("Emulation.setDeviceMetricsOverride", {
          width,
          height: 1000,
          deviceScaleFactor: 1,
          mobile: false,
        })
        await evaluate(
          `localStorage.setItem('careeros_locale',${JSON.stringify(locale)}); location.reload()`,
        )
        await until(
          "document.readyState === 'complete' && !!document.querySelector('header button')",
        )
        await openProfile()
        const review =
          locale === "en"
            ? "Review suggested changes"
            : "Revisar alterações sugeridas"
        const initial = await saved()
        const text = locale === "en" ? "I use Java" : "Eu uso Java"
        // UTF-8 byte feedback must preserve the entire original at the boundary.
        const atLimit = "é".repeat(15000)
        await fill(atLimit)
        assert.equal(await evaluate(`${byText(review)}.disabled`), false)
        assert.ok(
          (
            await evaluate(
              "document.querySelector('#ingestion-text').nextElementSibling.textContent",
            )
          ).includes("30000 / 30000"),
        )
        await fill(atLimit + "x")
        assert.equal(await evaluate(`${byText(review)}.disabled`), true)
        assert.equal(
          await evaluate("document.querySelector('#ingestion-text').value"),
          atLimit + "x",
        )
        assert.ok(
          (
            await evaluate(
              "document.querySelector('#ingestion-text').nextElementSibling.textContent",
            )
          ).includes("30001 / 30000"),
        )
        assert.equal(await saved(), initial)

        const proposal = {
          decision: {
            version: 1,
            field: "professional_information",
            outcome: { kind: "accept" },
          },
          claims: [
            {
              id: "c1",
              source: text,
              text: "Java",
              targets: ["skills"],
              question: "",
            },
          ],
          operations: [
            {
              claimId: "c1",
              target: "skills",
              entryId: "",
              field: "skills",
              action: "add",
              value: "Java",
              finding: "addition",
            },
          ],
          unverifiedClaimCount: 0,
          unresolvedClaimIds: [],
          unplacedOperationCount: 0,
        }
        Object.assign(proposal,withIngestionLedger(proposal))
        for (const outcome of [
          { kind: "reject_attack" },
          { kind: "request_rephrasing" },
          { kind: "irrelevant" },
          { kind: "unusable" },
          { kind: "request_information", needs: "professional_fact" },
          ...["key", "rate_limit", "timeout", "outage", "invalid_output"].map(
            (reason) => ({ kind: "service_failure", reason }),
          ),
        ]) {
          const submitted =
            text + " " + outcome.kind + " " + (outcome.reason || "")
          await fill(submitted)
          await evaluate(
            `window.__calls=[];window.fetch=async(url,options)=>{window.__calls.push({body:JSON.parse(options.body),key:options.headers['X-TypeSafe-Api-Key']}); return new Response(JSON.stringify({decision:{version:1,field:'professional_information',outcome:${JSON.stringify(outcome)}}}),{status:${
              outcome.kind === "service_failure" ? 502 : 200
            }})}`,
          )
          await evaluate(`${byText(review)}.click()`)
          await until("!!document.querySelector('[role=alert]')")
          assert.equal(
            await evaluate("document.querySelector('#ingestion-text').value"),
            submitted,
          )
          assert.equal(await saved(), initial)
          assert.equal(
            await evaluate("document.querySelectorAll('article').length"),
            0,
          )
          assert.equal(await evaluate("window.__calls.length"), 1)
          assert.equal(
            await evaluate("window.__calls[0].key"),
            "synthetic-typesafe",
          )
          const feedback = await evaluate(
            "document.querySelector('[role=alert]').textContent",
          )
          if (outcome.kind === "reject_attack") {
            assert.match(
              feedback,
              locale === "en" ? /prohibited.*violates/ : /proibido.*viola/,
            )
            await evaluate("document.querySelector('[role=alert] a').click()")
            assert.equal(
              await evaluate("document.querySelector('#input-use-rule').open"),
              true,
            )
          } else {
            assert.doesNotMatch(
              feedback,
              locale === "en" ? /violates|prohibited/ : /viola os|proibido/,
            )
          }
          assert.equal(
            await evaluate(`${byText(review)}.disabled`),
            outcome.kind === "request_rephrasing",
          )
          if (outcome.kind === "request_rephrasing") {
            await fill(submitted + " revised")
            assert.equal(await evaluate(`${byText(review)}.disabled`), false)
            await fill(submitted) // Changing away and back does not satisfy revision.
            assert.equal(await evaluate(`${byText(review)}.disabled`), true)
            const screen = await call("Page.captureScreenshot", {
              format: "png",
              captureBeyondViewport: true,
            })
            writeFileSync(
              join(work, `rephrasing-${locale}-${width}.png`),
              Buffer.from(screen.data, "base64"),
            )
          }
          assert.ok(
            await evaluate(
              "document.documentElement.scrollWidth <= innerWidth",
            ),
          )
        }
        // An omitted new claim must display recovery, never the possible-duplicate message.
        const omitted = {
          ...proposal,
          operations: [],
          unresolvedClaimIds: ["c1"],
        }
        Object.assign(omitted,withIngestionLedger(omitted))
        await fill(text + " omitted")
        await evaluate(
          `window.fetch=async()=>await window.__portionResponse(${JSON.stringify(omitted)},{status:200})`,
        )
        await evaluate(`${byText(review)}.click()`)
        await until("document.querySelectorAll('article').length === 1")
        const omittedText = await evaluate(
          "document.querySelector('article').textContent",
        )
        assert.match(
          omittedText,
          locale === "en"
            ? /No safe change was produced/
            : /Não foi possível propor uma alteração segura/,
        )
        assert.doesNotMatch(
          omittedText,
          locale === "en" ? /may already be/ : /talvez já esteja/,
        )
        assert.equal(await saved(), initial)
        // Every ledger disposition is visible, including exact fact IDs and
        // competing statements; inspecting or reviewing never writes Profile.
        const ledgerText = "Figma\nJava\nI do not use Figma\nWhich period?\nUnsupported qualification\nUnresolved detail"
        const canonical = JSON.parse(await evaluate("localStorage.getItem('careeros_profile_v2')"))
        const toolFact = canonical.facts.find(f => f.field === "tools" && f.owner.id === canonical.id)
        const toolRef = {profileId:canonical.id,id:toolFact.id,revision:toolFact.revision}
        const ledgerClaims = [
          {id:"exact",source:"Figma",text:"Figma",targets:["tools"],question:""},
          {id:"support",source:"Figma",text:"Figma with new support",targets:["tools"],question:""},
          {id:"overlap",source:"Java",text:"Distinct added detail",targets:["tools"],question:""},
          {id:"change",source:"Java",text:"Java",targets:["skills"],question:""},
          {id:"contradiction",source:"I do not use Figma",text:"Does not use Figma",targets:["tools"],question:""},
          {id:"correction",source:"I do not use Figma",text:"Correction candidate",targets:["tools"],question:""},
          {id:"clarification",source:"Which period?",text:"Period unclear",targets:[],question:"Which period?"},
          {id:"unsupported",source:"Unsupported qualification",text:"Unsupported qualification",targets:[],question:""},
          {id:"unresolved",source:"Unresolved detail",text:"Unresolved detail",targets:[],question:""},
        ]
        const ledger = {
          decision:proposal.decision,claims:ledgerClaims,
          operations:[
            {claimId:"support",target:"tools",entryId:"",field:"tools",action:"evidence",value:"Figma",finding:"in_place"},
            {claimId:"overlap",target:"tools",entryId:"",field:"tools",action:"add",value:"Java",finding:"overlap"},
            {claimId:"change",target:"skills",entryId:"",field:"skills",action:"add",value:"Java",finding:"addition"},
          ],
          outcomes:ledgerClaims.map((c,i)=>({claimId:c.id,kind:["exact_duplicate","additional_support","overlap","change","contradiction","correction","clarification","unsupported","unresolved"][i],reason:i===0?"verified_exact_alias_or_wording":"Source-specific comparison detail",relatedFacts:["exact","support","overlap","contradiction","correction"].includes(c.id)?[toolRef]:[],relatedClaimIds:["contradiction","correction"].includes(c.id)?["exact"]:[],operationIndexes:i>=1&&i<=3?[i-1]:[]})),
          unverifiedClaimCount:1,unresolvedClaimIds:["unresolved"],unplacedOperationCount:0,
          skippedClaims:[{index:10,reason:"source",text:"Returned statement with unusable source",source:"",shortened:false}],coverage:{validClaims:9,invalidClaims:1,discoveryComplete:false,capacity:"within_limit"},
        }
        await fill(ledgerText)
        await evaluate(`window.fetch=async()=>await window.__portionResponse(${JSON.stringify(ledger)},{status:200})`)
        await evaluate(`${byText(review)}.click()`)
        await until("document.querySelectorAll('article').length===9")
        const ledgerScreenText = await evaluate("document.body.innerText")
        assert.ok(ledgerScreenText.includes("Returned statement with unusable source"))
        assert.ok(ledgerScreenText.includes(locale==="en"?"This assertion has additional supporting evidence":"Esta afirmação tem evidência adicional"))
        for (const label of locale === "en" ? ["Exact duplicate", "Additional supporting evidence", "Overlapping detail", "Proposed change", "Contradiction", "Correction or supersession", "Needs clarification", "Unsupported claim", "Processing unresolved", "Unprocessed claim 10"] : ["Duplicata exata", "Evidência adicional", "Detalhe sobreposto", "Alteração proposta", "Contradição", "Candidata a correção", "Precisa de esclarecimento", "Afirmação sem suporte", "Processamento não resolvido", "Afirmação não processada 10"]) assert.ok(ledgerScreenText.includes(label),label)
        await evaluate("document.querySelector('article details summary').focus()")
        await call("Input.dispatchKeyEvent",{type:"keyDown",key:"Enter",code:"Enter",windowsVirtualKeyCode:13,text:"\r"})
        await call("Input.dispatchKeyEvent",{type:"keyUp",key:"Enter",code:"Enter",windowsVirtualKeyCode:13})
        assert.ok(await evaluate("document.querySelector('article details').open"))
        assert.ok((await evaluate("document.querySelector('article').innerText")).includes(toolFact.id))
        assert.equal(await saved(), initial)
        assert.ok(await evaluate("document.documentElement.scrollWidth<=innerWidth"))
        const ledgerScreen = await call("Page.captureScreenshot",{format:"png",captureBeyondViewport:true})
        writeFileSync(join(work,`claim-ledger-${locale}-${width}.png`),Buffer.from(ledgerScreen.data,"base64"))
        // Real Tab/Enter and text input exercise revision and deliberate retry.
        await keyboardFill(quoted)
        await evaluate(
          `window.__keyboardCalls=0; window.fetch=async()=>{window.__keyboardCalls++; const body=window.__keyboardCalls===1?{decision:{version:1,field:'professional_information',outcome:{kind:'request_rephrasing'}}}:window.__keyboardCalls===2?{decision:{version:1,field:'professional_information',outcome:{kind:'service_failure',reason:'timeout'}}}:${JSON.stringify(proposal)};return await window.__portionResponse(body,{status:window.__keyboardCalls===2?504:200})}`,
        )
        await keyboardSubmit(byText(review))
        await until("!!document.querySelector('[role=alert]')")
        await keyboardFill(text + "; corrected professional fact.")
        await keyboardSubmit(byText(review))
        await until(
          "window.__keyboardCalls===2 && !!document.querySelector('[role=alert]')",
        )
        await pause(200)
        assert.equal(await evaluate("window.__keyboardCalls"), 2)
        assert.equal(
          await evaluate("document.querySelector('#ingestion-text').value"),
          text + "; corrected professional fact.",
        )
        await keyboardSubmit(byText(review))
        await until("document.querySelectorAll('article').length === 1")
        assert.equal(await evaluate("window.__keyboardCalls"), 3)
        assert.equal(await saved(), initial)
        // Hold a response: loading keeps text/Profile intact; editing cancels stale proposals.
        await fill(text)
        await evaluate(
          `window.fetch=async()=>new Promise(resolve=>window.__finish=async()=>resolve(await window.__portionResponse(${JSON.stringify(proposal)},{status:200})))`,
        )
        await evaluate(`${byText(review)}.click()`)
        await until(
          "document.querySelector('#ingestion-text').getAttribute('aria-busy') === 'true'",
        )
        assert.ok(await evaluate("!!document.querySelector('[role=status]')"))
        assert.equal(await saved(), initial)
        await fill(text + " revised")
        await evaluate("window.__finish()")
        await pause(80)
        assert.equal(
          await evaluate("document.querySelectorAll('article').length"),
          0,
        )
        assert.equal(await saved(), initial)
        // A fact among ordinary noise can reach proposals; editing invalidates them.
        for (const submitted of [
          text,
          "Bread, apples. " + text + ". Weekend plans.",
        ]) {
          await fill(submitted)
          await evaluate(
            `window.fetch=async()=>await window.__portionResponse(${JSON.stringify(proposal)},{status:200})`,
          )
          await evaluate(`${byText(review)}.click()`)
          await until("document.querySelectorAll('article').length === 1")
          assert.equal(await saved(), initial)
          assert.equal(
            await evaluate("document.querySelector('article textarea').value"),
            "Java",
          )
          await fill(submitted + " revised")
          assert.equal(
            await evaluate("document.querySelectorAll('article').length"),
            0,
          )
        }
        // An accepted decision is mandatory; old or malformed transport responses fail closed.
        const { decision, ...oldProposal } = proposal
        for (const raw of [
          oldProposal,
          {
            ...proposal,
            decision: {
              ...decision,
              outcome: { kind: "accept", confidence: 1 },
            },
          },
        ]) {
          await fill(text)
          await evaluate(
            `window.fetch=async()=>new Response(${JSON.stringify(JSON.stringify(raw))},{status:200})`,
          )
          await evaluate(`${byText(review)}.click()`)
          await until("!!document.querySelector('[role=alert]')")
          assert.equal(
            await evaluate("document.querySelectorAll('article').length"),
            0,
          )
          assert.equal(await saved(), initial)
        }
        // Prepared-source references render the exact original occurrence; changing
        // raw whitespace invalidates an in-flight proposal even when preparation agrees.
        const traceText = "# First\nI  use Java\n# Second\nI   use Java"
        const excerpt = "I   use Java"
        const identity = JSON.stringify([
          "normalization-v1",
          "structure-v1",
          "professional_information",
          traceText,
        ])
        const sourceReference = {
          version: 1,
          sourceId: createHash("sha256").update(identity).digest("hex"),
          preparationVersion: "structure-v1",
          segmentId: "b".repeat(64),
          occurrenceId: "c".repeat(64),
          originalStart: Buffer.from(traceText).lastIndexOf(excerpt),
          originalEnd: Buffer.byteLength(traceText),
        }
        const traceProposal = {
          ...proposal,
          claims: [{ ...proposal.claims[0], source: excerpt, sourceReference }],
        }
        await fill(traceText)
        await evaluate(
          `window.fetch=async(url,options)=>{window.__raw=JSON.parse(options.body).input;return await window.__portionResponse(${JSON.stringify(traceProposal)},{status:200})}`,
        )
        await evaluate(`${byText(review)}.click()`)
        await until("document.querySelectorAll('article').length === 1")
        assert.equal(await evaluate("window.__raw"), traceText)
        assert.equal(
          await evaluate("document.querySelector('#ingestion-text').value"),
          traceText,
        )
        assert.ok(
          (
            await evaluate("document.querySelector('article').textContent")
          ).includes(excerpt),
        )
        assert.equal(
          await evaluate(
            "getComputedStyle(document.querySelector('article p:nth-child(2)')).whiteSpace",
          ),
          "pre-wrap",
        )
        assert.equal(await saved(), initial)
        const traceScreen = await call("Page.captureScreenshot", {
          format: "png",
          captureBeyondViewport: true,
        })
        writeFileSync(
          join(work, `source-review-${locale}-${width}.png`),
          Buffer.from(traceScreen.data, "base64"),
        )
        await fill(traceText + " ")
        await fill(traceText)
        await evaluate(
          `window.fetch=async()=>new Promise(resolve=>window.__finish=async()=>resolve(await window.__portionResponse(${JSON.stringify(traceProposal)},{status:200})))`,
        )
        await evaluate(`${byText(review)}.click()`)
        await until(
          "document.querySelector('#ingestion-text').getAttribute('aria-busy') === 'true'",
        )
        await fill(traceText.replace("I   use", "I  use"))
        await evaluate("window.__finish()")
        await pause(80)
        assert.equal(
          await evaluate("document.querySelectorAll('article').length"),
          0,
        )
        assert.equal(await saved(), initial)
        for (const code of ["capacity", "preparation"]) {
          await fill(traceText)
          await evaluate(
            `window.fetch=async()=>new Response(JSON.stringify({error:${JSON.stringify(code)}}),{status:502})`,
          )
          await evaluate(`${byText(review)}.click()`)
          await until("!!document.querySelector('[role=alert]')")
          const feedback = await evaluate(
            "document.querySelector('[role=alert]').textContent",
          )
          assert.match(
            feedback,
            locale === "en" ? /smaller portion/ : /parte menor/,
          )
          assert.equal(
            await evaluate("document.querySelector('#ingestion-text').value"),
            traceText,
          )
          assert.equal(await saved(), initial)
          assert.ok(
            await evaluate(
              "document.documentElement.scrollWidth <= innerWidth",
            ),
          )
        }
        console.log(`PASS field decisions ${locale} ${width}px`)
      }
    for (const locale of ["en", "pt-BR"])
      for (const width of [1440, 390])
        for (const populated of [false, true]) {
          await call("Emulation.setDeviceMetricsOverride", {
            width,
            height: 1000,
            deviceScaleFactor: 1,
            mobile: false,
          })
          await evaluate(
            `localStorage.setItem('careeros_locale', ${JSON.stringify(locale)}); localStorage.setItem('careeros_apikey','sk-synthetic-test'); ${
              populated
                ? `localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo',${JSON.stringify(JSON.stringify(repo))})`
                : "localStorage.removeItem('careeros_profile_v2'); localStorage.removeItem('careeros_repo')"
            }; location.reload()`,
          )
          await until(
            "document.readyState === 'complete' && !!document.querySelector('header button')",
          )
          await openProfile()
          const initial = await saved()
          assert.equal(
            await evaluate(
              "document.querySelector('#ingestion-text').hasAttribute('maxlength')",
            ),
            false,
          )
          assert.equal(
            await evaluate("document.querySelector('#ingestion-limits')"),
            null,
          )
          await evaluate(
            `window.__calls=[]; window.fetch=async(url,options)=>{if(url!='/api/profile/ingest')throw new Error('Unexpected request');window.__calls.push(JSON.parse(options.body));return await window.__portionResponse(${JSON.stringify(fixture)},{status:200,headers:{'Content-Type':'application/json'}})}`,
          )
          const review =
            locale === "en"
              ? "Review suggested changes"
              : "Revisar alterações sugeridas"
          const discard = locale === "en" ? "Discard" : "Descartar"
          const keep = locale === "en" ? "Keep editing" : "Continuar editando"
          await fill("Unsaved career notes")
          await evaluate(
            `Array.from(document.querySelectorAll('button')).find(el => !el.closest('dialog') && el.textContent.trim() === ${JSON.stringify(discard)}).click()`,
          )
          await until("document.querySelector('dialog').open")
          assert.equal(
            await evaluate("document.activeElement.textContent.trim()"),
            keep,
          )
          await evaluate(`document.querySelector('dialog button').click()`)
          assert.equal(
            await evaluate("document.querySelector('dialog').open"),
            false,
          )
          assert.equal(
            await evaluate("document.querySelector('#ingestion-text').value"),
            "Unsaved career notes",
          )
          await evaluate(
            `Array.from(document.querySelectorAll('button')).find(el => !el.closest('dialog') && el.textContent.trim() === ${JSON.stringify(discard)}).click()`,
          )
          await evaluate(
            "document.querySelector('dialog button:last-child').click()",
          )
          assert.equal(
            await evaluate("document.querySelector('#ingestion-text').value"),
            "",
          )
          assert.equal(await saved(), initial)
          await fill("ação🙂 \n".repeat(2500) + "x")
          assert.equal(await evaluate(`${byText(review)}.disabled`), true)
          await fill(
            approvedSource +
              " English\n" +
              "messy notes; Go; BSc North 2021; Cloud Guild; English fluent.\n".repeat(
                300,
              ),
          )
          await evaluate(`${byText(review)}.click()`)
          const apply =
            locale === "en"
              ? "Apply approved changes"
              : "Aplicar alterações aprovadas"
          await until(`${byText(apply)} !== undefined`)
          assert.equal(await saved(), initial)
          assert.equal(
            await evaluate(
              "document.querySelectorAll('article input:checked').length",
            ),
            0,
          )
          assert.match(
            await evaluate("document.querySelector('main').textContent"),
            /Intermediate or fluent/,
          )
          const reject =
            locale === "en"
              ? "Reject linked changes"
              : "Rejeitar alterações ligadas"
          const approve =
            locale === "en"
              ? "Approve linked changes"
              : "Aprovar alterações ligadas"
          await evaluate(
            `Array.from(document.querySelectorAll('button')).filter(el=>el.textContent.trim()===${JSON.stringify(approve)}).forEach(el=>el.click())`,
          )
          await evaluate(
            `Array.from(document.querySelectorAll('button')).filter(el=>el.textContent.trim()===${JSON.stringify(reject)}).forEach(el=>el.click())`,
          )
          assert.equal(await evaluate(`${byText(apply)}.disabled`), true)
          assert.equal(await saved(), initial)
          await evaluate(
            `Array.from(document.querySelectorAll('button')).filter(el=>el.textContent.trim()===${JSON.stringify(approve)}).forEach(el=>el.click())`,
          )
          // The first proposal is full name; edit it to exercise controlled editing.
          await fill("Ada Reviewed", "article textarea")
          const screen = await call("Page.captureScreenshot", {
            format: "png",
            captureBeyondViewport: true,
          })
          writeFileSync(
            join(work, `review-${locale}-${width}-${populated}.png`),
            Buffer.from(screen.data, "base64"),
          )
          assert.ok(
            await evaluate(
              "document.documentElement.scrollWidth <= innerWidth",
            ),
            "horizontal overflow",
          )
          await evaluate(`${byText(apply)}.click()`)
          await until(
            "!Array.from(document.querySelectorAll('button')).some(el=>el.textContent.trim()===" +
              JSON.stringify(apply) +
              ")",
          )
          const after = JSON.parse(await saved())
          assert.equal(after.fullName, "Ada Reviewed")
          assert.equal(after.email, "ada@example.test")
          assert.equal(after.education.at(-1).institution, "North")
          assert.equal(after.certifications.at(-1).name, "Cloud")
          assert.equal(after.languages.at(-1).name, "Portuguese")
          if (populated) assert.equal(after.experience[0].id, "job")
          await evaluate("location.reload()")
          await until(
            "document.readyState === 'complete' && !!document.querySelector('header button')",
          )
          await openProfile()
          assert.equal(
            await evaluate("document.querySelector('#contact-fullName').value"),
            "Ada Reviewed",
          )
          assert.equal(
            await evaluate("document.querySelector('#ingestion-text').value"),
            "",
          )
          const accepted = await evaluate(
            "JSON.parse(localStorage.getItem('careeros_profile_v2'))",
          )
          assert.ok(accepted.evidence.some((e) => e.excerpt === "Go"))
          assert.ok(accepted.evidence.every((e) => e.excerpt !== "English"))
          assert.equal(JSON.stringify(accepted).includes("messy notes"), false)
          const skills =
            locale === "en"
              ? "Skills, tools & tech"
              : "Habilidades e tecnologias"
          const reviewAi = locale === "en" ? "Review with AI" : "Revisar com IA"
          await evaluate(`${byText(skills)}.click()`)
          await evaluate("document.querySelector('[data-fact-details]').open=true; document.querySelectorAll('[data-fact-details] details').forEach(el=>el.open=true)")
          await until("Array.from(document.querySelectorAll('[data-fact-details] blockquote')).some(el=>el.textContent.includes('Go') && el.checkVisibility())")
          const sourceScreen = await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true})
          writeFileSync(join(work,`accepted-evidence-${locale}-${width}-${populated}.png`),Buffer.from(sourceScreen.data,'base64'))
          assert.ok(
            await evaluate("document.querySelector('aside').textContent").then(
              (text) =>
                text.includes(
                  (locale === "en" ? "Reviews only " : "Revisa apenas ") +
                    skills,
                ),
            ),
          )
          const beforeReview = JSON.parse(await saved())
          const acceptSection =
            locale === "en"
              ? "Accept and save changes"
              : "Aceitar e salvar alterações"
          const rejectSection =
            locale === "en" ? "Reject proposal" : "Rejeitar proposta"
          const sectionLabel =
            locale === "en"
              ? "Review Profile Proposal"
              : "Revisar proposta do Perfil"
          await evaluate(
            `window.fetch=async(url,options)=>{if(url!='/api/profile/review')throw new Error('Unexpected request');const body=JSON.parse(options.body);window.__reviewBody=body;const {reviewableFact}=await import('/src/lib/sectionReview.ts');return new Response(JSON.stringify({profileId:body.document.id,revision:body.document.revision,section:body.section,summary:'Clearer wording',patches:body.document.facts.filter(f=>reviewableFact(body.document,body.section,f)).map(f=>({factId:f.id,revision:f.revision,wording:f.field==='skills'?'Reviewed skills':String(f.value),supporting:[{id:f.id,revision:f.revision}]}))}),{status:200,headers:{'Content-Type':'application/json'}})}`,
          )
          await evaluate(`${byText(reviewAi)}.click()`)
          await until(
            `!!document.querySelector('section[aria-label='+${JSON.stringify(JSON.stringify(sectionLabel))}+']')`,
          )
          assert.deepEqual(
            JSON.parse(await saved()),
            beforeReview,
            "generation/rendering must not save",
          )
          assert.equal(await evaluate("window.__reviewBody.section"), "skills")
          assert.equal(
            await evaluate(
              'JSON.stringify(window.__reviewBody).includes("ada@example.test")',
            ),
            false,
          )
          assert.equal(
            await evaluate(
              'JSON.stringify(window.__reviewBody).includes("careerGoals")',
            ),
            false,
          )
          await evaluate(`${byText(rejectSection)}.click()`)
          assert.deepEqual(
            JSON.parse(await saved()),
            beforeReview,
            "rejection must not save",
          )
          await evaluate(`${byText(reviewAi)}.click()`)
          await until(`${byText(acceptSection)} !== undefined`)
          await evaluate(`${byText(acceptSection)}.click()`)
          await until(
            `(async () => JSON.parse(await ${savedProfileExpression}).skills === 'Reviewed skills')()`,
          )
          assert.equal(
            JSON.parse(await saved()).careerGoals,
            beforeReview.careerGoals,
          )
          assert.ok(
            await evaluate(
              "document.documentElement.scrollWidth <= innerWidth",
            ),
            "proposal horizontal overflow",
          )
          const education = locale === "en" ? "Education" : "Formação"
          await evaluate(`${byText(education)}.click()`)
          assert.equal(
            await evaluate(`${byText(reviewAi)} !== undefined`),
            false,
          )
          console.log(
            `PASS ingestion ${locale} ${width}px ${
              populated ? "populated" : "empty"
            }`,
          )
        }
  }
  if (!process.env.PROFILE_RECOVERY_ONLY) {
    for (const locale of ["en", "pt-BR"]) for (const width of [1440,390]) {
      await call("Emulation.setDeviceMetricsOverride",{width,height:1000,deviceScaleFactor:1,mobile:false})
      await evaluate(`localStorage.setItem('careeros_locale',${JSON.stringify(locale)});localStorage.removeItem('careeros_profile_v2');localStorage.removeItem('careeros_repo');location.reload()`)
      await until("document.readyState==='complete' && !!document.querySelector('header button')")
      await openProfile()
      const review=locale==="en"?"Review suggested changes":"Revisar alterações sugeridas"
      const next=locale==="en"?"Process next portion":"Processar próxima parte"
      const dismiss=locale==="en"?"Dismiss remaining suggestions for this portion":"Dispensar sugestões restantes desta parte"
      const approve=locale==="en"?"Approve linked changes":"Aprovar alterações ligadas"
      const apply=locale==="en"?"Apply approved changes":"Aplicar alterações aprovadas"
      const retry=locale==="en"?"Retry first unfinished portion":"Tentar novamente a primeira parte não concluída"
      const input="# Role A\nJava\n# Role A\nJava\n# Role B\nRuby"
      await fill(input)
      await evaluate(`window.__portionCalls=[];window.__failPortion=true;window.fetch=async(url,options)=>{
        const req=JSON.parse(options.body);window.__portionCalls.push(req);
        const index=req.portion.index;
        if(index===2&&window.__failPortion) return new Response(JSON.stringify({error:'truncated'}),{status:502});
        const name=index===2?'Ruby':'Java';
        const claim={id:'c1',source:name,text:name,targets:['skills'],question:'',meaning:{assertion:'affirmed',intent:'actual',certainty:'certain',temporal:{wording:'',precision:'unknown'}}};
        const operations=index===1?[]:[{claimId:'c1',target:'skills',entryId:'',field:'skills',action:'add',value:name,finding:'addition'}];
        const fact=req.document.facts.find(f=>f.field==='skills'&&f.value==='Java');
        const payload={decision:{version:1,field:'professional_information',outcome:{kind:'accept'}},claims:[claim],operations,unverifiedClaimCount:0,unresolvedClaimIds:[],unplacedOperationCount:0,skippedClaims:[],coverage:{validClaims:1,invalidClaims:0,discoveryComplete:false,capacity:'within_limit'},outcomes:[{claimId:'c1',kind:index===1?'exact_duplicate':'change',reason:'validated_operation',relatedFacts:index===1?[{profileId:req.document.id,id:fact.id,revision:fact.revision}]:[],relatedClaimIds:[],operationIndexes:index===1?[]:[0]}]};
        const raw=await (await window.__portionResponse(payload)).json();
        const a=req.input.indexOf('# Role A',1),b=req.input.indexOf('# Role B');
        raw.continuation={...raw.continuation,index,total:3,regions:[[{Start:0,End:a}],[{Start:a,End:b}],[{Start:b,End:new TextEncoder().encode(req.input).length}]]};
        return new Response(JSON.stringify(raw),{status:200});
      }`)
      await evaluate(`${byText(review)}.click()`)
      await until("document.querySelectorAll('article').length===1")
      assert.equal(await evaluate("window.__portionCalls.length"),1)
      assert.equal(await evaluate(`${byText(next)}.disabled`),true)
      await evaluate(`${byText(approve)}.click()`)
      await evaluate(`${byText(apply)}.click()`)
      await until("document.querySelectorAll('article').length===0")
      assert.equal(JSON.parse(await saved()).skills,"Java")
      assert.equal(await evaluate("document.querySelector('#ingestion-text').value"),input)
      await evaluate(`${byText(next)}.click()`)
      await until("document.querySelectorAll('article').length===1")
      assert.equal(await evaluate("window.__portionCalls[1].profile.skills"),"Java")
      assert.ok((await evaluate("document.querySelector('article').textContent")).includes(locale==="en"?"Exact duplicate":"Duplicata exata"))
      await evaluate(`${byText(dismiss)}.click()`)
      await evaluate(`${byText(next)}.click()`)
      await until("!!document.querySelector('[role=alert]')")
      await pause(100)
      assert.equal(await evaluate("window.__portionCalls.length"),3)
      assert.equal(JSON.parse(await saved()).skills,"Java")
      assert.equal(await evaluate("document.querySelector('#ingestion-text').value"),input)
      await evaluate("window.__failPortion=false")
      await evaluate(`${byText(retry)}.click()`)
      await until("document.querySelectorAll('article').length===1")
      assert.equal(await evaluate("window.__portionCalls[3].portion.index"),2)
      assert.equal(JSON.parse(await saved()).skills,"Java")
      assert.ok(await evaluate("document.documentElement.scrollWidth<=innerWidth"))
      const screen=await call("Page.captureScreenshot",{format:"png",captureBeyondViewport:true})
      writeFileSync(join(work,`continuation-${locale}-${width}.png`),Buffer.from(screen.data,"base64"))
      // A new tab's incompatible revision invalidates pending work, preserving the paste.
      await evaluate(`(()=>{const old=localStorage.getItem('careeros_profile_v2');const doc=JSON.parse(old);doc.revision++;const value=JSON.stringify(doc);localStorage.setItem('careeros_profile_v2',value);window.dispatchEvent(new StorageEvent('storage',{key:'careeros_profile_v2',oldValue:old,newValue:value,storageArea:localStorage}))})()`)
      await until("document.querySelectorAll('article').length===0")
      assert.equal(await evaluate("document.querySelector('#ingestion-text').value"),input)
      assert.equal(await evaluate(`${byText(review)} !== undefined`),true)
      console.log(`PASS deliberate continuation, saved duplicate, truncation/retry and cross-tab ${locale} ${width}px`)
    }
  }

  // Persistence failures must retain the review and original input in both locales.
  if (!process.env.PROFILE_CONTINUATION_ONLY) for (const locale of ["en", "pt-BR"])
    for (const width of [1440, 390]) {
      await call("Emulation.setDeviceMetricsOverride", {
        width,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false,
      })
      await evaluate(
        `localStorage.setItem('careeros_locale',${JSON.stringify(locale)}); localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo',${JSON.stringify(JSON.stringify(repo))}); location.reload()`,
      )
      await until(
        "document.readyState === 'complete' && !!document.querySelector('header button')",
      )
      await openProfile()
      const original = await evaluate(
        "localStorage.getItem('careeros_profile_v2')",
      )
      const legacySnapshot = await evaluate(
        "localStorage.getItem('careeros_repo')",
      )
      const review =
        locale === "en"
          ? "Review suggested changes"
          : "Revisar alterações sugeridas"
      const approve =
        locale === "en"
          ? "Approve linked changes"
          : "Aprovar alterações ligadas"
      const apply =
        locale === "en"
          ? "Apply approved changes"
          : "Aplicar alterações aprovadas"
      const text =
        approvedSource +
        " English; Go; BSc North 2021; Cloud Guild; English fluent."
      await fill(text)
      await evaluate(
        `window.fetch=async()=>await window.__portionResponse(${JSON.stringify(fixture)},{status:200})`,
      )
      await evaluate(`${byText(review)}.click()`)
      await until(`${byText(apply)} !== undefined`)
      await evaluate(
        `Array.from(document.querySelectorAll('button')).filter(el=>el.textContent.trim()===${JSON.stringify(approve)}).forEach(el=>el.click())`,
      )
      await evaluate(
        `window.__setItem=Storage.prototype.setItem; Storage.prototype.setItem=function(key,value){if(key==='careeros_profile_v2')throw new DOMException('quota','QuotaExceededError');return window.__setItem.call(this,key,value)}`,
      )
      await evaluate(`${byText(apply)}.click()`)
      await until("!!document.querySelector('[role=alert]')")
      assert.match(
        await evaluate("document.querySelector('[role=alert]').textContent"),
        locale === "en" ? /could not be saved/ : /Não foi possível salvar/,
      )
      assert.equal(
        await evaluate("document.querySelector('#ingestion-text').value"),
        text,
      )
      assert.ok(
        await evaluate("document.querySelectorAll('article').length > 0"),
      )
      assert.equal(
        await evaluate("localStorage.getItem('careeros_profile_v2')"),
        original,
      )
      assert.equal(
        await evaluate("localStorage.getItem('careeros_repo')"),
        legacySnapshot,
      )
      assert.ok(
        await evaluate("document.documentElement.scrollWidth <= innerWidth"),
      )
      const screenshot = await call("Page.captureScreenshot", {
        format: "png",
        captureBeyondViewport: true,
      })
      writeFileSync(
        join(work, `save-failure-${locale}-${width}.png`),
        Buffer.from(screenshot.data, "base64"),
      )
      await evaluate(
        "Storage.prototype.setItem=window.__setItem; location.reload()",
      )
      await until(
        "document.readyState === 'complete' && !!document.querySelector('header button')",
      )
      await openProfile()
      assert.deepEqual(JSON.parse(await saved()), repo)
      // A delayed accepted save must not clear a newer paste entered while the
      // previous save waits for the origin's lock.
      await fill(text)
      await evaluate(
        `window.fetch=async()=>await window.__portionResponse(${JSON.stringify(fixture)},{status:200})`,
      )
      await evaluate(`${byText(review)}.click()`)
      await until(`${byText(apply)} !== undefined`)
      await evaluate(
        `Array.from(document.querySelectorAll('button')).filter(el=>el.textContent.trim()===${JSON.stringify(approve)}).forEach(el=>el.click())`,
      )
      await evaluate(
        `window.__heldLock=navigator.locks.request('careeros_profile_v2',()=>new Promise(resolve=>window.__releaseSave=resolve)); void 0`,
      )
      await until("typeof window.__releaseSave === 'function'")
      await evaluate(`${byText(apply)}.click()`)
      await fill("New career notes entered while saving previous review")
      await evaluate("window.__releaseSave()")
      await until(
        `localStorage.getItem('careeros_profile_v2') !== ${JSON.stringify(original)}`,
      )
      await pause(100)
      assert.equal(
        await evaluate("document.querySelector('#ingestion-text').value"),
        "New career notes entered while saving previous review",
      )
      await evaluate(
        `localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo',${JSON.stringify(JSON.stringify(repo))}); location.reload()`,
      )
      await until(
        "document.readyState === 'complete' && !!document.querySelector('header button')",
      )
      await openProfile()
      // A real same-origin sibling window writes a new revision. The storage event
      // blocks the old tab and preserves pending text rather than merging silently.
      await fill("Pending notes before another tab saves")
      await evaluate(
        `(() => { window.__peer=window.open('about:blank','profile-peer'); const doc=JSON.parse(localStorage.getItem('careeros_profile_v2')); doc.revision++; for(const fact of doc.facts)if(fact.owner.id===doc.id)fact.owner.revision=doc.revision; window.__peer.localStorage.setItem('careeros_profile_v2',JSON.stringify(doc)); })()`,
      )
      await until("!!document.querySelector('[role=alert]')")
      assert.match(
        await evaluate("document.querySelector('[role=alert]').textContent"),
        locale === "en" ? /another tab/ : /outra aba/,
      )
      assert.equal(
        await evaluate("document.querySelector('#ingestion-text').value"),
        "Pending notes before another tab saves",
      )
      await evaluate("window.__peer.close()")
      // Inject a quota failure before initialization: migration must retain the
      // original key and show its valid fields without installing v2 authority.
      const injection = await call("Page.addScriptToEvaluateOnNewDocument", {
        source: `window.__originalStorageSet=Storage.prototype.setItem; Storage.prototype.setItem=function(key,value){if(key==='careeros_profile_v2')throw new DOMException('quota','QuotaExceededError');return window.__originalStorageSet.call(this,key,value)}`,
      })
      await evaluate(
        `localStorage.removeItem('careeros_profile_v2'); location.reload()`,
      )
      await until(
        "document.readyState === 'complete' && !!document.querySelector('header button') && !!document.querySelector('[role=alert]')",
      )
      await openProfile()
      assert.equal(
        await evaluate("localStorage.getItem('careeros_profile_v2')"),
        null,
      )
      assert.equal(
        await evaluate("localStorage.getItem('careeros_repo')"),
        legacySnapshot,
      )
      assert.equal(
        await evaluate("document.querySelector('#contact-fullName').value"),
        repo.fullName,
      )
      assert.match(
        await evaluate("document.querySelector('[role=alert]').textContent"),
        locale === "en" ? /could not be saved/ : /Não foi possível salvar/,
      )
      await call("Page.removeScriptToEvaluateOnNewDocument", {
        identifier: injection.identifier,
      })
      await evaluate(`location.reload()`)
      await until(
        "document.readyState === 'complete' && !!document.querySelector('header button')",
      )
      console.log(`PASS save recovery and cross-tab ${locale} ${width}px`)
    }
  console.log("Screenshots:", work)
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}
