// Controlled browser evidence for the compact destination review presenter
// (#190). Synthetic plans only: no provider call, storage write or canonical
// Profile mutation is exercised here.
import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"
import { createBrowserEvaluation } from "./browser-evaluation.mjs"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-review-browser-"))
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const port = await new Promise((resolve) => {
  const server = createServer()
  server.listen(0, "127.0.0.1", () => {
    const value = server.address().port
    server.close(() => resolve(value))
  })
})
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
    } catch {}
    if (debugPort) break
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
    for (let attempt = 0; attempt < 100; attempt++) {
      if (await evaluate(expression)) return
      await pause(100)
    }
    throw new Error(
      `Timed out: ${expression}; ${await evaluate(
        "JSON.stringify({text:document.body.innerText.slice(0,2000), state:window.__reviewState})",
      )}`,
    )
  }
  const navigate = async (scenario, locale = "en") => {
    await call("Page.navigate", {
      url: `http://127.0.0.1:${port}/tests/review-harness.html?scenario=${scenario}&locale=${locale}`,
    })
    await until(
      `location.search.includes("scenario=${scenario}") && !!window.__reviewState && !!document.querySelector('[data-harness-scenario="${scenario}"]')`,
    )
    await pause(80)
  }
  const state = () => evaluate("window.__reviewState")
  const click = (selector) =>
    evaluate(
      `(() => { const el = document.querySelector(${JSON.stringify(
        selector,
      )}); if (!el) throw new Error(${JSON.stringify(
        `Missing ${selector}`,
      )}); el.click(); return true })()`,
    )
  const clickButton = (text) =>
    evaluate(
      `(() => { const el = Array.from(document.querySelectorAll('button')).find(el => el.textContent.trim() === ${JSON.stringify(
        text,
      )}); if (!el) throw new Error("Missing button ${text}"); el.click(); return true })()`,
    )
  const normalized = (selector) =>
    evaluate(
      `(() => { const el = document.querySelector(${JSON.stringify(
        selector,
      )}); return el ? el.textContent.replace(/\\s+/g, " ").trim() : null })()`,
    )
  const key = async (key, code, windowsVirtualKeyCode, text) => {
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
  }

  await call("Page.enable")
  await navigate("mixed")

  // Default state: no preselection, destination groups, explicit attention and
  // duplicate separation, proposed values before current values.
  let current = await state()
  const storageBefore = await evaluate(
    "localStorage.getItem('careeros_profile_v2')",
  )
  assert.equal(current.summary.selectable, 6)
  assert.equal(current.summary.selected, 0)
  assert.equal(current.summary.attention, 4)
  assert.equal(current.summary.conflicts, 1)
  assert.equal(current.summary.clarifications, 1)
  assert.equal(current.summary.unsupported, 1)
  assert.equal(current.summary.unresolved, 1)
  assert.equal(current.summary.duplicates, 1)
  assert.equal(current.summary.noChange, false)
  assert.equal(
    await evaluate(
      "Array.from(document.querySelectorAll('input[type=checkbox]')).every(el => !el.checked && !el.indeterminate)",
    ),
    true,
  )
  assert.match(await normalized("section h2"), /^Review changes/)
  assert.match(
    await normalized('[data-review-unit][data-review-kind="replacement"]'),
    /Led a global platform\.\s*Current: Led a platform\./,
  )
  assert.equal(
    await evaluate(
      `Array.from(document.querySelectorAll('[data-review-unit][data-review-kind="replacement"] p')).some(el => el.className.includes('line-through') && el.textContent.includes('Led a platform.'))`,
    ),
    true,
  )
  assert.match(
    await normalized('[data-review-unit][data-review-kind="removal"]'),
    /Remove: Kept note/,
  )
  const evidence = await normalized(
    '[data-review-unit][data-review-kind="evidence"]',
  )
  assert.match(evidence, /Attach source · Skills/)
  assert.match(evidence, /wording is unchanged/)
  assert.equal(
    await evaluate(
      `!document.querySelector('[data-review-unit][data-review-kind="evidence"] button')`,
    ),
    true,
  )
  assert.equal(
    await evaluate(
      `Array.from(document.querySelectorAll('summary')).some(el => el.textContent.includes('Already in your Profile (1)') && !el.closest('details').open)`,
    ),
    true,
  )
  assert.equal(
    await evaluate(
      `document.body.innerText.includes('Harbor Works · Product Lead · 2021 · 2024')`,
    ),
    true,
  )
  assert.equal(
    await evaluate(
      `document.body.innerText.split('Harbor Works · Product Lead · 2021 · 2024').length - 1`,
    ),
    1,
  )

  // One deduplicated source disclosure, preserving both origins and the deeper
  // reference context.
  const sourceOpened = await evaluate(
    `(() => {
      const row = Array.from(document.querySelectorAll('[data-review-unit][data-review-kind="addition"]')).find(el => el.textContent.includes('I use TypeScript'));
      if (!row) return false;
      const details = row.querySelector('details[data-review-source]');
      if (!details) return false;
      details.open = true;
      return true;
    })()`,
  )
  assert.equal(sourceOpened, true)
  const sourceText = await evaluate(
    `(() => {
      const row = Array.from(document.querySelectorAll('[data-review-unit][data-review-kind="addition"]')).find(el => el.textContent.includes('I use TypeScript'));
      return row.querySelector('details[data-review-source]').textContent.replace(/\\s+/g, ' ').trim();
    })()`,
  )
  assert.equal(sourceText.match(/Original paste/g)?.length, 2)
  assert.equal(sourceText.match(/I use TypeScript/g)?.length, 1)
  assert.equal(sourceText.match(/At Acme I used TypeScript\./g)?.length, 1)
  assert.equal(sourceText.match(/Clarification answer/g)?.length, 1)
  assert.equal(sourceText.match(/I used it in production\./g)?.length, 1)
  const detailsOpened = await evaluate(
    `(() => {
      const row = Array.from(document.querySelectorAll('[data-review-unit][data-review-kind="addition"]')).find(el => el.textContent.includes('I use TypeScript'));
      const details = row.querySelectorAll('details[data-review-source] details');
      const nested = Array.from(details).find(el => el.querySelector('summary')?.textContent.includes('Details'));
      if (!nested) return false;
      nested.open = true;
      return true;
    })()`,
  )
  assert.equal(detailsOpened, true)
  const deepText = await normalized(
    '[data-review-unit][data-review-kind="addition"] details[data-review-source]',
  )
  assert.match(deepText, /Statement ID: run\/0\/c1/)
  assert.match(deepText, /Occurrence: segment-1 · 0–18/)

  // Group, individual and scoped bulk selection.
  await click('input[aria-label^="Select all in Skills"]')
  current = await state()
  assert.equal(current.summary.selected, 2)
  assert.equal(current.preview.selectedOperationCount, 2)
  assert.equal(
    await evaluate(
      `document.querySelector('input[aria-label^="Select all in Skills"]').checked`,
    ),
    true,
  )
  await click('input[aria-label^="Select all in Skills"]')
  current = await state()
  assert.equal(current.summary.selected, 0)

  await click(
    '[data-review-unit][data-review-kind="replacement"] input[type=checkbox]',
  )
  current = await state()
  assert.equal(current.summary.selected, 1)
  assert.equal(
    await evaluate(
      `document.querySelector('input[aria-label^="Select all in Skills"]').indeterminate`,
    ),
    false,
  )
  await clickButton("Clear selection")
  current = await state()
  assert.equal(current.summary.selected, 0)

  await clickButton("Select clear additions")
  current = await state()
  assert.equal(current.summary.selected, 3)
  assert.equal(current.preview.selectedOperationCount, 6)
  const statuses = Object.fromEntries(
    current.units.map((unit) => [unit.kind, unit.status]),
  )
  assert.equal(statuses.replacement, "pending")
  assert.equal(statuses.removal, "pending")
  assert.equal(statuses.evidence, "pending")
  assert.equal(statuses.duplicate, "pending")
  assert.equal(statuses.conflict, "pending")
  assert.equal(
    current.preview.fields.some((field) => field.action === "remove"),
    false,
  )
  assert.equal(
    current.preview.fields.some((field) => field.action === "evidence"),
    false,
  )
  await clickButton("Clear selection")

  // Partial group state is exposed to assistive technology.
  await click(
    '[data-review-unit][data-review-kind="addition"] input[type=checkbox]',
  )
  assert.equal(
    await evaluate(
      `document.querySelector('input[aria-label^="Select all in Skills"]').indeterminate`,
    ),
    true,
  )
  await clickButton("Clear selection")

  // Focused edit previews the exact current/result pair, preserves selection,
  // records manual wording and can be reverted to the machine proposal.
  await click(
    '[data-review-unit][data-review-kind="replacement"] input[type=checkbox]',
  )
  await click('[data-review-unit][data-review-kind="replacement"] button')
  await until(
    `!!document.querySelector('[data-review-unit][data-review-kind="replacement"] textarea')`,
  )
  await evaluate(
    `(() => {
      const el = document.querySelector('[data-review-unit][data-review-kind="replacement"] textarea');
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, 'Led a worldwide platform.');
      el.dispatchEvent(new Event('input', { bubbles: true }));
    })()`,
  )
  await pause(40)
  assert.match(
    await normalized('[data-review-unit][data-review-kind="replacement"] form'),
    /Preview: Led a platform\. → Led a worldwide platform\./,
  )
  await clickButton("Save wording")
  await pause(40)
  current = await state()
  const edited = current.operations.find(
    (operation) => operation.claimId === "run/0/c3",
  )
  assert.equal(edited.value, "Led a worldwide platform.")
  assert.equal(edited.proposedValue, "Led a global platform.")
  assert.equal(
    current.units.find((unit) => unit.kind === "replacement").status,
    "selected",
  )
  assert.match(
    await normalized('[data-review-unit][data-review-kind="replacement"]'),
    /Your wording/,
  )
  assert.equal(
    current.preview.fields.find((field) => field.field === "description").after,
    "Led a worldwide platform.",
  )

  await click('[data-review-unit][data-review-kind="replacement"] button')
  await until(
    `!!document.querySelector('[data-review-unit][data-review-kind="replacement"] textarea')`,
  )
  await evaluate(
    `(() => {
      const el = document.querySelector('[data-review-unit][data-review-kind="replacement"] textarea');
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, 'Led a global platform.');
      el.dispatchEvent(new Event('input', { bubbles: true }));
    })()`,
  )
  await clickButton("Save wording")
  await pause(40)
  assert.equal(
    await evaluate(
      `document.querySelector('[data-review-unit][data-review-kind="replacement"]').textContent.includes('Your wording')`,
    ),
    false,
  )
  await clickButton("Clear selection")

  // Baseline keyboard semantics: Space toggles the focused checkbox, Enter opens
  // the editor, and cancelling returns focus to the edit trigger.
  await evaluate(
    `document.querySelector('[data-review-unit][data-review-kind="replacement"] input[type=checkbox]').focus()`,
  )
  await key(" ", "Space", 32, " ")
  await pause(40)
  current = await state()
  assert.equal(current.summary.selected, 1)
  await key(" ", "Space", 32, " ")
  await pause(40)
  current = await state()
  assert.equal(current.summary.selected, 0)

  await evaluate(
    `document.querySelector('[data-review-unit][data-review-kind="replacement"] button').focus()`,
  )
  await key("Enter", "Enter", 13, "\r")
  await until(
    `!!document.querySelector('[data-review-unit][data-review-kind="replacement"] textarea')`,
  )
  await evaluate(
    `Array.from(document.querySelectorAll('[data-review-unit][data-review-kind="replacement"] button')).find(el => el.textContent.trim() === 'Cancel').focus()`,
  )
  await key("Enter", "Enter", 13, "\r")
  await pause(40)
  assert.equal(
    await evaluate(
      `document.activeElement === document.querySelector('[data-review-unit][data-review-kind="replacement"] button')`,
    ),
    true,
  )
  assert.equal(
    await evaluate("localStorage.getItem('careeros_profile_v2')"),
    storageBefore,
    "review interactions never write the canonical Profile",
  )

  // All-duplicate completion state: no approval work, collapsed rollup.
  await navigate("duplicates")
  current = await state()
  assert.equal(current.summary.noChange, true)
  assert.equal(current.summary.selectable, 0)
  assert.equal(current.summary.duplicates, 1)
  assert.match(
    await normalized("section"),
    /Everything you pasted is already in your Profile/,
  )
  assert.match(await normalized("section"), /Already in your Profile \(1\)/)
  assert.equal(
    await evaluate(
      "document.querySelectorAll('section input, section button').length",
    ),
    0,
  )
  assert.equal(
    await evaluate(
      `Array.from(document.querySelectorAll('section details')).every(el => !el.open)`,
    ),
    true,
  )

  // Empty Profile: clear additions and a coherent new entry.
  await navigate("empty")
  current = await state()
  assert.equal(current.summary.selectable, 2)
  assert.equal(current.summary.duplicates, 0)
  assert.match(await normalized("section"), /New entry · Experience/)
  assert.match(
    await normalized("section"),
    /Nova Labs · Senior Engineer · 2025/,
  )
  await clickButton("Select clear additions")
  current = await state()
  assert.equal(current.summary.selected, 2)
  assert.equal(current.preview.selectedOperationCount, 4)
  assert.equal(current.preview.complete, true)

  // Focused clarification: the collapsed row names the destination, question,
  // competing statement and existing value; expansion adds the question, source
  // excerpt and explicit answer/leave-out actions. A rejected answer must change
  // before retry; a successful revision preserves unrelated selection and edit.
  await navigate("clarification")
  current = await state()
  assert.equal(current.summary.attention, 1)
  assert.equal(current.summary.clarifications, 1)
  const attentionSelector = '[data-review-unit][data-review-kind="clarification"]'
  const collapsed = await normalized(attentionSelector)
  assert.match(collapsed, /Experience/)
  assert.match(collapsed, /Was this full-time or contract\?/)
  assert.match(collapsed, /Competing statement/)
  assert.match(collapsed, /I use Svelte\./)
  assert.match(collapsed, /In your Profile/)
  assert.match(collapsed, /React/)
  assert.match(collapsed, /Why approval is blocked/)
  assert.equal(
    await evaluate(
      `document.querySelector('${attentionSelector} details[data-review-attention]').open`,
    ),
    false,
  )

  // Unrelated reviewed work: select Svelte and edit the Vue addition.
  const additionRow = (text) =>
    `Array.from(document.querySelectorAll('[data-review-unit][data-review-kind="addition"]')).find(el => el.textContent.includes(${JSON.stringify(text)}))`
  await evaluate(`${additionRow("Svelte")}.querySelector('input[type=checkbox]').click()`)
  await evaluate(`${additionRow("Vue")}.querySelector('button').click()`)
  await until(
    `${additionRow("Vue")}.querySelector('textarea') !== null`,
  )
  await evaluate(
    `(() => {
      const el = ${additionRow("Vue")}.querySelector('textarea');
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, 'Vue 3');
      el.dispatchEvent(new Event('input', { bubbles: true }));
    })()`,
  )
  await clickButton("Save wording")
  await pause(40)
  current = await state()
  assert.equal(
    current.operations.find((operation) => operation.claimId === "run/0/c3").value,
    "Vue 3",
  )

  // Expand and inspect the focused question/excerpt, then open the answer form.
  await evaluate(
    `document.querySelector('${attentionSelector} details[data-review-attention]').open = true`,
  )
  await pause(40)
  assert.match(
    await normalized(`${attentionSelector} details[data-review-attention]`),
    /Question/,
  )
  assert.match(
    await normalized(`${attentionSelector} details[data-review-attention]`),
    /I worked at Acme\./,
  )
  await click("[data-review-answer-open]")
  await until("!!document.querySelector('[data-review-answer] textarea')")
  assert.match(
    await normalized("[data-review-answer]"),
    /0 \/ 1,000/,
  )
  // Cancel returns focus to the Answer trigger without closing the row.
  await evaluate(
    `Array.from(document.querySelectorAll('[data-review-answer] button')).find(el => el.textContent.trim() === 'Cancel').click()`,
  )
  await pause(40)
  assert.equal(
    await evaluate(
      "document.activeElement?.hasAttribute('data-review-answer-open')",
    ),
    true,
  )

  // Leave out keeps the item unresolved and does not hide it.
  await click("[data-review-leave-out]")
  await pause(40)
  assert.match(
    await normalized(attentionSelector),
    /Kept unresolved/,
  )
  assert.equal((await state()).leftOut.length, 1)
  assert.equal(
    await evaluate(
      "document.activeElement?.hasAttribute('data-review-answer-open')",
    ),
    true,
  )

  // Rejected wording blocks the same answer until it changes.
  await click("[data-review-answer-open]")
  await until("!!document.querySelector('[data-review-answer] textarea')")
  await evaluate(
    `(() => {
      const el = document.querySelector('[data-review-answer] textarea');
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, 'blocked answer');
      el.dispatchEvent(new Event('input', { bubbles: true }));
    })()`,
  )
  await clickButton("Review revised proposal")
  await until(
    `!!document.querySelector('[data-review-attention] p[role="alert"]')`,
  )
  assert.match(
    await normalized(attentionSelector),
    /Rewrite the professional description/,
  )
  assert.equal(
    await evaluate(
      `Array.from(document.querySelectorAll('[data-review-answer] button')).find(el => el.textContent.trim() === 'Review revised proposal').disabled`,
    ),
    true,
  )
  await evaluate(
    `(() => {
      const el = document.querySelector('[data-review-answer] textarea');
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, 'It was full-time.');
      el.dispatchEvent(new Event('input', { bubbles: true }));
    })()`,
  )
  await clickButton("Review revised proposal")
  await until(
    `!document.querySelector('${attentionSelector}')`,
  )
  await pause(60)
  current = await state()
  assert.equal(current.summary.clarifications, 0)
  assert.equal(current.summary.selected, 1)
  assert.equal(
    current.operations.find((operation) => operation.claimId === "run/0/c3").value,
    "Vue 3",
  )
  assert.equal(
    current.operations.find((operation) => operation.claimId === "run/0/c3").proposedValue,
    "Vue",
  )
  assert.equal(
    await evaluate(
      `(() => {
        const unit = Array.from(document.querySelectorAll('[data-review-unit]')).find((el) => el.textContent.includes('Contract'));
        return !!unit && document.activeElement === unit.querySelector('input[type=checkbox]');
      })()`,
    ),
    true,
    "resolving a clarification focuses the destination it now updates",
  )
  assert.equal(
    await evaluate("localStorage.getItem('careeros_profile_v2')"),
    storageBefore,
    "clarification never writes the canonical Profile",
  )

  // PT-BR clarification copy.
  await navigate("clarification", "pt-BR")
  assert.match(await normalized(attentionSelector), /Pergunta|Precisa de esclarecimento/)
  await evaluate(
    `document.querySelector('${attentionSelector} details[data-review-attention]').open = true`,
  )
  await pause(40)
  assert.equal(
    await evaluate(
      `Array.from(document.querySelectorAll('button')).some(el => el.textContent.trim() === 'Responder')`,
    ),
    true,
  )
  assert.equal(
    await evaluate(
      `Array.from(document.querySelectorAll('button')).some(el => el.textContent.trim() === 'Deixar de fora / manter em aberto')`,
    ),
    true,
  )

  // Concise PT-BR copy over the same controlled state.
  await navigate("mixed", "pt-BR")
  current = await state()
  assert.match(await normalized("section h2"), /^Revisar alterações/)
  assert.equal(
    await evaluate(
      `Array.from(document.querySelectorAll('button')).some(el => el.textContent.trim() === 'Selecionar adições claras')`,
    ),
    true,
  )
  assert.equal(
    await evaluate(
      `!!document.querySelector('input[aria-label^="Selecionar tudo em Habilidades"]')`,
    ),
    true,
  )
  assert.match(
    await normalized('[data-review-unit][data-review-kind="removal"]'),
    /Remover: Kept note/,
  )
  assert.match(
    await normalized('[data-review-unit][data-review-kind="replacement"]'),
    /Atual: Led a platform\./,
  )

  console.log(
    "PASS review renderer → no preselection → destination groups → deduplicated source disclosure/origins → group/individual/additions-only selection → duplicate/no-change and empty states → focused edit preview/provenance/revert → keyboard focus → focused clarification (collapsed attention, answer/leave-out, rejection/retry, unrelated-work retention) → EN/PT copy; zero provider calls or review-triggered canonical writes.",
  )
} finally {
  socket?.close()
  chrome.kill("SIGKILL")
  vite.kill("SIGKILL")
}
