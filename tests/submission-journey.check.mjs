import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { createServer } from "node:net"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createBrowserEvaluation } from "./browser-evaluation.mjs"
import { savedProfileExpression } from "./profile-browser-storage.mjs"

// Browser evidence for the composed Profile ingestion journey (#192/#193). Every
// provider response is synthetic; the canonical Profile is browser-local and no
// live request or deployment is used.
const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-submission-journey-"))
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
let traceLabel = "baseline"
const keyboardTrace = []
const port = await new Promise((resolve) => {
  const server = createServer()
  server.listen(0, "127.0.0.1", () => {
    const value = server.address().port
    server.close(() => resolve(value))
  })
})
const copy = {
  en: {
    add: "Add information",
    title: "Add professional information",
    prepare: "Prepare changes",
    stop: "Stop",
    resume: "Resume processing",
    retry: "Retry processing",
    review: "Review changes",
    selectAdditions: "Select clear additions",
    save: "Save to Profile",
    retrySave: "Retry save",
    discard: "Discard",
    keepEditing: "Keep editing",
    answer: "Answer",
    answerSubmit: "Review revised proposal",
    edit: "Edit",
    saveWording: "Save wording",
    source: "Source",
    edited: "Your wording",
    saved: "Saved to your Profile",
    addMore: "Add more information",
    clarification: "Needs clarification",
    duplicate: "Already in your Profile (1)",
    attention: "Attention needed",
  },
  "pt-BR": {
    add: "Adicionar informações",
    title: "Adicionar informações profissionais",
    prepare: "Preparar alterações",
    stop: "Parar",
    resume: "Retomar processamento",
    retry: "Tentar processar novamente",
    review: "Revisar alterações",
    selectAdditions: "Selecionar adições claras",
    save: "Salvar no Perfil",
    retrySave: "Tentar salvar novamente",
    discard: "Descartar",
    keepEditing: "Continuar editando",
    answer: "Responder",
    answerSubmit: "Revisar proposta atualizada",
    edit: "Editar",
    saveWording: "Salvar texto",
    source: "Fonte",
    edited: "Seu texto",
    saved: "Salvo no seu Perfil",
    addMore: "Adicionar mais informações",
    clarification: "Precisa de esclarecimento",
    duplicate: "No seu Perfil (1)",
    attention: "Atenção necessária",
  },
}
const populated = {
  fullName: "Ada Lovelace",
  email: "ada@example.test",
  phone: "",
  location: "",
  professionalLinks: "",
  careerGoals: "Build useful products.",
  skills: "React",
  competencies:
    "Existing competency: " + "b".repeat(180),
  tools: "",
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

function journeyFetch(url, options) {
  if (url !== "/api/profile/ingest") return window.__realFetch(url, options)
  const body = JSON.parse(options.body)
  const calls = (window.__journeyCalls ??= [])
  calls.push({ kind: body.stage?.kind, body })
  const scenario = window.__journeyScenario || "normal"
  const runId = "run-journey"
  const bytes = new TextEncoder().encode(body.input)
  const splitAt = body.input.indexOf("I led")
  const split = splitAt > 0 ? new TextEncoder().encode(body.input.slice(0, splitAt)).length : bytes.length
  const ranges =
    scenario === "partial"
      ? [
          [0, split],
          [split, bytes.length],
        ]
      : [[0, bytes.length]]
  const batches = ranges.map(([Start, End], index) => ({
    index,
    primary: [{ Start, End }],
    context: [],
    preparedBytes: 200,
    payloadBytes: 128,
  }))
  const receipt = (label) => `${label}.${"s".repeat(43)}`
  const expiresAt = Date.now() + 600000
  const ok = (raw) => new Response(JSON.stringify(raw), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })

  if (body.stage?.kind === "plan")
    return ok({
      stage: { version: 2, kind: "plan" },
      planReceipt: receipt("plan"),
      expiresAt,
      plan: {
        protocolVersion: "profile-submission-v1",
        sourceId: "source-journey",
        profileId: "profile-journey",
        runId,
        planVersion: "portions-v1",
        status: "ready",
        complete: true,
        originalBytes: bytes.length,
        preparedBytes: batches.length * 200,
        batches,
        remaining: [],
      },
    })

  if (body.stage?.kind === "extract") {
    if (
      scenario === "partial" &&
      body.batchIndex === 1 &&
      window.__failBatchOnce
    ) {
      window.__failBatchOnce = false
      return new Response(JSON.stringify({ error: "outage" }), { status: 502 })
    }
    const claim = {
      id: "c0",
      source: body.input,
      text:
        scenario === "clarification"
          ? "TypeScript version is unclear"
          : body.input,
      targets: scenario === "clarification" ? [] : ["skills"],
      question:
        scenario === "clarification"
          ? "Which TypeScript version do you use?"
          : "",
    }
    const richClaims = [
      {
        id: "c0",
        source: body.input,
        text: "I use Kubernetes.",
        targets: ["skills"],
        question: "",
      },
      {
        id: "c1",
        source: body.input,
        text: "I also use Go.",
        targets: ["careerGoals"],
        question: "",
      },
      {
        id: "c2",
        source: body.input,
        text: "I build with Terraform.",
        targets: ["tools"],
        question: "",
      },
      {
        id: "c3",
        source: body.input,
        text: "I lead cross-functional delivery.",
        targets: ["competencies"],
        question: "",
      },
      {
        id: "c4",
        source: body.input,
        text: "Build useful products.",
        targets: ["careerGoals"],
        question: "",
      },
      {
        id: "c5",
        source: body.input,
        text: "Python",
        targets: ["skills"],
        question: "",
      },
      {
        id: "c6",
        source: body.input,
        text: "French level is unclear.",
        targets: [],
        question: "What is your French level?",
      },
    ]
    const claims = scenario === "rich" ? richClaims : [claim]
    const skipped =
      scenario === "partial" && body.batchIndex === 0
        ? [
            {
              index: 1,
              reason: "source",
              text: "Unvalidated extracted claim",
              source: "",
              shortened: false,
            },
          ]
        : []
    const response = () =>
      ok({
        stage: { version: 2, kind: "extract" },
        batchReceipt: receipt(`batch${body.batchIndex}`),
        expiresAt,
        batchIndex: body.batchIndex,
        claims,
        unverifiedCount: skipped.length,
        skippedClaims: skipped,
        coverage: {
          validClaims: claims.length,
          invalidClaims: skipped.length,
          discoveryComplete: false,
          capacity: "within_limit",
        },
      })
    if (window.__holdExtract) {
      window.__holdExtract = false
      window.__extractHeld = true
      return new Promise((resolve, reject) => {
        window.__releaseExtract = () => resolve(response())
        options.signal?.addEventListener(
          "abort",
          () => {
            window.__extractAborted = true
            reject(new DOMException("Aborted", "AbortError"))
          },
          { once: true },
        )
      })
    }
    return response()
  }

  if (body.stage?.kind === "assemble") {
    const richClaimIds = Array.from(
      { length: 7 },
      (_, index) => `${runId}/0/c${index}`,
    )
    const claimIds =
      scenario === "rich"
        ? richClaimIds
        : batches.map((batch) => `${runId}/${batch.index}/c0`)
    return ok({
      stage: { version: 2, kind: "assemble" },
      contextReceipt: receipt("context"),
      expiresAt,
      context: {
        protocolVersion: "profile-submission-v1",
        sourceId: "source-journey",
        profileId: "profile-journey",
        runId,
        planVersion: "portions-v1",
        batches: batches.map((batch, index) => ({
          index: batch.index,
          primary: batch.primary,
          claimIds:
            scenario === "rich" ? richClaimIds : [claimIds[index]],
          claimCount: scenario === "rich" ? richClaimIds.length : 1,
          skippedCount: scenario === "partial" && index === 0 ? 1 : 0,
        })),
        coverage: {
          validClaims: scenario === "rich" ? richClaimIds.length : batches.length,
          invalidClaims: scenario === "partial" ? 1 : 0,
          discoveryComplete: false,
          capacity: "within_limit",
        },
      },
    })
  }

  if (body.stage?.kind === "reconcile") {
    const focused = body.focus.claimIds
    if (scenario === "rich") {
      const document = JSON.parse(
        localStorage.getItem("careeros_profile_v2") || "null",
      )
      const factReference = (field, value) => {
        const fact = document?.facts.find(
          (candidate) =>
            candidate.field === field && String(candidate.value) === value,
        )
        return fact
          ? [
              {
                profileId: document.id,
                id: fact.id,
                revision: fact.revision,
              },
            ]
          : []
      }
      const related = factReference("skills", "React")
      const duplicateRelated = factReference(
        "careerGoals",
        "Build useful products.",
      )
      const longValue = `Cross-functional delivery across product, design and engineering, with measurable outcomes for customers in multiple regions. https://example.test/${"a".repeat(140)}`
      const operation = (
        claimId,
        target,
        value,
        action = "add",
        finding = "addition",
      ) => ({
        claimId,
        target,
        entryId: "",
        field: target,
        action,
        value,
        finding,
      })
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: [
          operation(focused[0], "skills", "Kubernetes"),
          operation(
            focused[1],
            "careerGoals",
            "Build useful products and mentor.",
            "update",
            "in_place",
          ),
          operation(focused[2], "tools", "Terraform"),
          operation(
            focused[3],
            "competencies",
            longValue,
            "update",
            "in_place",
          ),
        ],
        outcomes: [
          ...focused.slice(0, 4).map((claimId) => ({
            claimId,
            kind: "change",
            reason: "validated_operation",
            relatedFacts: [],
            relatedClaimIds: [],
          })),
          {
            claimId: focused[4],
            kind: "exact_duplicate",
            reason: "verified_exact_alias_or_wording",
            relatedFacts: duplicateRelated,
            relatedClaimIds: [],
          },
          {
            claimId: focused[5],
            kind: "contradiction",
            reason: "source_ambiguity",
            relatedFacts: related,
            relatedClaimIds: [focused[4]],
          },
          {
            claimId: focused[6],
            kind: "clarification",
            reason: "source_ambiguity",
            relatedFacts: [],
            relatedClaimIds: [],
          },
        ],
      })
    }
    if (scenario === "duplicate")
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: [],
        outcomes: focused.map((claimId) => ({
          claimId,
          kind: "exact_duplicate",
          reason: "verified_exact_alias_or_wording",
          relatedFacts: [],
          relatedClaimIds: [],
        })),
      })
    if (scenario === "clarification")
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: [],
        outcomes: focused.map((claimId) => ({
          claimId,
          kind: "clarification",
          reason: "source_ambiguity",
          relatedFacts: [],
          relatedClaimIds: [],
        })),
      })
    if (scenario === "invalid")
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: focused.flatMap((claimId) => [
          {
            claimId,
            target: "experience",
            entryId: "new:experience-1",
            field: "title",
            action: "add",
            value: "Engineer",
            finding: "addition",
          },
          {
            claimId,
            target: "experience",
            entryId: "new:experience-1",
            field: "startDate",
            action: "add",
            value: "2020",
            finding: "addition",
          },
        ]),
        outcomes: focused.map((claimId) => ({
          claimId,
          kind: "change",
          reason: "validated_operation",
          relatedFacts: [],
          relatedClaimIds: [],
        })),
      })
    return ok({
      stage: { version: 2, kind: "reconcile" },
      focus: { claimIds: focused },
      operations: focused.map((claimId, index) => ({
        claimId,
        target: "skills",
        entryId: "",
        field: "skills",
        action: "add",
        value: index === 0 ? "TypeScript" : "Payments leadership",
        finding: "addition",
      })),
      outcomes: focused.map((claimId) => ({
        claimId,
        kind: "change",
        reason: "validated_operation",
        relatedFacts: [],
        relatedClaimIds: [],
      })),
    })
  }

  if (body.stage?.kind === "clarify") {
    const answer = body.answer.trim()
    const target = scenario === "rich" ? "additionalInfo" : "skills"
    const response = () =>
      ok({
        stage: { version: 2, kind: "clarify" },
        claim: {
          ...body.claim,
          targets: [target],
          question: "",
          supportingSources: [
            { source: answer, origin: "clarification_answer" },
          ],
        },
        focus: { claimIds: [body.claim.id] },
        operations: [
          {
            claimId: body.claim.id,
            target,
            entryId: "",
            field: target,
            action: "add",
            value: answer,
            finding: "addition",
          },
        ],
        outcomes: [
          {
            claimId: body.claim.id,
            kind: "change",
            reason: "validated_operation",
            relatedFacts: [],
            relatedClaimIds: [],
          },
        ],
      })
    if (window.__holdClarify) {
      window.__holdClarify = false
      window.__clarifyHeld = true
      return new Promise((resolve, reject) => {
        window.__releaseClarify = () => resolve(response())
        options.signal?.addEventListener(
          "abort",
          () => reject(new DOMException("Aborted", "AbortError")),
          { once: true },
        )
      })
    }
    return response()
  }

  throw new Error(`Unexpected stage ${body.stage?.kind}`)
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
  const browserEvaluation = createBrowserEvaluation(pause)
  socket.onmessage = ({ data }) => {
    const message = JSON.parse(data)
    browserEvaluation.observe(message)
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
  const evaluate = (expression) =>
    browserEvaluation.evaluate(call, expression, { userGesture: true })
  const until = async (expression) => {
    for (let attempt = 0; attempt < 120; attempt++) {
      if (await evaluate(expression)) return
      await pause(100)
    }
    throw new Error(
      `Timed out: ${expression}\n${await evaluate(
        "JSON.stringify({text:document.body.innerText.slice(0,2500),alerts:Array.from(document.querySelectorAll('[role=alert]')).map(e=>e.textContent),saveError:document.querySelector('[data-save-error]')?.dataset.saveError,calls:window.__journeyCalls?.map(c=>c.kind),state:window.__journeyDebug})",
      )}`,
    )
  }
  await call("Page.enable")
  await call("Emulation.setDeviceMetricsOverride", {
    width: 1280,
    height: 900,
    deviceScaleFactor: 1,
    mobile: false,
  })
  await call("Page.navigate", { url: `http://127.0.0.1:${port}/` })
  await until(
    "document.readyState === 'complete' && !!document.querySelector('header button')",
  )
  const openProfile = async () => {
    await pause(150)
    if (
      !(await evaluate(
        "document.querySelectorAll('header nav button').length === 4",
      ))
    ) {
      const opened = await evaluate(
        `(() => { const el = Array.from(document.querySelectorAll('button')).find(el=>/open app/i.test(el.textContent) || /abrir app/i.test(el.textContent)); if (!el) return false; el.click(); return true; })()`,
      )
      assert.ok(opened, "Landing entry point is missing")
    }
    await until("document.querySelectorAll('header nav button').length === 4")
    await evaluate("document.querySelectorAll('header nav button')[1].click()")
  }
  const byText = (text) =>
    `Array.from(document.querySelectorAll('button')).find(el=>el.textContent.trim()===${JSON.stringify(text)})`
  const clickText = async (text) => {
    const found = await evaluate(`!!${byText(text)}`)
    assert.ok(found, `Missing button ${text}`)
    await evaluate(`${byText(text)}.click()`)
    await pause(80)
  }
  const fill = async (selector, value) => {
    await evaluate(
      `(() => { const el = document.querySelector(${JSON.stringify(selector)}); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, ${JSON.stringify(value)}); el.dispatchEvent(new Event('input', { bubbles: true })); })()`,
    )
    await pause(60)
  }
  const installMock = async (scenario) => {
    await evaluate(
      `(() => {
        if (!window.__realFetch) window.__realFetch = window.fetch.bind(window);
        window.__journeyScenario = ${JSON.stringify(scenario)};
        window.__journeyCalls = [];
        window.__holdExtract = false;
        window.__extractHeld = false;
        window.__extractAborted = false;
        window.__holdClarify = false;
        window.__clarifyHeld = false;
        window.__failBatchOnce = ${scenario === "partial"};
        window.__failSaveOnce = false;
        window.fetch = ${journeyFetch.toString()};
      })()`,
    )
  }
  const resetStorage = async (profile, locale = "en") => {
    await evaluate(
      `(() => {
        localStorage.removeItem('careeros_profile_v2');
        ${profile ? `localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(profile))});` : "localStorage.removeItem('careeros_repo');"}
        localStorage.setItem('careeros_apikey', 'synthetic-test-key');
        localStorage.setItem('careeros_typesafe_key', 'synthetic-typesafe');
        localStorage.setItem('careeros_locale', ${JSON.stringify(locale)});
        location.reload();
      })()`,
    )
    await until(
      "document.readyState === 'complete' && !!document.querySelector('header button')",
    )
  }
  const watchWrites = async () => {
    await evaluate(
      `(() => {
        window.__profileWrites = 0;
        if (!window.__setPatched) {
          window.__realSet = Storage.prototype.setItem;
          Storage.prototype.setItem = function(key, value) {
            if (key === 'careeros_profile_v2') {
              if (window.__failSaveOnce) {
                window.__failSaveOnce = false;
                throw new DOMException('quota', 'QuotaExceededError');
              }
              window.__profileWrites++;
            }
            return window.__realSet.call(this, key, value);
          };
          window.__setPatched = true;
        }
      })()`,
    )
  }

  const setViewport = async (width) => {
    await call("Emulation.setDeviceMetricsOverride", {
      width,
      height: width === 390 ? 844 : 1000,
      deviceScaleFactor: 1,
      mobile: false,
    })
    await pause(80)
  }
  const setLocale = async (locale) => {
    await evaluate(
      `(() => {
        const el = document.querySelector('header select');
        if (!el) throw new Error('Language selector is missing');
        Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, ${JSON.stringify(locale)});
        el.dispatchEvent(new Event('change', { bubbles: true }));
      })()`,
    )
    await until(
      `document.documentElement.lang === ${JSON.stringify(locale)}`,
    )
    await pause(100)
  }
  const pressKey = async (key, code, windowsVirtualKeyCode, text) => {
    await call("Input.dispatchKeyEvent", {
      type: "keyDown",
      key,
      code,
      windowsVirtualKeyCode,
      ...(text ? { text } : {}),
    })
    await call("Input.dispatchKeyEvent", {
      type: "keyUp",
      key,
      code,
      windowsVirtualKeyCode,
    })
    await pause(40)
    const focus = await activeInfo()
    keyboardTrace.push({
      label: traceLabel,
      key,
      tag: focus.tag,
      type: focus.type,
      id: focus.id,
      aria: focus.aria,
      role: focus.role,
      text: focus.text,
      focusVisible: focus.focusVisible,
    })
  }
  const pressEnter = () => pressKey("Enter", "Enter", 13, "\r")
  const pressSpace = () => pressKey(" ", "Space", 32, " ")
  const activeInfo = () =>
    evaluate(
      `(() => {
        const el = document.activeElement;
        return {
          tag: el?.tagName || '',
          type: el?.getAttribute?.('type') || '',
          id: el?.id || '',
          text: (el?.textContent || '').trim().replace(/\\s+/g, ' ').slice(0, 140),
          aria: el?.getAttribute?.('aria-label') || '',
          role: el?.getAttribute?.('role') || '',
          dataAnswer: !!el?.hasAttribute?.('data-review-answer-open'),
          reviewKind: el?.closest?.('[data-review-unit]')?.dataset?.reviewKind || '',
          focusVisible: !!el?.matches?.(':focus-visible'),
        };
      })()`,
    )
  const openJourneyLocalized = async (labels) => {
    if (!(await evaluate("!!document.querySelector('#submission-text')"))) {
      await clickText(labels.add)
      await until("!!document.querySelector('#submission-text')")
    }
  }
  const assertNoOverflow = async (label) => {
    const metrics = await evaluate(
      `(() => {
        const doc = document.documentElement;
        const journey = document.querySelector('[data-submission-journey]');
        const review = document.querySelector('#review-heading')?.closest('section');
        const actions = document.querySelector('[data-submission-actions]');
        const actionRects = actions
          ? Array.from(actions.querySelectorAll('button,p')).map((el) => {
              const rect = el.getBoundingClientRect();
              return { left: rect.left, right: rect.right, text: el.textContent.trim().slice(0, 40) };
            })
          : [];
        const smallTargets = Array.from(
          journey?.querySelectorAll('button, summary, select, input[type=checkbox]') || [],
        )
          .map((el) => (el.tagName === 'INPUT' ? el.closest('label') || el : el))
          .filter((el) => {
            const rect = el.getBoundingClientRect();
            const style = getComputedStyle(el);
            return style.display !== 'none' && style.visibility !== 'hidden' && rect.width > 0 && rect.height > 0;
          })
          .filter((el) => {
            const rect = el.getBoundingClientRect();
            return rect.width < 24 || rect.height < 24;
          })
          .map((el) => ({
            tag: el.tagName,
            text: (el.textContent || '').trim().slice(0, 40),
            width: Math.round(el.getBoundingClientRect().width),
            height: Math.round(el.getBoundingClientRect().height),
          }));
        return {
          doc: [doc.scrollWidth, doc.clientWidth],
          journey: journey ? [journey.scrollWidth, journey.clientWidth] : null,
          review: review ? [review.scrollWidth, review.clientWidth] : null,
          viewport: window.innerWidth,
          actionPosition: actions ? getComputedStyle(actions).position : '',
          actionRects,
          smallTargets,
        };
      })()`,
    )
    assert.ok(
      metrics.doc[0] <= metrics.doc[1] + 1,
      `${label}: document overflows horizontally (${metrics.doc.join("/")})`,
    )
    if (metrics.journey)
      assert.ok(
        metrics.journey[0] <= metrics.journey[1] + 1,
        `${label}: journey overflows horizontally (${metrics.journey.join("/")})`,
      )
    if (metrics.review)
      assert.ok(
        metrics.review[0] <= metrics.review[1] + 1,
        `${label}: review overflows horizontally (${metrics.review.join("/")})`,
      )
    for (const rect of metrics.actionRects) {
      assert.ok(
        rect.left >= -1 && rect.right <= metrics.viewport + 1,
        `${label}: action ${rect.text} is outside the page`,
      )
    }
    assert.notEqual(
      metrics.actionPosition,
      "fixed",
      `${label}: action footer must not cover review content`,
    )
    assert.notEqual(
      metrics.actionPosition,
      "sticky",
      `${label}: action footer must remain in the review flow`,
    )
    assert.deepEqual(
      metrics.smallTargets,
      [],
      `${label}: interactive target below 24px`,
    )
    return metrics
  }
  const assertSingleColumn = async (label) => {
    const rects = await evaluate(
      `Array.from(document.querySelectorAll('[data-review-destination]')).map((el) => {
        const rect = el.getBoundingClientRect();
        return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom };
      })`,
    )
    assert.ok(rects.length > 1, `${label}: expected multiple destination groups`)
    for (let index = 0; index < rects.length; index++) {
      assert.ok(
        Math.abs(rects[index].left - rects[0].left) < 1,
        `${label}: destinations must share one mobile column`,
      )
      assert.ok(
        rects[index].right <= rects[0].right + 1,
        `${label}: destination exceeds the mobile column`,
      )
      if (index > 0)
        assert.ok(
          rects[index].top >= rects[index - 1].bottom - 1,
          `${label}: destinations must stack vertically`,
        )
    }
  }
  const capture = async (name) => {
    await evaluate(
      `(() => {
        document.activeElement?.blur?.();
        document.documentElement.style.scrollBehavior = 'auto';
        // Chromium's full-page capture stitches a sticky header at its current
        // viewport offset. Keep the header in flow only for the capture so the
        // retained evidence has no duplicated or clipped page chrome.
        document.querySelector('.app-header')?.style.setProperty('position', 'static', 'important');
        window.scrollTo(0, 0);
        return new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      })()`,
    )
    await pause(200)
    const shot = await call("Page.captureScreenshot", {
      format: "png",
      captureBeyondViewport: true,
    })
    writeFileSync(join(work, `${name}.png`), Buffer.from(shot.data, "base64"))
    await evaluate(
      `(() => {
        document.querySelector('.app-header')?.style.removeProperty('position');
        document.documentElement.style.scrollBehavior = '';
      })()`,
    )
  }

  // Empty Profile: pasting is the primary entry, Stop keeps the draft, Resume
  // completes the same submission, a failed save retains the review, and one
  // explicit retry saves exactly the selected operation.
  await resetStorage(null)
  await installMock("normal")
  await openProfile()
  await until("!!document.querySelector('#submission-text')")
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Add professional information/,
  )
  assert.equal(
    await evaluate(
      "!!(document.querySelector('[data-submission-journey]').compareDocumentPosition(document.querySelector('#contact-fullName')) & Node.DOCUMENT_POSITION_FOLLOWING)",
    ),
    true,
    "Empty Profile must expose the ingestion journey before contact editing",
  )
  assert.equal(
    await evaluate("document.querySelector('[data-submission-journey]').innerText.includes('Process next portion')"),
    false,
  )
  await fill("#submission-text", "I use TypeScript.")
  await watchWrites()
  await evaluate("window.__holdExtract = true")
  await clickText("Prepare changes")
  await until("window.__extractHeld === true")
  assert.equal(await evaluate("window.__profileWrites"), 0)
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey] [role=status]').innerText"),
    /Extracting information/,
  )
  await clickText("Stop")
  await until("window.__extractAborted === true")
  await until(`!!${byText("Resume processing")}`)
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /I use TypeScript\./,
  )
  await clickText("Resume processing")
  await until("!!document.querySelector('#review-heading')")
  assert.equal(
    await evaluate("!!document.querySelector('#submission-text')"),
    false,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Your pasted information/,
  )
  assert.equal(
    await evaluate(`${byText("Save to Profile")}.disabled`),
    true,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-actions]').innerText"),
    /No changes selected/,
  )
  await evaluate(
    "document.querySelector('[data-review-unit] input[type=checkbox]').click()",
  )
  await until(`${byText("Save to Profile")}.disabled === false`)
  assert.match(
    await evaluate("document.querySelector('[data-submission-actions]').innerText"),
    /1 of 1 changes selected/,
  )
  const reviewShot = await call("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: true,
  })
  writeFileSync(
    join(work, "journey-review-en-1280.png"),
    Buffer.from(reviewShot.data, "base64"),
  )
  assert.equal(await evaluate("window.__profileWrites"), 0)
  assert.equal(
    await evaluate("window.__journeyCalls.filter(c=>c.kind==='clarify').length"),
    0,
  )
  await evaluate("window.__failSaveOnce = true")
  await clickText("Save to Profile")
  await until(`!!${byText("Retry save")}`)
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey] [role=alert]').innerText"),
    /Not saved/,
  )
  const providerCalls = await evaluate("window.__journeyCalls.length")
  assert.equal(await evaluate("window.__profileWrites"), 0)
  await clickText("Retry save")
  await until(`!!${byText("Add more information")}`)
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey] [role=status]').innerText"),
    /Saved to your Profile/,
  )
  assert.equal(
    await evaluate("window.__journeyCalls.length"),
    providerCalls,
    "Retry save must make zero provider calls",
  )
  assert.equal(await evaluate("window.__profileWrites"), 1)
  const savedProfile = JSON.parse(await evaluate(savedProfileExpression))
  assert.match(savedProfile.skills, /TypeScript/)
  // Leaving and returning after a durable save drops the success panel and
  // returns to the populated Profile entry instead of showing duplicate state.
  await evaluate("document.querySelectorAll('header nav button')[0].click()")
  await pause(100)
  await openProfile()
  await until("!!document.querySelector('#contact-fullName')")
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Add information/,
  )
  assert.equal(
    await evaluate("document.querySelector('[data-submission-journey]').innerText.includes('Saved to your Profile')"),
    false,
  )
  await clickText("Add information")
  await until("!!document.querySelector('#submission-text')")
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "",
  )

  console.log("PASS empty Profile journey: stop/resume, explicit save, save retry, one write")

  const clickJourneyDiscard = async () => {
    const clicked = await evaluate(
      `(() => { const el = Array.from(document.querySelectorAll('[data-submission-journey] button')).find(el => el.textContent.trim() === 'Discard' && !el.closest('dialog')); if (!el) return false; el.click(); return true; })()`,
    )
    assert.ok(clicked, "Journey Discard action is missing")
    await until("!!document.querySelector('[data-submission-journey] dialog[open]')")
  }
  const confirmDiscard = async () => {
    await evaluate(
      "document.querySelector('[data-submission-journey] dialog[open] button:last-of-type').click()",
    )
    await until(
      "!document.querySelector('[data-submission-journey] dialog[open]')",
    )
  }
  const openJourney = async () => {
    if (!(await evaluate("!!document.querySelector('#submission-text')"))) {
      await clickText("Add information")
      await until("!!document.querySelector('#submission-text')")
    }
  }
  const runToReview = async (scenario, input) => {
    await installMock(scenario)
    await openJourney()
    await fill("#submission-text", input)
    await clickText("Prepare changes")
    await until("!!document.querySelector('#review-heading')")
  }

  // Populated Profile: manual editing stays separate, Add information opens the
  // same journey, and the existing discard confirmation preserves work on cancel.
  await resetStorage(populated)
  await installMock("normal")
  await openProfile()
  await until("!!document.querySelector('#contact-fullName')")
  assert.equal(
    await evaluate("document.querySelector('#contact-fullName').value"),
    "Ada Lovelace",
  )
  assert.equal(
    await evaluate("!!document.querySelector('#submission-text')"),
    false,
  )
  assert.equal(
    await evaluate(
      "document.querySelector('[data-submission-journey]').innerText.includes('Process next portion')",
    ),
    false,
  )
  await openJourney()
  await fill("#submission-text", "I use TypeScript.")
  await clickJourneyDiscard()
  await evaluate(
    "document.querySelector('[data-submission-journey] dialog[open] button:first-of-type').click()",
  )
  await until(
    "!document.querySelector('[data-submission-journey] dialog[open]')",
  )
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "I use TypeScript.",
  )
  await clickJourneyDiscard()
  await confirmDiscard()
  await until(
    "!document.querySelector('#submission-text') && !!Array.from(document.querySelectorAll('[data-submission-journey] button')).find(el=>el.textContent.trim()==='Add information')",
  )
  console.log("PASS populated Profile entry and discard confirmation")

  // No-change: duplicate-only output ends without approval work or any write.
  await resetStorage(null)
  await installMock("duplicate")
  await openProfile()
  await watchWrites()
  await openJourney()
  await fill("#submission-text", "I use TypeScript.")
  await clickText("Prepare changes")
  await until("!!document.querySelector('#review-heading')")
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Everything you pasted is already in your Profile/,
  )
  assert.equal(
    await evaluate("!!document.querySelector('[data-review-unit] input[type=checkbox]')"),
    false,
  )
  assert.equal(await evaluate(`${byText("Save to Profile")}.disabled`), true)
  assert.equal(await evaluate("window.__profileWrites"), 0)
  await clickJourneyDiscard()
  await confirmDiscard()
  await until("!!document.querySelector('#submission-text')")
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "",
  )
  console.log("PASS all-duplicate no-change journey")

  // A selection that cannot form a complete transaction keeps Save disabled.
  await resetStorage(null)
  await installMock("invalid")
  await openProfile()
  await watchWrites()
  await openJourney()
  await fill("#submission-text", "I worked as an engineer.")
  await clickText("Prepare changes")
  await until("!!document.querySelector('#review-heading')")
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Cannot be saved yet/,
  )
  assert.equal(
    await evaluate("document.querySelectorAll('[data-review-unit] input[type=checkbox]').length"),
    0,
  )
  assert.equal(
    await evaluate(`${byText("Save to Profile")}.disabled`),
    true,
    "Save must stay disabled for a blocked selection",
  )
  assert.equal(await evaluate("window.__profileWrites"), 0)
  await clickJourneyDiscard()
  await confirmDiscard()
  await until("!!document.querySelector('#submission-text')")
  console.log("PASS invalid selection keeps Save disabled")

  // Partial extraction failure: Retry processing reuses the plan and completed
  // batch, marks unprocessed source explicitly, and saves only selected work.
  await resetStorage(null)
  await installMock("partial")
  await openProfile()
  await watchWrites()
  await openJourney()
  await fill(
    "#submission-text",
    "I use TypeScript. I led a payments project.",
  )
  await clickText("Prepare changes")
  await until(`!!${byText("Retry processing")}`)
  assert.equal(
    await evaluate(
      "window.__journeyCalls.filter(c=>c.kind==='plan').length",
    ),
    1,
  )
  assert.equal(
    await evaluate(
      "window.__journeyCalls.filter(c=>c.kind==='extract').length",
    ),
    2,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /I use TypeScript\. I led a payments project\./,
  )
  await clickText("Retry processing")
  await until("!!document.querySelector('#review-heading')")
  assert.equal(
    await evaluate(
      "window.__journeyCalls.filter(c=>c.kind==='plan').length",
    ),
    1,
  )
  assert.equal(
    await evaluate(
      "window.__journeyCalls.filter(c=>c.kind==='extract').length",
    ),
    3,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Some pasted regions could not be fully processed/,
  )
  await clickText("Select clear additions")
  await until(`${byText("Save to Profile")}.disabled === false`)
  await clickText("Save to Profile")
  await until(`!!${byText("Add more information")}`)
  assert.equal(await evaluate("window.__profileWrites"), 1)
  const partialSaved = JSON.parse(await evaluate(savedProfileExpression))
  assert.match(partialSaved.skills, /TypeScript/)
  assert.match(partialSaved.skills, /Payments leadership/)
  await clickText("Add more information")
  await until("!!document.querySelector('#submission-text')")
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "",
  )
  console.log("PASS partial failure/resume, explicit unprocessed notice, one save")

  // Focused clarification stays inside the journey, revises only its claim and
  // keeps the final save explicit.
  await resetStorage(null)
  await installMock("clarification")
  await openProfile()
  await watchWrites()
  await openJourney()
  await fill("#submission-text", "I use TypeScript.")
  await clickText("Prepare changes")
  await until("!!document.querySelector('#review-heading')")
  await evaluate(
    "document.querySelector('[data-review-kind=clarification] details summary').click()",
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Which TypeScript version do you use\?/,
  )
  await evaluate("document.querySelector('[data-review-answer-open]').click()")
  await until("!!document.querySelector('[data-review-answer] textarea')")
  await fill(
    "[data-review-answer] textarea",
    "TypeScript 5 for frontend work.",
  )
  await evaluate("window.__holdClarify = true")
  await evaluate(
    "document.querySelector('[data-review-answer] button[type=submit]').click()",
  )
  await until("window.__clarifyHeld === true")
  assert.equal(
    await evaluate(`${byText("Save to Profile")}.disabled`),
    true,
    "Save must be disabled while the focused clarification is in flight",
  )
  await evaluate("window.__releaseClarify()")
  await until(
    "window.__journeyCalls.filter(c=>c.kind==='clarify').length === 1 && !document.querySelector('[data-review-kind=clarification]')",
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /TypeScript 5 for frontend work/,
  )
  assert.equal(await evaluate("window.__profileWrites"), 0)
  await clickText("Select clear additions")
  await until(`${byText("Save to Profile")}.disabled === false`)
  await clickText("Save to Profile")
  await until(`!!${byText("Add more information")}`)
  assert.equal(await evaluate("window.__profileWrites"), 1)
  const clarifiedSaved = JSON.parse(await evaluate(savedProfileExpression))
  assert.match(clarifiedSaved.skills, /TypeScript 5 for frontend work/)
  console.log("PASS focused clarification without leaving the journey")

  // A same-origin peer writes a newer Profile revision. The review stays visible
  // but unsavable until Reload; then the journey requires Edit/Discard.
  await resetStorage(null)
  await installMock("normal")
  await openProfile()
  await openJourney()
  await fill("#submission-text", "I use TypeScript.")
  await clickText("Prepare changes")
  await until("!!document.querySelector('#review-heading')")
  assert.equal(await evaluate("window.__profileWrites ?? 0"), 0)
  await evaluate(
    `(() => {
      window.__peer = window.open('about:blank', 'profile-peer');
      const doc = JSON.parse(localStorage.getItem('careeros_profile_v2'));
      doc.revision++;
      for (const fact of doc.facts)
        if (fact.owner.id === doc.id) fact.owner.revision = doc.revision;
      window.__peer.localStorage.setItem('careeros_profile_v2', JSON.stringify(doc));
    })()`,
  )
  await until(
    "!!document.querySelector('[data-profile-content] [role=alert]')",
  )
  assert.match(
    await evaluate("document.body.innerText"),
    /another tab/i,
  )
  assert.match(
    await evaluate("document.querySelector('[data-profile-content] [role=alert]').innerText"),
    /Profile changed since these suggestions/i,
  )
  assert.equal(
    await evaluate("!!document.querySelector('#review-heading')"),
    true,
  )
  assert.equal(await evaluate(`${byText("Save to Profile")}.disabled`), true)
  await evaluate("document.querySelector('[data-profile-refresh]').click()")
  await until(
    "document.querySelector('[data-submission-journey]').innerText.includes('can no longer continue')",
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /I use TypeScript\./,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Start a new submission/,
  )
  await clickJourneyDiscard()
  await confirmDiscard()
  await until("!!document.querySelector('#submission-text')")
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "",
  )
  await evaluate("window.__peer.close()")
  console.log("PASS stale/cross-tab review, reload, explicit discard")

  // Integrated presentation matrix (#193): the same composed journey in both
  // locales at desktop and narrow widths. This adds long values/sources, many
  // destination groups, duplicates, a conflict, a clarification, a save
  // failure and a processing failure to the controlled scenarios above.
  const richInput =
    "I use Kubernetes. Fonte original: React. I also use Go. I build with Terraform. I lead cross-functional delivery for customers in many regions. I know French at an unclear level."
  const rawEnumCodes = [
    "validated_operation",
    "verified_exact_alias_or_wording",
    "source_ambiguity",
    "exact_duplicate",
    "in_place",
    "addition",
  ]
  const selectedCount = () =>
    evaluate(
      "document.querySelectorAll('[data-review-unit] input[type=checkbox]:checked').length",
    )
  const assertNoRawEnums = async (label) => {
    const text = await evaluate(
      "document.querySelector('[data-submission-journey]').textContent",
    )
    for (const code of rawEnumCodes) {
      const match = new RegExp(`\\b${code}\\b`).exec(text)
      if (match)
        console.log(
          "DEBUG ENUM",
          code,
          text.slice(
            Math.max(0, match.index - 120),
            match.index + code.length + 400,
          ),
        )
      assert.equal(match, null, `${label}: exposed internal enum ${code}`)
    }
  }
  const journeyDialog = "document.querySelector('[data-submission-journey] dialog')"
  const focusExpression = async (expression) => {
    const found = await evaluate(`!!(${expression})`)
    assert.ok(found, `Missing keyboard target: ${expression}`)
    await evaluate(`(${expression}).focus()`)
  }
  for (const locale of ["en", "pt-BR"]) {
    for (const width of [1440, 390]) {
      const labels = copy[locale]
      const tag = `${locale}-${width}`
      traceLabel = `${tag} empty`
      await setViewport(width)

      // Empty Profile at this locale and width. The baseline scenario owns the
      // save-failure retry; this matrix cell still runs one explicit save so the
      // empty flow is exercised in every locale/width combination.
      await resetStorage(null, locale)
      await installMock("normal")
      await watchWrites()
      await openProfile()
      await openJourneyLocalized(labels)
      await until(
        `document.querySelector('[data-submission-journey]').innerText.includes(${JSON.stringify(labels.title)})`,
      )
      await assertNoOverflow(`empty ${tag}`)
      await capture(`empty-${tag}`)
      traceLabel = `${tag} empty`
      await fill("#submission-text", "I use TypeScript.")
      await focusExpression(byText(labels.prepare))
      await pressEnter()
      await until("!!document.querySelector('#review-heading')")
      await until("document.activeElement?.id === 'review-heading'")
      await assertNoOverflow(`empty review ${tag}`)
      await focusExpression(
        "document.querySelector('[data-review-unit] input[type=checkbox]')",
      )
      await pressSpace()
      await until(`${byText(labels.save)}.disabled === false`)
      await focusExpression(byText(labels.save))
      await pressEnter()
      await until("!!document.querySelector('[data-submission-saved]')")
      assert.equal(
        await evaluate("window.__profileWrites"),
        1,
        `${tag}: empty Profile save did not write exactly once`,
      )

      // Populated Profile with many groups, long values/sources, duplicate,
      // conflict and clarification.
      await resetStorage(populated, locale)
      await installMock("rich")
      await watchWrites()
      await openProfile()
      await until("document.documentElement.lang === " + JSON.stringify(locale))
      assert.equal(
        await evaluate("!!document.querySelector('#submission-text')"),
        false,
      )
      await openJourneyLocalized(labels)
      traceLabel = `${tag} review`
      await fill("#submission-text", richInput)
      await focusExpression(byText(labels.prepare))
      await pressEnter()
      await until("!!document.querySelector('#review-heading')")
      await until("document.activeElement?.id === 'review-heading'")
      await until(
        "document.querySelectorAll('[data-review-destination]').length >= 4",
      )
      assert.ok(
        (await evaluate(
          "document.querySelectorAll('[data-review-unit]').length",
        )) >= 6,
        `${tag}: expected many grouped changes`,
      )
      assert.equal(
        await evaluate(
          `document.querySelector('[data-submission-journey]').textContent.includes(${JSON.stringify(labels.duplicate)})`,
        ),
        true,
        `${tag}: duplicate state is missing`,
      )
      assert.equal(
        await evaluate(
          "!!document.querySelector('[data-review-unit][data-review-kind=conflict]')",
        ),
        true,
        `${tag}: conflict state is missing`,
      )
      assert.equal(
        await evaluate(
          "!!document.querySelector('[data-review-unit][data-review-kind=clarification]')",
        ),
        true,
        `${tag}: clarification state is missing`,
      )
      await assertNoRawEnums(`review ${tag}`)
      await assertNoOverflow(`review ${tag}`)
      if (width === 390) await assertSingleColumn(`review ${tag}`)

      // Long current/proposed values and a long source expand inside their
      // destination, without clipping or losing context.
      const longLayout = await evaluate(
        `(() => {
          const destination = Array.from(document.querySelectorAll('[data-review-destination]')).find((el) => el.textContent.includes('Cross-functional'));
          const source = destination?.querySelector('details[data-review-source]');
          if (source) source.open = true;
          return {
            destination: !!destination,
            before: !!destination?.textContent.includes('Existing competency'),
            after: !!destination?.textContent.includes('Cross-functional'),
            source: !!source,
            sourceContext: source?.closest('[data-review-destination]') === destination,
          };
        })()`,
      )
      assert.equal(longLayout.destination, true, `${tag}: long destination missing`)
      assert.equal(longLayout.before, true, `${tag}: current value missing`)
      assert.equal(longLayout.after, true, `${tag}: proposed value missing`)
      assert.equal(longLayout.source, true, `${tag}: long source missing`)
      assert.equal(
        longLayout.sourceContext,
        true,
        `${tag}: expanded source lost its destination`,
      )
      await assertNoOverflow(`expanded source ${tag}`)
      if (width === 390) {
        const stacked = await evaluate(
          `(() => {
            const unit = Array.from(document.querySelectorAll('[data-review-unit]')).find((el) => el.textContent.includes('Cross-functional'));
            if (!unit) return false;
            const current = Array.from(unit.querySelectorAll('p')).find((el) => el.className.includes('line-through'));
            const proposed = Array.from(unit.querySelectorAll('p')).find((el) => el.textContent.includes('Cross-functional') && !el.className.includes('line-through'));
            if (!current || !proposed) return false;
            return current.getBoundingClientRect().top >= proposed.getBoundingClientRect().bottom - 1;
          })()`,
        )
        assert.equal(stacked, true, `${tag}: current/proposed values must stack`)
      }

      // Source expansion stays keyboard operable and keeps focus on its summary.
      const sourceSummary =
        "document.querySelector('[data-review-unit] details[data-review-source] summary')"
      assert.equal(
        await evaluate(`getComputedStyle(${sourceSummary}).display`),
        "list-item",
        `${tag}: source disclosure lost its native marker`,
      )
      await focusExpression(sourceSummary)
      await pressEnter()
      await until(
        "document.querySelector('[data-review-unit] details[data-review-source]').open === true",
      )
      assert.equal(
        await evaluate(`document.activeElement === ${sourceSummary}`),
        true,
      )
      await pressEnter()
      await until(
        "document.querySelector('[data-review-unit] details[data-review-source]').open === false",
      )
      assert.equal(
        await evaluate(`document.activeElement === ${sourceSummary}`),
        true,
      )
      await capture(`review-${tag}`)

      // A group toggle is keyboard operable and updates the selected summary.
      const groupSkills = `Array.from(document.querySelectorAll('input[type=checkbox][aria-label]')).find((el) => el.getAttribute('aria-label').startsWith(${JSON.stringify(
        locale === "en" ? "Select all in Skills" : "Selecionar tudo em Habilidades",
      )}))`
      await focusExpression(groupSkills)
      await pressSpace()
      await until(`(${groupSkills}).checked === true`)
      const selectedBeforeLocale = await selectedCount()
      assert.ok(selectedBeforeLocale > 0, `${tag}: group selection failed`)

      // Edit is keyboard reachable; focus returns to the disclosure context and
      // the selected state survives both the edit and a locale change.
      const firstEdit = "document.querySelector('[data-review-unit] button[aria-expanded]')"
      await focusExpression(firstEdit)
      await pressEnter()
      await until(
        "!!document.querySelector('[data-review-unit] textarea')",
      )
      await evaluate(
        "document.querySelector('[data-review-unit] textarea').focus(); document.querySelector('[data-review-unit] textarea').select()",
      )
      await call("Input.insertText", { text: "Kubernetes and Helm" })
      await until(
        "document.querySelector('[data-review-unit] textarea').value.includes('Helm')",
      )
      await focusExpression(
        `Array.from(document.querySelectorAll('[data-review-unit] button')).find((el) => el.textContent.trim() === ${JSON.stringify(labels.saveWording)})`,
      )
      await pressEnter()
      await until(
        `document.querySelector('[data-submission-journey]').textContent.includes(${JSON.stringify(labels.edited)})`,
      )
      assert.equal(
        await evaluate(`document.activeElement === (${firstEdit})`),
        true,
        `${tag}: saving an edit did not return focus to Edit`,
      )
      const sourceText = await evaluate(
        "document.querySelector('[data-review-destination] details[data-review-source]')?.textContent || ''",
      )
      assert.match(sourceText, /Fonte original: React/)
      const questionText = await evaluate(
        "document.querySelector('[data-submission-journey]').textContent",
      )
      assert.match(questionText, /What is your French level\?/)
      const otherLocale = locale === "en" ? "pt-BR" : "en"
      const otherLabels = copy[otherLocale]
      traceLabel = `${tag} locale ${otherLocale}`
      await setLocale(otherLocale)
      await until(
        `document.querySelector('[data-submission-journey]').innerText.includes(${JSON.stringify(otherLabels.review)})`,
      )
      assert.equal(
        await selectedCount(),
        selectedBeforeLocale,
        `${tag}: locale change reset selection`,
      )
      const switchedText = await evaluate(
        "document.querySelector('[data-submission-journey]').textContent",
      )
      assert.match(switchedText, /Kubernetes and Helm/)
      assert.match(switchedText, /Fonte original: React/)
      assert.match(switchedText, /What is your French level\?/)
      assert.equal(
        await evaluate(`${byText(otherLabels.save)}.disabled`),
        false,
        `${tag}: locale change disabled a valid save`,
      )
      await assertNoRawEnums(`switched ${tag}`)
      await assertNoOverflow(`switched ${tag}`)
      traceLabel = `${tag} locale ${locale}`
      await setLocale(locale)
      await until(
        `document.querySelector('[data-submission-journey]').innerText.includes(${JSON.stringify(labels.review)})`,
      )
      assert.equal(await selectedCount(), selectedBeforeLocale)
      await assertNoRawEnums(`restored ${tag}`)

      // Discard opens a focused native dialog and Escape returns to its trigger
      // without discarding the review.
      traceLabel = `${tag} discard dialog`
      await focusExpression(byText(labels.discard))
      await pressEnter()
      await until(`${journeyDialog}?.open === true`)
      await pause(80)
      const dialogFocus = await activeInfo()
      assert.match(
        dialogFocus.text,
        new RegExp(labels.keepEditing),
        `${tag}: dialog did not focus the safe action`,
      )
      await pressKey("Tab", "Tab", 9)
      const dialogSecond = await activeInfo()
      assert.equal(
        dialogSecond.text,
        labels.discard,
        `${tag}: dialog discard is not next in keyboard order`,
      )
      await pressKey("Escape", "Escape", 27)
      await until(`${journeyDialog}?.open !== true`)
      await pause(80)
      assert.equal(
        await evaluate(
          `!document.activeElement.closest('dialog') && document.activeElement.textContent.trim() === ${JSON.stringify(labels.discard)}`,
        ),
        true,
        `${tag}: Escape did not return focus to the discard trigger`,
      )

      // The clarification can be answered with the keyboard. Focus moves to
      // the destination it now updates, not back to the top of the review.
      traceLabel = `${tag} clarification`
      const attentionSummary =
        "document.querySelector('[data-review-unit][data-review-kind=clarification] [data-review-attention-summary]')"
      await focusExpression(attentionSummary)
      await pressEnter()
      await until(
        "document.querySelector('[data-review-unit][data-review-kind=clarification] details[data-review-attention]').open === true",
      )
      await focusExpression(
        "document.querySelector('[data-review-unit][data-review-kind=clarification] [data-review-answer-open]')",
      )
      await pressEnter()
      await until(
        "document.activeElement?.tagName === 'TEXTAREA' && !!document.activeElement.closest('[data-review-unit][data-review-kind=clarification]')",
      )
      await call("Input.insertText", { text: "French B2" })
      await until("document.activeElement.value === 'French B2'")
      await focusExpression(
        "document.querySelector('[data-review-unit][data-review-kind=clarification] [data-review-answer] button[type=submit]')",
      )
      await pressEnter()
      await until(
        "!document.querySelector('[data-review-unit][data-review-kind=clarification]')",
      )
      await until(
        "(document.activeElement.closest('[data-review-unit]')?.textContent || '').includes('French B2')",
      )
      assert.notEqual(await evaluate("document.activeElement?.id"), "review-heading")

      // Select every destination group, fail the first save, focus the alert,
      // then retry without another provider call.
      traceLabel = `${tag} save`
      await evaluate(
        `Array.from(document.querySelectorAll('[data-review-destination] input[type=checkbox][aria-label]')).filter((el) => /Select all in|Selecionar tudo em/.test(el.getAttribute('aria-label'))).forEach((el) => { if (!el.checked) el.click() })`,
      )
      await until(
        `${byText(labels.save)} && ${byText(labels.save)}.disabled === false`,
      )
      const callsBeforeSave = await evaluate("window.__journeyCalls.length")
      await evaluate("window.__failSaveOnce = true")
      await focusExpression(byText(labels.save))
      await pressEnter()
      await until(`!!${byText(labels.retrySave)}`)
      await pause(80)
      assert.equal((await activeInfo()).role, "alert")
      assert.equal(await evaluate("window.__profileWrites"), 0)
      await capture(`save-failure-${tag}`)
      await focusExpression(byText(labels.retrySave))
      await pressEnter()
      await until("!!document.querySelector('[data-submission-saved]')")
      await pause(80)
      assert.equal(
        await evaluate(
          "document.activeElement?.hasAttribute('data-submission-saved')",
        ),
        true,
        `${tag}: saved status did not receive focus`,
      )
      assert.equal(await evaluate("window.__profileWrites"), 1)
      assert.equal(
        await evaluate("window.__journeyCalls.length"),
        callsBeforeSave,
        `${tag}: retrying a save called a provider`,
      )
      const saved = JSON.parse(await evaluate(savedProfileExpression))
      assert.match(saved.skills, /Kubernetes/)
      assert.match(saved.careerGoals, /mentor/)
      assert.match(saved.tools, /Terraform/)
      assert.match(saved.competencies, /Cross-functional/)
      assert.match(saved.additionalInfo, /French B2/)
      await assertNoOverflow(`saved ${tag}`)
      await capture(`saved-${tag}`)
      console.log(`PASS integrated EN/PT rich review ${tag}`)

      // Processing failure and the distinct Stop/Resume/Retry/Discard controls.
      traceLabel = `${tag} failure`
      await resetStorage(null, locale)
      await installMock("partial")
      await watchWrites()
      await openProfile()
      await openJourneyLocalized(labels)
      await fill(
        "#submission-text",
        "I use TypeScript. I led a payments project.",
      )
      await evaluate("window.__holdExtract = true")
      await focusExpression(byText(labels.prepare))
      await pressEnter()
      await until("window.__extractHeld === true")
      await until(
        `document.activeElement?.textContent.trim() === ${JSON.stringify(labels.stop)}`,
      )
      await pressEnter()
      await until("window.__extractAborted === true")
      await until("!!document.querySelector('[data-submission-recovery]')")
      await until(
        "document.activeElement === document.querySelector('[data-submission-recovery]')",
      )
      await focusExpression(byText(labels.resume))
      await pressEnter()
      await until(`!!${byText(labels.retry)}`)
      await until(
        "document.activeElement === document.querySelector('[data-submission-recovery]')",
      )
      await assertNoOverflow(`processing failure ${tag}`)
      await assertNoRawEnums(`processing failure ${tag}`)
      await capture(`failure-${tag}`)
      await focusExpression(byText(labels.retry))
      await pressEnter()
      await until("!!document.querySelector('#review-heading')")
      await until("document.activeElement?.id === 'review-heading'")
      await assertNoOverflow(`failure review ${tag}`)
      await capture(`failure-review-${tag}`)

      traceLabel = `${tag} failure discard`
      await focusExpression(byText(labels.discard))
      await pressEnter()
      await until(`${journeyDialog}?.open === true`)
      await pause(80)
      assert.match((await activeInfo()).text, new RegExp(labels.keepEditing))
      await pressKey("Tab", "Tab", 9)
      assert.equal((await activeInfo()).text, labels.discard)
      assert.equal(
        await evaluate("!!document.activeElement.closest('dialog')"),
        true,
        `${tag}: dialog discard is not inside the dialog`,
      )
      await pressEnter()
      await until(`${journeyDialog}?.open !== true`)
      await until("!!document.querySelector('#submission-text')")
      await pause(80)
      assert.equal(
        await evaluate("document.activeElement?.id"),
        "submission-text",
        `${tag}: discard did not return focus to the empty draft`,
      )
      assert.equal(
        await evaluate("document.querySelector('#submission-text').value"),
        "",
      )
      assert.equal(await evaluate("window.__profileWrites"), 0)
      console.log(`PASS integrated EN/PT failure and keyboard controls ${tag}`)
    }
  }
  writeFileSync(
    join(work, "keyboard-trace.json"),
    JSON.stringify(keyboardTrace, null, 2),
  )
  console.log("Screenshots:", work)
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}
