import assert from "node:assert/strict"
import { jobContextFixture } from "./job-context-fixtures.mjs"
import { withIngestionLedger } from "./ingestion-fixtures.mjs"

// Extends the real Apply browser workflow; all network responses are controlled.
export async function enrichmentScenarios({
  evaluate,
  until,
  pause,
  clickApply,
  fill,
  generate,
  respond,
  repo,
  locale,
  mode = "success",
  materials,
}) {
  await evaluate(
    `localStorage.removeItem('careeros_profile_v2');localStorage.setItem('careeros_repo',${JSON.stringify(JSON.stringify(repo))});location.reload()`,
  )
  await until(
    "document.readyState==='complete' && !!document.querySelector('header button')",
  )
  await clickApply()
  await evaluate(
    "window.__pending=[];window.fetch=(url,options)=>new Promise((resolve,reject)=>window.__pending.push({url,options,resolve,reject}))",
  )
  await fill("AWS required.")
  await generate()
  await respond(
    {
      gaps: [
        {
          kind: "skill",
          requirement: "AWS",
          details: "Give a project example.",
        },
      ],
    },
    200,
    true,
  )
  await until("!!document.querySelector('#gap-note-0')")
  const answer =
    locale === "en"
      ? "I used AWS Lambda and S3 in Service Atlas."
      : "Usei AWS Lambda e S3 no Service Atlas."
  await evaluate(
    `(()=>{const el=document.querySelector('#gap-note-0');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,${JSON.stringify(answer)});el.dispatchEvent(new Event('input',{bubbles:true}))})()`,
  )
  await pause(60)
  const original = await evaluate("localStorage.getItem('careeros_profile_v2')")
  assert.equal(
    await evaluate("window.__pending.length"),
    0,
    "Typing/unchecked answer must not trigger ingestion",
  )
  await evaluate(
    "Array.from(document.querySelectorAll('[role=dialog] button')).at(-2).click()",
  )
  await until("window.__pending.length===1")
  assert.equal(
    await evaluate("window.__pending[0].url"),
    "/api/application-draft",
  )
  // A failed Draft must not prevent the independently consented enrichment.
  if (mode === "success-draft") {
    await respond(materials)
    await until("!!document.querySelector('.results-content')")
  } else await respond({ error: "outage" }, 503)
  await until("!!document.querySelector('[data-apply-enrichment]')")
  if (await evaluate("!!document.querySelector('[role=dialog]')"))
    await evaluate("document.querySelector('[role=dialog] button').click()")
  assert.equal(
    await evaluate("localStorage.getItem('careeros_profile_v2')"),
    original,
  )
  const artifactText =
    "(()=>{const el=document.querySelector('.results-content')?.cloneNode(true);if(!el)return null;el.querySelectorAll('[role=alert]').forEach(node=>node.remove());return el.textContent})()"
  const originalArtifact = await evaluate(artifactText)
  if (mode === "decline") {
    await evaluate(
      "document.querySelector('[data-enrichment-dismiss]').click()",
    )
    assert.equal(await evaluate("window.__pending.length"), 0)
    assert.equal(
      await evaluate("localStorage.getItem('careeros_profile_v2')"),
      original,
    )
    return
  }
  await evaluate("document.querySelector('[data-enrichment-review]').click()")
  await until("window.__pending.length===1")
  const request = await evaluate("JSON.parse(window.__pending[0].options.body)")
  assert.equal(request.input, answer)
  assert.equal(request.applyAnswer.language, locale)
  assert.equal(request.jobPosting, undefined)
  assert.equal(request.requirement, undefined)
  const fixture = withIngestionLedger({
    decision: {
      version: 1,
      field: "professional_information",
      outcome: { kind: "accept" },
    },
    claims: [
      {
        id: "c1",
        source: answer,
        text: answer,
        targets: ["projects"],
        question: "",
      },
    ],
    operations: [
      {
        claimId: "c1",
        target: "projects",
        entryId: "project",
        field: "technologies",
        action: "update",
        value: "Figma, AWS Lambda, S3",
        finding: "in_place",
      },
    ],
    unverifiedClaimCount: 0,
    unresolvedClaimIds: [],
    unplacedOperationCount: 0,
    applyAnswer: request.applyAnswer,
  })
  if (mode === "quota" || mode === "cross-tab") {
    await respond(fixture)
    await until("!!document.querySelector('[data-enrichment-save]')")
    await evaluate(
      "document.querySelector('[data-enrichment-operation]').click()",
    )
    let expected = original
    if (mode === "quota") {
      await evaluate(
        "window.__storageSet=Storage.prototype.setItem;Storage.prototype.setItem=function(key,value){if(key==='careeros_profile_v2')throw new DOMException('Synthetic quota','QuotaExceededError');return window.__storageSet.call(this,key,value)}",
      )
      await evaluate("document.querySelector('[data-enrichment-save]').click()")
      await until(
        "document.querySelector('[data-apply-enrichment] [role=alert]') !== null",
      )
      await evaluate("Storage.prototype.setItem=window.__storageSet")
    } else {
      expected = await evaluate(
        `(async()=>{const {editProfile}=await import('/src/lib/profileDocument.ts');const {saveProfile}=await import('/src/lib/profileStorage.ts');const before=JSON.parse(localStorage.getItem('careeros_profile_v2'));const next=editProfile(before,{type:'field',ownerId:before.id,field:'additionalInfo',value:'Unrelated later edit'});await saveProfile(localStorage,before,next);const raw=localStorage.getItem('careeros_profile_v2');window.dispatchEvent(new StorageEvent('storage',{key:'careeros_profile_v2',oldValue:JSON.stringify(before),newValue:raw}));return raw})()`,
      )
      await until("document.querySelector('[data-enrichment-save]').disabled")
    }
    assert.equal(
      await evaluate("!!document.querySelector('[data-enrichment-saved]')"),
      false,
    )
    assert.equal(
      await evaluate(
        "document.querySelector('[data-enrichment-answer]').value",
      ),
      answer,
    )
    assert.equal(
      await evaluate("localStorage.getItem('careeros_profile_v2')"),
      expected,
    )
    assert.equal(
      await evaluate("window.__pending.length"),
      0,
      "No automatic provider retry on storage failure/stale update",
    )
    return
  }
  // Cancel before the response; an obsolete provider result cannot restore review.
  await evaluate("document.querySelector('[data-enrichment-cancel]').click()")
  await respond(fixture)
  assert.equal(
    await evaluate("!!document.querySelector('[data-enrichment-save]')"),
    false,
  )
  assert.equal(
    await evaluate("localStorage.getItem('careeros_profile_v2')"),
    original,
  )
  await evaluate("document.querySelector('[data-enrichment-review]').click()")
  await until("window.__pending.length===1")
  await respond({ error: "outage" }, 503)
  assert.equal(
    await evaluate("window.__pending.length"),
    0,
    "Failed review does not retry automatically",
  )
  assert.equal(
    await evaluate("document.querySelector('[data-enrichment-answer]').value"),
    answer,
  )
  await evaluate("document.querySelector('[data-enrichment-review]').click()")
  await until("window.__pending.length===1")
  fixture.applyAnswer = await evaluate(
    "JSON.parse(window.__pending[0].options.body).applyAnswer",
  )
  await respond(fixture)
  await until("!!document.querySelector('[data-enrichment-save]')")
  assert.equal(
    await evaluate("document.querySelector('[data-enrichment-save]').disabled"),
    true,
    "No preselected operations",
  )
  assert.equal(
    await evaluate("localStorage.getItem('careeros_profile_v2')"),
    original,
  )
  await evaluate(
    "document.querySelector('[data-enrichment-operation]').click()",
  )
  await evaluate(
    "document.querySelector('[data-enrichment-save]').click();document.querySelector('[data-enrichment-save]').click()",
  )
  await until("!!document.querySelector('[data-enrichment-saved]')")
  const saved = await evaluate(
    "JSON.parse(localStorage.getItem('careeros_profile_v2'))",
  )
  assert.equal(
    saved.revision,
    JSON.parse(original).revision + 1,
    "One commit despite repeated Save",
  )
  assert.ok(
    saved.evidence.some(
      (e) => e.excerpt === answer && e.origin === "apply_answer",
    ),
  )
  assert.equal(
    await evaluate("window.__pending.length"),
    0,
    "Save uses no provider",
  )
  assert.ok(
    (await evaluate(
      "document.querySelector('[data-enrichment-answer]').value",
    )) === answer,
    "Recovery answer survives analysis reset",
  )
  if (mode === "success-draft") {
    assert.equal(
      await evaluate(artifactText),
      originalArtifact,
      "Saved enrichment cannot replace the original artifact snapshot",
    )
    return
  }
  // Saving and later retrieval/matching are separate. A deliberate fresh attempt
  // carries the approved fact and excerpt; this controlled matcher supports only
  // basic AWS use, with no invented certification/advanced-service assertion.
  await generate()
  await until("window.__pending.length===1")
  assert.equal(
    await evaluate("window.__pending[0].url"),
    "/api/qualification-gaps",
  )
  const later = await evaluate("JSON.parse(window.__pending[0].options.body)")
  const fact = later.profileEvidence.facts.find(
    (f) => f.field === "technologies" && f.value.includes("Lambda"),
  )
  assert.ok(fact, "Saved project fact enters later qualification projection")
  assert.ok(
    later.profileEvidence.evidence.some(
      (e) => e.excerpt === answer && e.origin === "apply_answer",
    ),
  )
  const context = jobContextFixture(later.jobPosting)
  const facts = later.profileEvidence.facts.map((f) => ({
    ...f,
    section:
      later.profileEvidence.entities.find((e) => e.id === f.owner.id)?.kind ??
      "profile",
    evidence: later.profileEvidence.links
      .filter(
        (l) =>
          l.kind === "supports" && l.from.id === f.id && l.state === "active",
      )
      .flatMap((l) =>
        later.profileEvidence.evidence
          .filter((e) => e.id === l.to.id)
          .map(({ id, revision, excerpt }) => ({ id, revision, excerpt })),
      ),
  }))
  await respond(
    {
      gaps: [],
      jobContext: context,
      matches: [
        {
          requirementIndex: 0,
          requirement: context.job.qualifications[0],
          state: "supported",
          factIds: [fact.id],
          explanation: "Controlled basic AWS usage support only.",
          question: "",
          complete: true,
          excluded: 0,
          facts,
        },
      ],
    },
    200,
    true,
  )
  await until("!!document.querySelector('[data-requirement-state=supported]')")
  assert.equal(
    await evaluate("!!document.querySelector('#gap-note-0')"),
    false,
    "Controlled supported basic use does not ask again",
  )
  // This fixture is not evidence of semantic provider recall or quality.
  await evaluate("document.querySelector('[role=dialog] button').click()")
  assert.equal(
    await evaluate("document.documentElement.scrollWidth<=window.innerWidth"),
    true,
    "Enrichment fits the viewport",
  )
}
