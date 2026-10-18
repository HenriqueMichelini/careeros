import test from "node:test"
import assert from "node:assert/strict"
import { execFileSync } from "node:child_process"
import { mkdtempSync, readFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
import { repositoryDefaults } from "./cv-support-fixtures.mjs"

// ADR 0003 / issue #128: end-to-end controlled verification of the activated
// statement-support contract.
//
// This test joins two real production components and nothing else:
//   1. the Go CV-generation HTTP handler, driven through #34's keyless controlled
//      transport by `scripts/semantic-quality --mode controlled` over the
//      controlled EN/PT corpus `docs/evaluations/cv-support/cases.v1.json`; and
//   2. the production TypeScript client (`src/lib/cvGeneration.ts`) transpiled
//      in-process and exercised through its real `generateCv` request path.
//
// The scripted judgments are synthetic: this establishes controlled enforcement
// (an unsupported machine sentence is blocked; a supported one stays usable),
// never live assessor accuracy. No network, credentials, deployment or new paid
// provider behavior is involved.

const root = new URL("..", import.meta.url).pathname
const outputPath = join(
  mkdtempSync(join(tmpdir(), "cv-support-integration-")),
  "controlled.json",
)
execFileSync(
  "go",
  [
    "run",
    "./scripts/semantic-quality",
    "--mode",
    "controlled",
    "--fixtures",
    "docs/evaluations/cv-support/cases.v1.json",
    "--split",
    "all",
    "--output",
    outputPath,
  ],
  {
    cwd: root,
    env: { ...process.env, GOCACHE: "/tmp/careeros-go-cache" },
    stdio: "pipe",
  },
)
const report = JSON.parse(readFileSync(outputPath, "utf8"))
const corpus = JSON.parse(
  readFileSync(
    new URL("../docs/evaluations/cv-support/cases.v1.json", import.meta.url),
    "utf8",
  ),
)
// Expected statuses, call counts and per-unit judgments are declared by the
// fixture, independently of the handler that returns them. The runner ignores
// this test-only map; the integration test asserts the handler output against it
// before deciding whether acceptance should succeed.
const expectations = corpus.expectations

const clientPath = join(
  mkdtempSync(join(tmpdir(), "cv-support-client-")),
  "generation.mjs",
)
const { writeFileSync } = await import("node:fs")
writeFileSync(
  clientPath,
  ts.transpileModule(
    readFileSync(
      new URL("../src/lib/cvGeneration.ts", import.meta.url),
      "utf8",
    ),
    {
      compilerOptions: {
        module: ts.ModuleKind.ESNext,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText,
)
const {
  CV_SUPPORT_REQUEST,
  stableCvFacts,
  generateCv,
  validateCvSupport,
  cvSupportUnits,
  cvReviewUnits,
  cvReviewResolved,
  resolveCvReview,
  acceptCvResult,
  createCuratedCv,
  parseCuratedCv,
  cvWordingTrust,
  cvSupportStatus,
  emptyCvReview,
} = await import(clientPath)

// Build a fully valid synthetic canonical Profile from the corpus facts so the
// retained fact ids, ownership and citations travel through `stableCvFacts`
// unchanged. The client persists a v3 snapshot only from a real ProfileDocument.
function canonicalDocument(facts) {
  const docId = "synthetic-profile"
  const entities = []
  const ownerFor = (fact) => {
    if (!fact.entryId) return { profileId: docId, id: docId, revision: 1 }
    let entity = entities.find((e) => e.legacyId === fact.entryId)
    if (!entity) {
      entity = {
        id: `entity-${fact.entryId}`,
        revision: 1,
        kind: fact.section,
        legacyId: fact.entryId,
      }
      entities.push(entity)
    }
    return { profileId: docId, id: entity.id, revision: 1 }
  }
  return {
    id: docId,
    revision: 1,
    entities,
    evidence: [],
    links: [],
    facts: facts.map((fact, order) => ({
      id: fact.id,
      revision: 1,
      owner: ownerFor(fact),
      context: [],
      field: fact.field,
      order,
      kind: "legacy_block",
      value: fact.text,
      assertion: "unknown",
      intent: "unknown",
      certainty: "unknown",
      support: "unsupported",
    })),
  }
}

function repositoryFromFacts(facts) {
  const repo = {
    fullName: "",
    email: "",
    phone: "",
    location: "",
    professionalLinks: "",
    careerGoals: "",
    skills: "",
    competencies: "",
    tools: "",
    employmentStatus: "",
    currentSalary: "",
    desiredSalary: "",
    additionalInfo: "",
    experience: [],
    projects: [],
    education: [],
    certifications: [],
    languages: [],
  }
  for (const fact of facts) {
    if (!fact.entryId) {
      repo[fact.field] = fact.text
      continue
    }
    let entry = repo[fact.section].find((e) => e.id === fact.entryId)
    if (!entry) {
      entry = { id: fact.entryId, ...repositoryDefaults[fact.section] }
      repo[fact.section].push(entry)
    }
    entry[fact.field] = fact.text
  }
  return repo
}

// Drive the activated client request path with one controlled handler response.
async function clientOutcome(facts, cvLanguage, run) {
  const originalFetch = globalThis.fetch
  let outbound
  try {
    globalThis.fetch = async (_url, options) => {
      outbound = JSON.parse(options.body)
      return new Response(JSON.stringify(run.output), {
        status: run.status,
        headers: { "Content-Type": "application/json" },
      })
    }
    let result
    let error
    try {
      result = await generateCv(
        facts,
        cvLanguage,
        "sk-controlled",
        new AbortController().signal,
      )
    } catch (thrown) {
      error = thrown
    }
    return { outbound, result, error }
  } finally {
    globalThis.fetch = originalFetch
  }
}

test("the controlled corpus is a v3 assessment run over the local production handler only", () => {
  assert.equal(report.mode, "controlled")
  assert.equal(report.fixtureVersion, corpus.version)
  assert.match(report.scope, /synthetic/i)
  assert.match(report.scope, /no browser|deployment/i)
  assert.equal(report.runs.length, corpus.cases.length)
  assert.equal(
    CV_SUPPORT_REQUEST,
    true,
    "activation requests the v3 assessment",
  )
  // Every case carries an independent expectation and no stale expectation is
  // left behind.
  assert.deepEqual(
    Object.keys(expectations).sort(),
    corpus.cases.map((c) => c.id).sort(),
  )
  for (const fixture of corpus.cases) {
    const run = report.runs.find((r) => r.id === fixture.id)
    const expected = expectations[fixture.id]
    assert.ok(run, fixture.id)
    assert.equal(run.mode, "controlled", fixture.id)
    assert.equal(run.language, fixture.language, fixture.id)
    assert.equal(run.task, "cv_generation", fixture.id)
    // Expected status and call count come from the fixture, not the run.
    assert.equal(run.status, expected.status, fixture.id)
    assert.equal((run.calls ?? []).length, expected.calls, fixture.id)
    // No corpus run may exceed the one generation call plus the single
    // assessment call; a hard guard fails before the assessment is attempted.
    assert.ok((run.calls ?? []).length <= 2, fixture.id)
  }
})

test("the client requests the v3 assessment and joins real handler output with real review decisions", async () => {
  let falseAccepts = 0
  let falseRejects = 0
  const summary = []

  for (const fixture of corpus.cases) {
    const run = report.runs.find((r) => r.id === fixture.id)
    const rawFacts = fixture.request.facts.map((f) => ({
      id: f.id,
      section: f.section,
      entryId: f.entryId,
      field: f.field,
      text: f.text,
    }))
    const document = canonicalDocument(rawFacts)
    const facts = stableCvFacts(document)
    const repo = repositoryFromFacts(rawFacts)
    const { outbound, result, error } = await clientOutcome(
      facts,
      fixture.language,
      run,
    )

    // The activated client always opts into the v3 contract.
    assert.equal(outbound.statementSupport, true, fixture.id)
    assert.equal(outbound.cvLanguage, fixture.language, fixture.id)

    const expected = expectations[fixture.id]
    if (fixture.role === "negative_control") {
      // Deterministic hard guard: the fixture expects one generation call and no
      // assessment, so the client never receives machine wording to accept.
      assert.equal(run.status, expected.status, fixture.id)
      assert.equal((run.calls ?? []).length, expected.calls, fixture.id)
      assert.equal(error?.message, "invalid_output", fixture.id)
      assert.equal(result, undefined, fixture.id)
      summary.push(`${fixture.id}\t${run.status}\thard-guard\t1 call`)
      continue
    }

    assert.equal(run.status, expected.status, fixture.id)
    assert.equal((run.calls ?? []).length, expected.calls, fixture.id)
    assert.equal(run.output.schemaVersion, 3, fixture.id)
    assert.ok(!error, `${fixture.id}: ${error?.message}`)
    assert.ok(validateCvSupport(result.support, result), fixture.id)
    assert.equal(
      result.support.policyVersion,
      "cv-statement-support-v1",
      fixture.id,
    )

    // Assert the handler's assessment against the fixture expectation before
    // selecting the acceptance path. A regression that promoted an unsupported
    // judgment to supported fails here instead of silently accepting it.
    const units = cvReviewUnits(result)
    assert.equal(units.length, cvSupportUnits(result).length, fixture.id)
    assert.equal(result.support.assessment, expected.assessment, fixture.id)
    for (const unit of units)
      assert.equal(
        unit.status,
        expected.judgments[unit.id],
        `${fixture.id} ${unit.id}`,
      )
    const expectedUnresolved = Object.values(expected.judgments).some(
      (status) => status !== "supported",
    )

    if (!expectedUnresolved) {
      // A supported machine sentence stays usable: no false reject.
      try {
        assert.equal(cvReviewResolved(result, emptyCvReview), true, fixture.id)
        const accepted = acceptCvResult(result, facts, emptyCvReview)
        assert.ok(accepted.summary.length, fixture.id)
        const saved = createCuratedCv(
          repo,
          facts,
          result,
          fixture.language,
          "balanced",
          document,
          emptyCvReview,
        )
        assert.equal(saved.version, 3, fixture.id)
        assert.equal(cvWordingTrust(saved), "assessed", fixture.id)
        assert.equal(cvSupportStatus(saved, document), "current", fixture.id)
        assert.deepEqual(
          parseCuratedCv(JSON.stringify(saved)),
          saved,
          fixture.id,
        )
        assert.ok(
          saved.provenance.every((r) => r.origin === "generated"),
          fixture.id,
        )
        // The machine sentence is retained verbatim; the source fact is intact.
        assert.ok(
          result.summary.some((s) => saved.summary.includes(s.text)),
          fixture.id,
        )
        assert.deepEqual(document, canonicalDocument(rawFacts), fixture.id)
      } catch (thrown) {
        falseRejects += 1
        throw thrown
      }
      summary.push(`${fixture.id}\t${run.status}\taccepted\t2 calls`)
      continue
    }

    // Unsupported or unavailable machine text must never be accepted as-is.
    assert.ok(
      units.some((u) => u.status !== "supported"),
      `${fixture.id}: expected an unresolved machine unit`,
    )
    let acceptedAsIs = cvReviewResolved(result, emptyCvReview)
    try {
      acceptCvResult(result, facts, emptyCvReview)
      acceptedAsIs = true
    } catch (thrown) {
      assert.match(
        thrown.message,
        /unresolved_output|invalid_output/,
        fixture.id,
      )
    }
    if (acceptedAsIs) falseAccepts += 1
    assert.throws(
      () => createCuratedCv(repo, facts, result, fixture.language),
      /unresolved_output|invalid_output/,
      fixture.id,
    )

    if (expected.assessment === "unavailable") {
      // Assessment outage fails closed: every unit is already uncertain, so the
      // user must explicitly confirm before anything is persisted.
      assert.equal(result.support.assessment, "unavailable", fixture.id)
      assert.ok(
        units.every((u) => u.status === "uncertain"),
        fixture.id,
      )
      assert.equal(
        cvReviewResolved(result, {
          ...emptyCvReview,
          confirmedIds: units.map((u) => u.id),
        }),
        true,
        fixture.id,
      )
      const confirmed = createCuratedCv(
        repo,
        facts,
        result,
        fixture.language,
        "balanced",
        document,
        { ...emptyCvReview, confirmedIds: units.map((u) => u.id) },
      )
      assert.equal(confirmed.version, 3, fixture.id)
      assert.equal(cvWordingTrust(confirmed), "assessed", fixture.id)
      assert.ok(
        confirmed.provenance.every(
          (r) => r.origin === "user_confirmed" && r.decision === "confirm",
        ),
        fixture.id,
      )
      summary.push(`${fixture.id}\t${run.status}\tconfirmed\t2 calls`)
      continue
    }

    // Unsupported ordinary claims: the user corrects the sentence with their own
    // wording, which resolves review without ever promoting the machine claim.
    const unsupported = units.filter((u) => u.kind === "summary")
    const editedText = Object.fromEntries(
      unsupported.map((u) => [u.id, "Own wording grounded in my experience."]),
    )
    const decisions = { ...emptyCvReview, editedText }
    assert.equal(cvReviewResolved(result, decisions), true, fixture.id)
    const { result: resolved, provenance } = resolveCvReview(
      result,
      facts,
      decisions,
    )
    assert.equal(provenance[0].origin, "user_edited", fixture.id)
    assert.equal(provenance[0].original.status, "unsupported", fixture.id)
    const edited = createCuratedCv(
      repo,
      facts,
      result,
      fixture.language,
      "balanced",
      document,
      decisions,
    )
    assert.equal(edited.version, 3, fixture.id)
    assert.equal(cvWordingTrust(edited), "assessed", fixture.id)
    assert.ok(
      !edited.summary.includes(run.output.summary[0].text),
      `${fixture.id}: the unsupported machine sentence never persists`,
    )
    assert.ok(resolved.summary.length, fixture.id)
    summary.push(`${fixture.id}\t${run.status}\tblocked+edited\t2 calls`)
  }

  // The controlled enforcement invariant: nothing unsupported was accepted and
  // nothing supported was wrongly rejected.
  assert.equal(falseAccepts, 0, "no unsupported machine wording was accepted")
  assert.equal(falseRejects, 0, "no supported machine wording was rejected")
  console.log(summary.join("\n"))
})

test("the activated request is cancellable and accepts nothing on abort", async () => {
  const rawFacts = [
    {
      id: "java",
      section: "skills",
      entryId: "",
      field: "skills",
      text: "Java",
    },
  ]
  const document = canonicalDocument(rawFacts)
  const facts = stableCvFacts(document)
  const originalFetch = globalThis.fetch
  const controller = new AbortController()
  let outbound
  try {
    globalThis.fetch = (_url, options) =>
      new Promise((_resolve, reject) => {
        outbound = JSON.parse(options.body)
        options.signal.addEventListener("abort", () =>
          reject(new DOMException("aborted", "AbortError")),
        )
      })
    const pending = generateCv(facts, "en", "sk-controlled", controller.signal)
    await new Promise((resolve) => setTimeout(resolve, 0))
    controller.abort()
    await assert.rejects(pending, /cancelled/)
  } finally {
    globalThis.fetch = originalFetch
  }
  assert.equal(outbound.statementSupport, true)
})
