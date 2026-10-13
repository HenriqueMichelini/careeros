import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { execFileSync } from "node:child_process"
import { readFileSync, writeFileSync } from "node:fs"
import { join } from "node:path"
import { reviewedDraftFixture } from "./reviewed-resume-fixture.mjs"
import { jobContextFixture } from "./job-context-fixtures.mjs"

// Controlled reconstructions for Results delivery/export, never historical raw
// responses or provider-quality samples. CDP captures the browser's actual HTTP
// bodies; the response boundary is fulfilled locally with synthetic fixtures.
export async function checkResultsRelease({
  call,
  evaluate,
  until,
  pause,
  onEvent,
  work,
  root,
  origin,
}) {
  const hash = (text) => createHash("sha256").update(text).digest("hex")
  const template = JSON.parse(
    readFileSync(
      join(root, "docs/evaluations/workflow-settings/cases.v1.json"),
      "utf8",
    ),
  ).cases.find((c) => c.id === "pt-reviewed-draft")
  const ptMarkdown = readFileSync(
    join(root, "docs/release-evidence/issue-52/frozen-ruby-cv-copy.pt.txt"),
    "utf8",
  ).trim()
  const captures = new Map()
  const profileComparisons = []
  const failures = []
  let current
  onEvent("Network.requestWillBeSent", ({ requestId, request, timestamp }) => {
    if (request.url.startsWith(origin + "/api/"))
      captures.set(requestId, {
        url: request.url,
        requestBody: request.postData,
        started: timestamp,
      })
  })
  onEvent("Network.responseReceived", ({ requestId, response }) => {
    const capture = captures.get(requestId)
    if (capture) {
      capture.status = response.status
      capture.headers = Object.fromEntries(
        Object.entries(response.headers).filter(([key]) =>
          ["content-type", "cache-control"].includes(key.toLowerCase()),
        ),
      )
    }
  })
  onEvent("Network.loadingFinished", async ({ requestId, timestamp }) => {
    const capture = captures.get(requestId)
    if (!capture) return
    try {
      const result = await call("Network.getResponseBody", { requestId })
      capture.responseBody = result.base64Encoded
        ? Buffer.from(result.body, "base64").toString("utf8")
        : result.body
      capture.durationMs = (timestamp - capture.started) * 1000
    } catch (error) {
      failures.push(error)
    }
  })
  onEvent("Fetch.requestPaused", async ({ requestId, request }) => {
    try {
      const input = JSON.parse(request.postData)
      assert.equal(input.jobPosting, current.posting)
      assert.deepEqual(input.profileEvidence, current.doc)
      const output = request.url.endsWith("qualification-gaps")
        ? { gaps: [], jobContext: jobContextFixture(current.posting) }
        : reviewedDraftFixture(current.materials, input)
      await call("Fetch.fulfillRequest", {
        requestId,
        responseCode: 200,
        responseHeaders: [
          { name: "Content-Type", value: "application/json" },
          { name: "Cache-Control", value: "no-store" },
        ],
        body: Buffer.from(JSON.stringify(output)).toString("base64"),
      })
    } catch (error) {
      failures.push(error)
      await call("Fetch.failRequest", { requestId, errorReason: "Failed" })
    }
  })
  await call("Network.enable")
  await call("Fetch.enable", {
    patterns: [{ urlPattern: origin + "/api/*", requestStage: "Request" }],
  })
  await call("Browser.grantPermissions", {
    origin,
    permissions: ["clipboardReadWrite", "clipboardSanitizedWrite"],
  })
  await call("Page.bringToFront")
  const key = async (name, code, keyCode) => {
    await call("Input.dispatchKeyEvent", {
      type: "keyDown",
      key: name,
      code,
      windowsVirtualKeyCode: keyCode,
      ...(name === "Enter" ? { text: "\r" } : {}),
    })
    await call("Input.dispatchKeyEvent", {
      type: "keyUp",
      key: name,
      code,
      windowsVirtualKeyCode: keyCode,
    })
  }
  const nativeCopy = async (selector, expected, method, copiedLabel) => {
    await evaluate("navigator.clipboard.writeText('')")
    await evaluate(
      `document.querySelector(${JSON.stringify(selector)}).focus()`,
    )
    if (method === "pointer") {
      const rect = await evaluate(
        `(() => { const r=document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect(); return {x:r.x+r.width/2,y:r.y+r.height/2} })()`,
      )
      await call("Input.dispatchMouseEvent", {
        type: "mousePressed",
        ...rect,
        button: "left",
        clickCount: 1,
      })
      await call("Input.dispatchMouseEvent", {
        type: "mouseReleased",
        ...rect,
        button: "left",
        clickCount: 1,
      })
    } else
      await key(
        method === "Enter" ? "Enter" : " ",
        method === "Enter" ? "Enter" : "Space",
        method === "Enter" ? 13 : 32,
      )
    await until(
      `document.querySelector(${JSON.stringify(selector)}).textContent.trim() === ${JSON.stringify(copiedLabel)}`,
    )
    assert.equal(
      await evaluate("navigator.clipboard.readText()"),
      expected,
      `${method} must transfer the complete accepted text through the native clipboard`,
    )
  }
  try {
    for (const locale of ["en", "pt-BR"]) {
      const pt = locale === "pt-BR"
      const doc = structuredClone(template.request.profileEvidence)
      doc.facts[0].value = doc.facts[0].normalization.observed = "Ruby"
      doc.evidence[0].excerpt = pt
        ? "Uso Ruby para manter ferramentas CLI internas."
        : "I use Ruby to maintain internal CLI tools."
      const markdown = pt
        ? ptMarkdown
        : "## Technical Skills\n- Ruby: maintaining internal CLI tools."
      const posting = pt
        ? "Desenvolvedor Ruby.\nMantenha ferramentas CLI internas usando Ruby.\nRuby é obrigatório.\nExplique como você usa Ruby em português."
        : "Ruby developer.\nMaintain internal CLI tools using Ruby.\nRuby is required.\nExplain how you use Ruby in English."
      const answer = pt
        ? "Uso Ruby para manter ferramentas CLI internas."
        : "I use Ruby to maintain internal CLI tools."
      const answers = pt
        ? "## Respostas solicitadas pela empresa\n" +
          answer +
          "\n\n## Sugestões adicionais\nQue ferramentas a equipe mantém?"
        : "## Employer-requested answers\n" +
          answer +
          "\n\n## Additional suggestions\nWhich tools does the team maintain?"
      current = {
        doc,
        posting,
        materials: {
          jobTitle: pt ? "Desenvolvedor Ruby" : "Ruby developer",
          company: null,
          jobSummary: posting,
          resume: markdown,
          coverLetter: {
            greeting: pt
              ? "Prezada equipe de contratação,"
              : "Dear hiring team,",
            body: answer,
            closing: pt ? "Atenciosamente," : "Sincerely,",
          },
          applicationAnswers: answers,
        },
      }
      await evaluate(
        `localStorage.clear();localStorage.setItem('careeros_profile_v2',${JSON.stringify(JSON.stringify(doc))});localStorage.setItem('careeros_locale',${JSON.stringify(locale)});localStorage.setItem('careeros_cv_language',${JSON.stringify(locale)});localStorage.setItem('careeros_apikey','sk-release-capture');localStorage.setItem('careeros_typesafe_key','release-capture');window.__releaseReloadPending=true;location.reload()`,
      )
      await until(
        "!window.__releaseReloadPending && document.readyState==='complete' && !!document.querySelector('header button')",
      )
      await evaluate(
        "document.querySelector('header button:last-child').click()",
      )
      await until("document.querySelectorAll('nav button').length===4")
      await evaluate("document.querySelectorAll('nav button')[0].click()")
      await until("!!document.querySelector('main textarea')")
      await evaluate(
        `{const el=document.querySelector('main textarea');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,${JSON.stringify(posting)});el.dispatchEvent(new Event('input',{bubbles:true}))}`,
      )
      const generate = `Array.from(document.querySelectorAll('main button')).find(b=>/Generate|Gerar/.test(b.textContent) && !b.disabled)`
      await until(`!!(${generate})`)
      await evaluate(`${generate}.click()`)
      await until("!!document.querySelector('[role=dialog]')")
      await evaluate(
        "Array.from(document.querySelectorAll('[role=dialog] button')).at(-2).click()",
      )
      await until("!!document.querySelector('.artifact-review')")
      const acceptArtifacts = pt
        ? "Aceitar materiais da candidatura"
        : "Accept application materials"
      await evaluate(
        `Array.from(document.querySelectorAll('.artifact-review button')).find(b=>b.textContent.trim()===${JSON.stringify(acceptArtifacts)}).click()`,
      )
      await evaluate(
        "document.querySelectorAll('.results-tabs button')[1].click()",
      )
      await until("!!document.querySelector('.resume-review')")
      const acceptResume = pt ? "Aceitar currículo" : "Accept resume"
      await evaluate(
        `Array.from(document.querySelectorAll('.resume-review button')).find(b=>b.textContent.trim()===${JSON.stringify(acceptResume)}).click()`,
      )
      await until("!!document.querySelector('.cv-paper-content')")
      await evaluate(
        "window.__nativeClipboard=navigator.clipboard;window.__nativeExecCommand=document.execCommand",
      )
      for (const [native, fallback, success] of [
        ["reject", "false", false],
        ["missing", "false", false],
        ["throw", "throw", false],
        ["missing", "true", true],
        ["reject", "true", true],
      ]) {
        await evaluate(
          `Object.defineProperty(navigator,'clipboard',{configurable:true,value:${
            native === "missing"
              ? "undefined"
              : native === "throw"
                ? "{writeText:()=>{throw Error('Denied')}}"
                : "{writeText:async()=>{throw Error('Denied')}}"
          }});document.execCommand=()=>{${
            fallback === "throw"
              ? "throw Error('Unavailable')"
              : `window.__fallbackText=document.querySelector('textarea').value;return ${fallback}`
          }};document.querySelector('.results-resume-actions button').focus();document.querySelector('.results-resume-actions button').click()`,
        )
        await until(
          success
            ? `document.querySelector('.results-resume-actions button').textContent.trim()===${JSON.stringify(pt ? "Copiado!" : "Copied!")}`
            : "!!document.querySelector('.results-page [role=alert]')",
        )
        if (success)
          assert.equal(
            await evaluate("window.__fallbackText"),
            markdown,
            "acknowledged legacy copy receives the complete accepted text",
          )
        else
          assert.equal(
            await evaluate(
              "document.querySelector('.results-resume-actions button').textContent.trim()",
            ),
            pt ? "Copiar texto" : "Copy text",
          )
        assert.equal(
          await evaluate(
            "document.activeElement===document.querySelector('.results-resume-actions button')",
          ),
          true,
          "legacy copying preserves keyboard focus",
        )
      }
      await evaluate(
        "Object.defineProperty(navigator,'clipboard',{configurable:true,value:window.__nativeClipboard});document.execCommand=window.__nativeExecCommand",
      )
      for (const width of [1280, 390]) {
        await call("Emulation.setDeviceMetricsOverride", {
          width,
          height: 844,
          deviceScaleFactor: 1,
          mobile: false,
        })
        assert.equal(
          await evaluate("document.documentElement.scrollWidth <= innerWidth"),
          true,
        )
        assert.equal(
          await evaluate(
            "document.querySelector('.cv-paper-content section h3').textContent.trim()",
          ),
          pt ? "Competências Técnicas" : "Technical Skills",
        )
        for (const method of ["Enter", "Space", "pointer"])
          await nativeCopy(
            ".results-resume-actions button",
            markdown,
            method,
            pt ? "Copiado!" : "Copied!",
          )
        await evaluate(
          "window.__printCalled=false;window.print=()=>{window.__printCalled=true};document.querySelector('.results-resume-actions button:last-child').click()",
        )
        await until(
          "window.__printCalled && !!document.querySelector('.results-print-root')",
        )
        const pdf = join(work, `ruby-${locale}-${width}.pdf`)
        writeFileSync(
          pdf,
          Buffer.from(
            (
              await call("Page.printToPDF", {
                printBackground: true,
                preferCSSPageSize: true,
              })
            ).data,
            "base64",
          ),
        )
        assert.match(
          execFileSync("pdfinfo", [pdf], { encoding: "utf8" }),
          /Pages:\s+1\b[\s\S]*\(A4\)/,
        )
        const pdfText = execFileSync("pdftotext", ["-raw", pdf, "-"], {
          encoding: "utf8",
        })
        assert.match(
          pdfText,
          pt ? /Competências Técnicas/i : /Technical Skills/i,
        )
        assert.equal(
          pdfText.split(
            pt
              ? "Ruby: manutenção de ferramentas CLI internas."
              : "Ruby: maintaining internal CLI tools.",
          ).length - 1,
          1,
        )
        assert.doesNotMatch(
          pdfText,
          /HABILIDADES TÉCNICAS|Generated Application|Application Q&A/,
        )
        execFileSync("pdftoppm", [
          "-scale-to",
          "1200",
          "-singlefile",
          "-png",
          pdf,
          pdf.slice(0, -4),
        ])
        await evaluate("window.dispatchEvent(new Event('afterprint'))")
      }
      for (const [tab, expected] of [
        [0, posting],
        [
          2,
          [
            current.materials.coverLetter.greeting,
            answer,
            current.materials.coverLetter.closing,
          ].join("\n\n"),
        ],
        [3, answers],
      ]) {
        await evaluate(
          `document.querySelectorAll('.results-tabs button')[${tab}].click()`,
        )
        if (tab !== 0)
          assert.equal(
            await evaluate(
              "document.querySelector('.results-content pre').textContent",
            ),
            expected,
          )
        if (tab !== 0)
          for (const method of ["Enter", "Space", "pointer"])
            await nativeCopy(
              ".results-other-actions button",
              expected,
              method,
              pt ? "Copiado!" : "Copied!",
            )
      }
      assert.deepEqual(
        await evaluate(
          "JSON.parse(localStorage.getItem('careeros_profile_v2'))",
        ),
        doc,
        "accepted drafts and exports must preserve the full saved Profile",
      )
      profileComparisons.push({
        locale,
        fullParsedProfileUnchanged: true,
        beforeHash: hash(JSON.stringify(doc)),
        afterHash: hash(
          await evaluate("localStorage.getItem('careeros_profile_v2')"),
        ),
      })
    }
    for (
      let attempt = 0;
      attempt < 100 && [...captures.values()].some((c) => !c.responseBody);
      attempt++
    )
      await pause(30)
    assert.deepEqual(
      failures.map((e) => e.message),
      [],
    )
    assert.equal(
      captures.size,
      4,
      "one qualification and draft per language, without automatic retries",
    )
    for (const capture of captures.values()) {
      assert.equal(capture.status, 200)
      assert.ok(capture.requestBody && capture.responseBody)
      assert.ok(capture.durationMs >= 0)
      assert.ok(
        !JSON.stringify(capture).includes("sk-release-capture"),
        "authentication headers must never enter the capture artifact",
      )
      capture.requestHash = hash(capture.requestBody)
      capture.responseHash = hash(capture.responseBody)
      capture.requestBytes = Buffer.byteLength(capture.requestBody)
      capture.responseBytes = Buffer.byteLength(capture.responseBody)
      delete capture.started
    }
    await evaluate("navigator.clipboard.writeText('')")
    assert.equal(
      await evaluate("navigator.clipboard.readText()"),
      "",
      "cleanup must acknowledge and read back an empty native clipboard before reload",
    )
    await evaluate(
      "localStorage.removeItem('careeros_apikey');localStorage.removeItem('careeros_typesafe_key');window.__releaseReloadPending=true;location.reload()",
    )
    await until(
      "!window.__releaseReloadPending && document.readyState==='complete' && !!document.querySelector('header button')",
    )
    assert.equal(
      await evaluate("localStorage.getItem('careeros_apikey')"),
      null,
    )
    assert.equal(
      await evaluate("localStorage.getItem('careeros_typesafe_key')"),
      null,
    )
    assert.equal(
      await evaluate("navigator.clipboard.readText()"),
      "",
      "clipboard remains empty after the fresh load",
    )
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('nav button').length===4")
    await evaluate("document.querySelectorAll('nav button')[0].click()")
    await until("!!document.querySelector('main textarea')")
    await evaluate(
      `{const el=document.querySelector('main textarea');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,${JSON.stringify(current.posting)});el.dispatchEvent(new Event('input',{bubbles:true}))}`,
    )
    await until(
      "Array.from(document.querySelectorAll('[data-checklist-state]')).map(e=>e.dataset.checklistState).join(',')==='pending,ready,ready'",
    )
    assert.equal(
      await evaluate("document.querySelector('#job-typesafe-key').value"),
      "",
    )
    assert.equal(
      await evaluate(
        "Array.from(document.querySelectorAll('main button')).find(b=>/Generate|Gerar/.test(b.textContent)).disabled",
      ),
      true,
      "fresh-load generation must stay disabled without keys despite a ready Profile and posting",
    )
    writeFileSync(
      join(work, "controlled-browser-capture.json"),
      JSON.stringify(
        {
          evidence:
            "synthetic browser response reconstruction; no provider calls",
          sourceHashes: Object.fromEntries(
            [
              "src/pages/ResultsPage.tsx",
              "src/components/GeneratedResumePreview.tsx",
              "src/lib/ai.ts",
              "backend/application-draft/main.go",
              "backend/application-draft/artifacts.go",
              "tests/results-release-checks.mjs",
            ].map((path) => [path, hash(readFileSync(join(root, path)))]),
          ),
          nativeClipboard: {
            locales: ["en", "pt-BR"],
            resumeViewports: [1280, 390],
            methods: ["Enter", "Space", "pointer"],
            artifacts: ["resume", "coverLetter", "applicationAnswers"],
            successfulTransfers: 24,
          },
          cleanup: {
            clipboardEmptyBeforeAndAfterReload: true,
            storedKeysAbsent: true,
            renderedMissingKeyReadiness: true,
          },
          profileComparisons,
          captures: [...captures.values()],
        },
        null,
        2,
      ),
    )
    console.log(
      "EN/PT native Enter/Space/pointer clipboard, retained PT headings, four actual A4 PDFs, full Profile preservation and synthetic HTTP capture: passed",
    )
  } finally {
    await call("Fetch.disable")
    for (const method of [
      "Network.requestWillBeSent",
      "Network.responseReceived",
      "Network.loadingFinished",
      "Fetch.requestPaused",
    ])
      onEvent(method, null)
  }
}
