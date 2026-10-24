import assert from "node:assert/strict"
import { writeFileSync } from "node:fs"
import { join } from "node:path"
import { savedProfileExpression } from "./profile-browser-storage.mjs"

// Extends the existing Apply browser harness; all data and responses are synthetic.
export async function focusedJourneyScenarios({
  evaluate,
  until,
  call,
  repo,
  locale,
  fill,
  generate,
  respond,
  work,
  width,
}) {
  const screenshot = async (name) => {
    await new Promise((resolve) => setTimeout(resolve, 200))
    const result = await call("Page.captureScreenshot", {
      format: "png",
      captureBeyondViewport: true,
    })
    writeFileSync(
      join(work, `focused-${name}-${locale}-${width}.png`),
      Buffer.from(result.data, "base64"),
    )
  }
  const nav = "document.querySelectorAll('header nav button')"
  const guidance = "document.querySelector('.journey-guidance')"
  const submit =
    "document.querySelector('[data-checklist-state]').parentElement.parentElement.querySelector('button')"
  const reset = async (source = repo) => {
    await evaluate(
      `localStorage.setItem('careeros_locale', ${JSON.stringify(locale)}); localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(source))}); localStorage.setItem('careeros_apikey', 'synthetic-test-key'); localStorage.setItem('careeros_typesafe_key', 'synthetic-typesafe'); localStorage.removeItem('careeros_curated_cv_v1'); location.reload()`,
    )
    await until(
      "document.readyState === 'complete' && !!document.querySelector('header button')",
    )
    await evaluate(
      "window.__pending = []; window.fetch = (url, options) => new Promise(resolve => window.__pending.push({ url, options, resolve }))",
    )
  }
  const enter = async (selector = "header button:last-child") => {
    await evaluate(
      `document.querySelector(${JSON.stringify(selector)}).click()`,
    )
    await until(`${nav}.length === 4`)
    assert.equal(
      await evaluate(
        "document.activeElement === document.querySelector('main')",
      ),
      true,
    )
    assert.equal(
      await evaluate("document.documentElement.scrollWidth <= innerWidth"),
      true,
    )
  }
  const navigate = async (index) => {
    await evaluate(`${nav}[${index}].click()`)
    await until(`${nav}[${index}].getAttribute('aria-current') === 'page'`)
    // Font/layout anchoring can shift scroll by a few pixels after a save.
    // The customer-facing contract is visible guidance below the sticky header.
    await until(`${guidance}.querySelector('p').getBoundingClientRect().top >= document.querySelector('header').getBoundingClientRect().bottom`)
    assert.equal(
      await evaluate(
        `${guidance}.querySelector('p').getBoundingClientRect().top >= document.querySelector('header').getBoundingClientRect().bottom`,
      ),
      true,
    )
  }
  const contact = async (value) => {
    await evaluate(
      "document.querySelector('#contact-fullName').focus(); document.querySelector('#contact-fullName').select()",
    )
    await call("Input.insertText", { text: value })
  }
  const empty = Object.fromEntries(
    Object.entries(repo).map(([key, value]) => [
      key,
      Array.isArray(value) ? [] : "",
    ]),
  )
  const readyText =
    locale === "en"
      ? "Professional information saved in this browser."
      : "Informações profissionais salvas neste navegador."
  const savingText =
    locale === "en"
      ? "Saving Profile in this browser…"
      : "Salvando Perfil neste navegador…"

  // Every landing action routes empty and contact-only Profiles to Profile.
  for (const source of [
    empty,
    { ...empty, fullName: "Synthetic person", email: "synthetic@example.com" },
  ])
    for (const selector of [
      "header button:last-child",
      "#top button",
      "#top section:last-child button",
    ]) {
      await reset(source)
      await enter(selector)
      assert.equal(
        await evaluate(`${nav}[1].getAttribute('aria-current')`),
        "page",
      )
      assert.equal(
        await evaluate(`!!${guidance}.querySelector('button')`),
        false,
      )
      assert.equal(
        await evaluate("localStorage.getItem('careeros_curated_cv_v1')"),
        null,
      )
      await navigate(0)
      await fill("Java developer AWS required.")
      assert.equal(await evaluate(`${submit}.disabled`), true)
      assert.equal(await evaluate("window.__pending.length"), 0)
    }
  for (const selector of [
    "header button:last-child",
    "#top button",
    "#top section:last-child button",
  ]) {
    await reset()
    await enter(selector)
    assert.equal(
      await evaluate(`${nav}[0].getAttribute('aria-current')`),
      "page",
    )
    assert.equal(
      await evaluate("localStorage.getItem('careeros_curated_cv_v1')"),
      null,
    )
  }

  // The next-step control is reachable by keyboard, and navigation moves focus.
  await navigate(1)
  await until(`${guidance}.textContent.includes(${JSON.stringify(readyText)})`)
  const colors = await evaluate(`[
    [getComputedStyle(document.querySelector('aside nav button')).color, getComputedStyle(document.querySelector('aside nav button')).backgroundColor],
    [getComputedStyle(${guidance}.querySelector('p:nth-child(2)')).color, getComputedStyle(document.body).backgroundColor]
  ]`)
  const luminance = (color) =>
    color
      .match(/[0-9.]+/g)
      .slice(0, 3)
      .map(Number)
      .map((value) => value / 255)
      .map((value) =>
        value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4,
      )
      .reduce(
        (sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index],
        0,
      )
  for (const [foreground, background] of colors) {
    const values = [luminance(foreground), luminance(background)]
    assert.ok(
      (Math.max(...values) + 0.05) / (Math.min(...values) + 0.05) >= 4.5,
    )
  }
  await screenshot("profile")
  await evaluate("document.querySelector('main').focus()")
  await call("Input.dispatchKeyEvent", {
    type: "keyDown",
    key: "Tab",
    code: "Tab",
    windowsVirtualKeyCode: 9,
  })
  await call("Input.dispatchKeyEvent", {
    type: "keyUp",
    key: "Tab",
    code: "Tab",
    windowsVirtualKeyCode: 9,
  })
  assert.equal(
    await evaluate(
      `document.activeElement === ${guidance}.querySelector('button')`,
    ),
    true,
  )
  assert.equal(
    await evaluate("getComputedStyle(document.activeElement).outlineStyle"),
    "solid",
  )
  await call("Input.dispatchKeyEvent", {
    type: "keyDown",
    key: "Enter",
    code: "Enter",
    windowsVirtualKeyCode: 13,
    text: "\r",
  })
  await call("Input.dispatchKeyEvent", {
    type: "keyUp",
    key: "Enter",
    code: "Enter",
    windowsVirtualKeyCode: 13,
  })
  await until(`${nav}[0].getAttribute('aria-current') === 'page'`)
  assert.equal(
    await evaluate("document.activeElement === document.querySelector('main')"),
    true,
  )
  // Standalone CV and an explicitly labeled key field stay available.
  await navigate(2)
  assert.ok(
    await evaluate(
      `${guidance}.textContent.includes(${JSON.stringify(locale === "en" ? "standalone CV is optional" : "CV geral é opcional")})`,
    ),
  )
  await evaluate(
    "document.querySelector('header .flex-shrink-0 button:last-child').click()",
  )
  await until("!!document.querySelector('#openai-key')")
  assert.equal(
    await evaluate("document.querySelector('#openai-key').labels.length"),
    1,
  )

  // A delayed save is not readiness; release restores readiness without inference.
  await reset()
  await enter()
  await navigate(1)
  const before = await evaluate("localStorage.getItem('careeros_profile_v2')")
  await evaluate(
    "window.__held = false; window.__hold = navigator.locks.request('careeros_profile_v2', async () => { window.__held = true; await new Promise(resolve => window.__release = resolve) }); undefined",
  )
  await until("window.__held")
  await contact("Synthetic pending name")
  await until(`${guidance}.textContent.includes(${JSON.stringify(savingText)})`)
  assert.equal(
    await evaluate("localStorage.getItem('careeros_profile_v2')"),
    before,
  )
  assert.equal(await evaluate(`!!${guidance}.querySelector('button')`), false)
  await navigate(0)
  await fill("Java developer AWS required.")
  assert.equal(await evaluate(`${submit}.disabled`), true)
  assert.equal(
    await evaluate(
      "document.querySelectorAll('[data-checklist-state]')[1].dataset.checklistState",
    ),
    "pending",
  )
  await evaluate("window.__release()")
  await until(`!${submit}.disabled`)
  assert.equal(
    JSON.parse(await evaluate(savedProfileExpression)).fullName,
    "Synthetic pending name",
  )
  assert.equal(await evaluate("window.__pending.length"), 0)

  // Quota failure keeps authoritative bytes, recovery, and no generation.
  await navigate(1)
  const saved = await evaluate("localStorage.getItem('careeros_profile_v2')")
  await evaluate(
    "window.__set = Storage.prototype.setItem; Storage.prototype.setItem = function(key, value) { if (key === 'careeros_profile_v2') throw new DOMException('Synthetic quota failure', 'QuotaExceededError'); return window.__set.call(this, key, value) }",
  )
  await contact("Synthetic unsaved name")
  await until("!!document.querySelector('[data-profile-refresh]')")
  assert.equal(
    await evaluate("localStorage.getItem('careeros_profile_v2')"),
    saved,
  )
  assert.equal(await evaluate(`!!${guidance}.querySelector('button')`), false)
  await navigate(0)
  assert.equal(await evaluate(`${submit}.disabled`), true)
  assert.equal(
    await evaluate(
      "document.querySelectorAll('[data-checklist-state]')[1].dataset.checklistState",
    ),
    "failed",
  )
  assert.equal(await evaluate("window.__pending.length"), 0)
  await screenshot("save-failed")
  await evaluate(
    "Storage.prototype.setItem = window.__set; document.querySelector('[data-profile-refresh]').click()",
  )
  await until("!document.querySelector('[data-profile-refresh]')")
  assert.equal(await evaluate(`${submit}.disabled`), false)
  assert.equal(
    JSON.parse(await evaluate(savedProfileExpression)).fullName,
    "Synthetic pending name",
  )

  // Lost lock acknowledgment after commit is recovered from persisted authority.
  await navigate(1)
  await evaluate(
    "window.__lock = navigator.locks.request.bind(navigator.locks); navigator.locks.request = async (...args) => { await window.__lock(...args); throw new Error('Synthetic lost acknowledgment') }",
  )
  await contact("Synthetic acknowledged from storage")
  await until(
    `(async () => JSON.parse(await ${savedProfileExpression}).fullName === 'Synthetic acknowledged from storage')()`,
  )
  await until(`${guidance}.textContent.includes(${JSON.stringify(readyText)})`)
  assert.equal(
    JSON.parse(await evaluate(savedProfileExpression)).fullName,
    "Synthetic acknowledged from storage",
  )
  assert.equal(
    await evaluate("!!document.querySelector('[data-profile-refresh]')"),
    false,
  )
  await evaluate("navigator.locks.request = window.__lock")

  // A stale storage notification while analysis awaits confirmation blocks both
  // generation actions, preserving posting/notes and making no second API call.
  await navigate(0)
  await generate()
  await until("window.__pending.length === 1")
  await respond({ gaps: [] }, 200, true)
  await until("!!document.querySelector('[role=dialog]')")
  await evaluate(
    "window.dispatchEvent(new StorageEvent('storage', { key: 'careeros_profile_v2', newValue: 'changed in another tab' }))",
  )
  await until("!!document.querySelector('[data-profile-refresh]')")
  assert.equal(
    await evaluate(
      "Array.from(document.querySelectorAll('[role=dialog] button')).slice(-2).every(button => button.disabled)",
    ),
    true,
  )
  assert.equal(
    await evaluate("document.querySelector('main textarea').value"),
    "Java developer AWS required.",
  )
  assert.equal(await evaluate("window.__pending.length"), 0)
}
