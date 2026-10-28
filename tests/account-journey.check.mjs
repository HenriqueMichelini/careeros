// Real React screens + real PostgreSQL auth/Profile APIs. OIDC and AI are
// deterministic test fixtures. Every workflow still crosses the server guard.
import assert from "node:assert/strict"
import { spawn, execFileSync } from "node:child_process"
import { createServer } from "node:net"
import { mkdtempSync, writeFileSync, openSync, readFileSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { withAccountBrowser, pause } from "./account-browser.fixture.mjs"
import { journeyFetch } from "./submission-browser.fixture.mjs"
import { jobContextFixture } from "./job-context-fixtures.mjs"
import { reviewedDraftFixture } from "./reviewed-resume-fixture.mjs"

if (!process.env.CAREEROS_TEST_DATABASE_URL)
  throw new Error(
    "Use an isolated synthetic PostgreSQL database via CAREEROS_TEST_DATABASE_URL",
  )
const root = new URL("..", import.meta.url).pathname
const output = mkdtempSync(join(tmpdir(), "careeros-first-draft-"))
// Keep executable output in a private, atomically created checkout directory.
// The remaining evidence is synthetic and can stay in the OS temp directory.
const build = mkdtempSync(join(root, ".careeros-journey-build-"))
process.once("exit", () => rmSync(build, { recursive: true, force: true }))
const binary = join(build, "journey-server.test")
// This Linux browser harness also uses /usr/bin/google-chrome explicitly.
execFileSync("/usr/local/go/bin/go", ["test", "-c", "-o", binary, "./internal/accountapi"], {
  cwd: root,
  env: { ...process.env, GOCACHE: "/tmp/careeros-go-cache" },
})
const freePort = () =>
  new Promise((resolve) => {
    const s = createServer()
    s.listen(0, "127.0.0.1", () => {
      const p = s.address().port
      s.close(() => resolve(p))
    })
  })

function installWorkflows(ingestion, context, reviewed) {
  window.__realFetch = window.fetch.bind(window)
  window.__calls = []
  window.__mode = "success"
  window.fetch = async (url, options = {}) => {
    const response = await window.__realFetch(url, options)
    if (
      typeof url !== "string" ||
      !/^\/api\/(profile\/ingest|qualification-gaps|application-draft)$/.test(url) ||
      !response.ok
    )
      return response
    const body = JSON.parse(options.body)
    window.__calls.push({
      url,
      body,
      account: new Headers(options.headers).get("X-CareerOS-Account"),
    })
    if (url === "/api/profile/ingest") return ingestion(url, options)
    if (url === "/api/qualification-gaps")
      return new Response(
        JSON.stringify({
          gaps: [],
          matches: [],
          jobContext: context(body.jobPosting),
        }),
      )
    if (window.__mode === "hold") {
      window.__held = true
      await new Promise((resolve, reject) => {
        window.__release = resolve
        options.signal.addEventListener(
          "abort",
          () => reject(new DOMException("Aborted", "AbortError")),
          { once: true },
        )
      })
    }
    if (window.__mode === "failure")
      return new Response(JSON.stringify({ error: "outage" }), { status: 503 })
    if (window.__mode === "incomplete")
      return new Response(JSON.stringify({ jobSummary: "Clipped output" }))
    const skill = body.profileEvidence.facts.find(
      (f) => f.field === "skills",
    ).value
    return new Response(
      JSON.stringify(
        reviewed(
          {
            jobTitle: null,
            company: null,
            jobSummary: "TypeScript developer.",
            resume: `## Technical Skills\n- ${skill}`,
            coverLetter: {
              greeting:
                body.cvLanguage === "pt-BR" ? "Prezada equipe," : "Dear team,",
              body:
                body.cvLanguage === "pt-BR"
                  ? "Tenho experiência com TypeScript."
                  : "I use TypeScript.",
              closing:
                body.cvLanguage === "pt-BR" ? "Atenciosamente," : "Sincerely,",
            },
            applicationAnswers:
              body.cvLanguage === "pt-BR"
                ? "Tenho experiência com TypeScript."
                : "I use TypeScript.",
          },
          body,
        ),
      ),
    )
  }
}

const reports = []
for (const [locale, width] of [
  ["en", 1440],
  ["pt-BR", 390],
]) {
  const apiPort = await freePort()
  let server
  try {
    await withAccountBrowser(
      async ({ evidence, call, evaluate, wait, port }) => {
        const pt = locale === "pt-BR"
        await call("Emulation.setDeviceMetricsOverride", {
          width,
          height: 1000,
          deviceScaleFactor: 1,
          mobile: width < 500,
        })
        const click = async (text) => {
          const clicked = await evaluate(
            `(()=>{const b=[...document.querySelectorAll('button,a')].filter(b=>b.textContent.trim()===${JSON.stringify(text)} && !b.disabled && !b.closest('[hidden]')).at(-1);if(!b)return false;b.click();return true})()`,
          )
          assert.equal(clicked, true, "Missing enabled control: " + text)
        }
        const fill = async (selector, text) => {
          await evaluate(
            `document.querySelector(${JSON.stringify(selector)}).focus();document.querySelector(${JSON.stringify(selector)}).select()`,
          )
          await call("Input.insertText", { text })
        }
        const navigate = async (index) => {
          await evaluate(
            `document.querySelectorAll('header nav button')[${index}].click()`,
          )
          await wait(
            `document.querySelectorAll('header nav button')[${index}].getAttribute('aria-current')==='page'`,
          )
        }
        const readProfile = () =>
          evaluate(
            `(async()=>{const s=await(await window.__realFetch('/api/auth/session')).json();return (await(await window.__realFetch('/api/profile/current',{headers:{'X-CareerOS-Account':s.account.id}})).json()).document})()`,
          )
        const shot = async (name) => {
          const data = await call("Page.captureScreenshot", {
            format: "png",
            captureBeyondViewport: true,
          })
          writeFileSync(
            join(output, `${name}-${locale}-${width}.png`),
            Buffer.from(data.data, "base64"),
          )
        }
        const signIn = async (name) => {
          await evaluate(
            `window.__realFetch('/api/auth/demo-account?name=${name}')`,
          )
          await evaluate(`location.href='/api/auth/google/start'`)
          await wait(
            `document.querySelectorAll('header nav button').length===4`,
          )
        }
        await wait(
          `!!document.querySelector('a[href="/api/auth/google/start"]')`,
        )
        await click(pt ? "Entrar com Google" : "Sign in with Google")
        await wait(`!!document.querySelector('#submission-text')`)
        assert.equal(await readProfile(), null)
        assert.equal(
          await evaluate(
            `document.querySelectorAll('header nav button')[1].getAttribute('aria-current')`,
          ),
          "page",
        )
        await navigate(0)
        await fill("#application-job-posting", "TypeScript developer.")
        assert.equal(
          await evaluate(
            `document.querySelector('.journey-guidance button').textContent.trim()`,
          ),
          pt ? "Abrir Perfil" : "Open Profile",
        )
        assert.equal(await evaluate(`window.__calls.length`), 0)
        await navigate(1)
        await evaluate(
          `document.querySelector('[data-api-key-settings]').click()`,
        )
        await fill("#openai-key", "sk-synthetic")
        await click(pt ? "Salvar" : "Save")
        await evaluate(
          `document.querySelector('[data-submission-journey] details').open=true`,
        )
        await wait(`!!document.querySelector('#typesafe-key')`)
        await fill("#typesafe-key", "synthetic-typesafe")
        await fill("#submission-text", "I use TypeScript.")
        await click(pt ? "Preparar alterações" : "Prepare changes")
        await wait(`!!document.querySelector('#review-heading')`)
        assert.equal(
          await readProfile(),
          null,
          "Proposal preparation must not Save",
        )
        await click(pt ? "Selecionar adições claras" : "Select clear additions")
        await shot("profile-review")
        await click(pt ? "Salvar no Perfil" : "Save to Profile")
        await wait(`!!document.querySelector('[data-submission-saved]')`)
        const saved = await readProfile()
        assert.equal(
          saved.facts.some(
            (f) => f.field === "skills" && f.value === "TypeScript",
          ),
          true,
        )
        assert.equal(
          await evaluate(`localStorage.getItem('careeros_profile_v2')`),
          "LEGACY_SENTINEL",
        )
        assert.equal(
          await evaluate(`localStorage.getItem('careeros_curated_cv_v1')`),
          null,
        )
        // Deliberate keyboard transition into Apply and focus transfer.
        await evaluate(`document.querySelector('main').focus()`)
        for (const [key, code, n] of [
          ["Tab", "Tab", 9],
          ["Enter", "Enter", 13],
        ]) {
          await call("Input.dispatchKeyEvent", {
            type: "keyDown",
            key,
            code,
            windowsVirtualKeyCode: n,
            ...(n === 13 ? { text: "\r" } : {}),
          })
          await call("Input.dispatchKeyEvent", {
            type: "keyUp",
            key,
            code,
            windowsVirtualKeyCode: n,
          })
        }
        await wait(
          `document.querySelectorAll('header nav button')[0].getAttribute('aria-current')==='page'`,
        )
        assert.equal(
          await evaluate(
            `document.activeElement===document.querySelector('main')`,
          ),
          true,
        )
        assert.equal(
          await evaluate(
            `document.querySelector('#application-job-posting').labels.length`,
          ),
          1,
        )
        await fill("#application-job-posting", "TypeScript developer.")
        const generate = async () => {
          await click(pt ? "Gerar materiais" : "Generate Materials")
          await wait(`!!document.querySelector('[role=dialog]')`)
          await click(pt ? "Gerar materiais" : "Generate Materials")
        }
        await generate()
        await wait(`!!document.querySelector('.results-page-header')`)
        assert.deepEqual(
          await readProfile(),
          saved,
          "Generation never writes Profile facts",
        )
        const request = await evaluate(
          `window.__calls.find(c=>c.url==='/api/application-draft')`,
        )
        assert.equal(request.body.profileEvidence.id, saved.id)
        assert.equal(request.body.repository.skills, "TypeScript")
        await click(
          pt
            ? "Aceitar materiais da candidatura"
            : "Accept application materials",
        )
        await evaluate(
          `document.querySelector('.results-tabs button:nth-child(2)')?.click()`,
        )
        if (!(await evaluate(`!!document.querySelector('.resume-review')`))) {
          await click(pt ? "Currículo" : "Resume")
        }
        await click(pt ? "Aceitar currículo" : "Accept resume")
        await wait(
          `!!document.querySelector('.cv-render') || !!document.querySelector('.results-resume-actions button:not(:disabled)')`,
        )
        assert.equal(
          await evaluate(
            `document.querySelectorAll('.results-resume-actions button:disabled').length`,
          ),
          0,
        )
        await call("Browser.grantPermissions", {
          origin: `http://127.0.0.1:${port}`,
          permissions: ["clipboardReadWrite", "clipboardSanitizedWrite"],
        })
        await evaluate(
          `document.querySelector('.results-resume-actions button').click()`,
        )
        await wait(
          `document.querySelector('.results-resume-actions button').textContent.match(/Copied|Copiado/)`,
        )
        await evaluate(
          `window.print=()=>{window.__printed=true};document.querySelectorAll('.results-resume-actions button')[1].click()`,
        )
        await wait(`window.__printed===true`)
        const pdf = await call("Page.printToPDF", {
          printBackground: true,
          preferCSSPageSize: true,
        })
        writeFileSync(
          join(output, `draft-${locale}-${width}.pdf`),
          Buffer.from(pdf.data, "base64"),
        )
        await shot("results")
        for (const mode of ["incomplete", "failure", "hold"]) {
          await navigate(0)
          await evaluate(
            `window.__mode=${JSON.stringify(mode)};window.__held=false`,
          )
          await generate()
          if (mode === "hold") {
            await wait(`window.__held`)
            await click(pt ? "Cancelar solicitação" : "Cancel request")
            await wait(`!document.querySelector('[role=dialog]')`)
          } else await wait(`!!document.querySelector('[role=alert]')`)
          const count = await evaluate(
            `window.__calls.filter(c=>c.url==='/api/application-draft').length`,
          )
          await pause(100)
          assert.equal(
            await evaluate(
              `window.__calls.filter(c=>c.url==='/api/application-draft').length`,
            ),
            count,
            "No automatic retry",
          )
          if (await evaluate(`!!document.querySelector('[role=dialog]')`))
            await click(pt ? "Voltar à vaga" : "Back to posting")
          await navigate(3)
          assert.equal(
            await evaluate(`document.body.innerText.includes('TypeScript')`),
            true,
          )
          assert.deepEqual(await readProfile(), saved)
        }
        await shot("previous-preserved")
        // A second valid candidate preserves the prior accepted one in this tab.
        await navigate(0)
        await evaluate(`window.__mode='success'`)
        await generate()
        await wait(`!!document.querySelector('[data-previous-draft]')`)
        await evaluate(
          `document.querySelector('[data-previous-draft]').click()`,
        )
        await wait(`document.body.innerText.includes('TypeScript')`)
        // Reload and sign-in replacement restore PostgreSQL Profile, not Drafts.
        await call("Page.reload")
        await wait(`!!document.querySelector('#application-job-posting')`)
        assert.deepEqual(await readProfile(), saved)
        await signIn("alpha")
        assert.deepEqual(await readProfile(), saved)
        await signIn("beta")
        await wait(`!!document.querySelector('#submission-text')`)
        assert.equal(await readProfile(), null)
        assert.equal(
          await evaluate(`document.body.innerText.includes('TypeScript')`),
          false,
        )
        await signIn("alpha")
        assert.deepEqual(await readProfile(), saved)
        assert.equal(
          await evaluate(`localStorage.getItem('careeros_apikey')`),
          null,
        )
        assert.equal(
          await evaluate(`document.documentElement.scrollWidth<=innerWidth`),
          true,
        )
        reports.push({
          locale,
          width,
          status: "passed",
          profileId: saved.id,
          evidence,
        })
      },
      {
        path: "/",
        env: {
          VITE_CAREEROS_ACCOUNT_MODE: "1",
          CAREEROS_API_TARGET: `http://127.0.0.1:${apiPort}`,
        },
        initialize: `localStorage.setItem('careeros_locale',${JSON.stringify(locale)});(${installWorkflows.toString()})(${journeyFetch.toString()},${jobContextFixture.toString()},${reviewedDraftFixture.toString()})`,
        beforeNavigate: async ({ port }) => {
          const log = openSync(join(output, `server-${locale}.log`), "w")
          server = spawn(
            binary,
            ["-test.run=^TestJourneyDemoServer$", "-test.timeout=180s"],
            {
              cwd: join(root, "internal/accountapi"),
              env: {
                ...process.env,
                CAREEROS_JOURNEY_DEMO: "1",
                CAREEROS_JOURNEY_ORIGIN: `http://127.0.0.1:${port}`,
                CAREEROS_JOURNEY_ADDR: `127.0.0.1:${apiPort}`,
              },
              stdio: ["ignore", log, log],
            },
          )
          for (let i = 0; i < 100; i++) {
            try {
              if ((await fetch(`http://127.0.0.1:${apiPort}/ready`)).ok) return
            } catch {}
            await pause(50)
          }
          throw new Error(
            readFileSync(join(output, `server-${locale}.log`), "utf8"),
          )
        },
      },
    )
  } finally {
    server?.kill("SIGTERM")
  }
}
writeFileSync(
  join(output, "report.json"),
  JSON.stringify({ reports, output }, null, 2),
)
console.log(JSON.stringify({ status: "passed", reports, output }))
