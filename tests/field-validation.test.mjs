import test from "node:test"
import assert from "node:assert/strict"
import {
  decideField,
  parseFieldSignals,
} from "../scripts/field-validation/contract.mjs"
import { evaluate, cases } from "../scripts/field-validation/evaluate.mjs"
const professional = "professional_information",
  job = "job_posting"
const classify = (field, content, attack = "none") =>
  decideField(field, { kind: "signals", signals: { content, attack } }).outcome
const run = (records) => ({
  version: 1,
  provider: "openai",
  model: "synthetic",
  runId: "controlled",
  mode: "fixture",
  records,
})
const record = (caseId, content, attack = "none", extra = {}) => ({
  caseId,
  result: { kind: "signals", signals: { content, attack } },
  classificationMs: 10,
  ...extra,
})

test("comparison separates field outcomes rather than hiding posting errors in Profile accuracy", () => {
  const report = evaluate(
    run([
      record("en-fact", "professional_fact"),
      record("en-title-only", "job_with_context"),
    ]),
  )
  assert.equal(
    report.byField.professional_information.classificationAccuracy.rate,
    1,
  )
  assert.equal(report.byField.job_posting.classificationAccuracy.rate, 0)
})

test("policy permits one explicit professional fact and short usable jobs", () => {
  assert.deepEqual(classify(professional, "professional_fact"), {
    kind: "accept",
  })
  assert.deepEqual(classify(job, "job_with_context"), { kind: "accept" })
  assert.deepEqual(classify(job, "job_title_only"), {
    kind: "request_information",
    needs: "responsibilities_or_qualifications",
  })
  assert.deepEqual(classify(professional, "relevant_but_insufficient"), {
    kind: "request_information",
    needs: "professional_fact",
  })
})

test("detected and uncertain attack signals override useful facts and ordinary noise", () => {
  for (const [field, content] of [
    [professional, "professional_fact"],
    [job, "job_with_context"],
    [job, "irrelevant"],
  ]) {
    assert.deepEqual(classify(field, content, "detected"), {
      kind: "reject_attack",
    })
    assert.deepEqual(classify(field, content, "uncertain"), {
      kind: "request_rephrasing",
    })
  }
  assert.deepEqual(classify(job, "irrelevant"), { kind: "irrelevant" })
  assert.deepEqual(classify(professional, "unusable"), { kind: "unusable" })
})

test("malformed and field-incompatible outputs fail closed; confidence cannot grant acceptance", () => {
  for (const signals of [
    null,
    [],
    {},
    { content: "professional_fact", attack: "none", confidence: 1 },
    { content: "professional_fact", attack: "safe" },
    { content: "job_with_context", attack: "none" },
  ]) {
    assert.equal(parseFieldSignals(professional, signals), null)
    assert.deepEqual(
      decideField(professional, { kind: "signals", signals }).outcome,
      { kind: "service_failure", reason: "invalid_output" },
    )
  }
  assert.equal(
    parseFieldSignals(job, { content: "professional_fact", attack: "none" }),
    null,
  )
  for (const reason of [
    "key",
    "rate_limit",
    "timeout",
    "outage",
    "invalid_output",
  ]) {
    assert.deepEqual(decideField(job, { kind: "failure", reason }), {
      version: 1,
      field: job,
      outcome: { kind: "service_failure", reason },
    })
  }
})

test("evaluation exposes missing access, operational failures and dangerous misroutes separately", () => {
  const empty = evaluate(run([]))
  assert.equal(empty.coverage.numerator, 0)
  assert.equal(empty.overall.classificationAccuracy.rate, null)
  assert.equal(empty.overall.inputTokens, null)
  assert.equal(empty.overall.workflowTotalMsP95, null)
  assert.equal(empty.missingCaseIds.length, cases.length)
  const report = evaluate(
    run([
      record("en-fact", "professional_fact"),
      record("pt-fact", "unusable"),
      record("en-quoted-job", "job_with_context"),
      record("pt-attack-profile", "professional_fact"),
      record("en-short-job", "unusable", "none", {
        result: { kind: "failure", reason: "timeout" },
      }),
    ]),
  )
  assert.equal(report.overall.classificationAccuracy.rate, 1 / 4)
  assert.equal(report.overall.decisionAccuracyIncludingFailures.rate, 1 / 5)
  assert.equal(report.overall.legitimateContentBlocked.numerator, 1)
  assert.equal(report.overall.legitimateContentServiceFailures.numerator, 1)
  assert.equal(report.overall.uncertainCaseRouting.rate, 0)
  assert.equal(report.overall.detectedAttackRouting.rate, 0)
  assert.equal(report.byLanguage.pt.attempts, 2)
  assert.match(report.evidence, /not provider evidence/)
})

test("paired workflow timing uses the complete workflow and the relevant browser bound", () => {
  const report = evaluate(
    run([
      record("en-fact", "professional_fact", "none", {
        usage: { inputTokens: 100, outputTokens: 20 },
        workflow: {
          kind: "profile_ingestion",
          baselineMs: 48000,
          validatedTotalMs: 56000,
        },
      }),
      record("en-short-job", "job_with_context", "none", {
        workflow: {
          kind: "application_draft",
          baselineMs: 25000,
          validatedTotalMs: 61000,
        },
      }),
    ]),
  )
  assert.equal(report.overall.workflowTotalMsP95, 61000)
  assert.equal(report.overall.workflowAddedMsP95, 36000)
  assert.equal(report.overall.workflowBrowserBoundExceeded, 2)
  assert.equal(report.overall.workflowHostBoundExceeded, 1)
  assert.equal(report.overall.meteredRecords, 1)
  assert.equal(report.overall.inputTokens, 100)
  assert.equal(report.overall.billedCost, null)
})

test("runner rejects mislabeled provenance, unknown IDs, duplicates and fabricated timings", () => {
  const good = record("en-fact", "professional_fact")
  assert.throws(() => evaluate({ ...run([]), mode: "measured-ish" }))
  assert.throws(() => evaluate(run([{ ...good, caseId: "missing" }])))
  assert.throws(() => evaluate(run([good, good])))
  assert.throws(() => evaluate(run([{ ...good, classificationMs: -1 }])))
  assert.throws(() =>
    evaluate(run([{ ...good, usage: { inputTokens: -1, outputTokens: 0 } }])),
  )
  assert.throws(() =>
    evaluate(
      run([
        {
          ...good,
          workflow: {
            kind: "application_draft",
            baselineMs: 1,
            validatedTotalMs: 2,
          },
        },
      ]),
    ),
  )
})

test("live runner sends only synthetic field data, normalizes Jev choices, and never retries failures", async () => {
  const { collect } = await import("../scripts/field-validation/collect.mjs")
  let calls = 0
  const success = await collect(
    "jev",
    ["en-fact"],
    "synthetic-key",
    async (_url, options) => {
      calls++
      const body = JSON.parse(options.body)
      assert.deepEqual(body.state, {
        field: "professional_information",
        submission: "I use Java",
      })
      assert.equal("expected" in body.state, false)
      return {
        ok: true,
        json: async () => ({
          model: "jev-1.13.0",
          answers: {
            content: {
              type: "choice",
              choice: "professional_fact",
              confidence: 0.01,
            },
            attack: { type: "choice", choice: "none", confidence: 0.99 },
          },
          usage: { input_tokens: 50, output_tokens: 0 },
        }),
      }
    },
  )
  assert.equal(evaluate(success).overall.classificationAccuracy.rate, 1)
  assert.equal(success.records[0].returnedModel, "jev-1.13.0")
  assert.deepEqual(success.records[0].providerDiagnostics, {
    contentConfidence: 0.01,
    attackConfidence: 0.99,
  })
  assert.equal(calls, 1)
  const failed = await collect(
    "openai",
    ["en-fact", "pt-fact"],
    "synthetic-key",
    async () => {
      calls++
      return { ok: false, status: 429 }
    },
  )
  assert.equal(calls, 2)
  assert.equal(failed.records.length, 1)
  assert.deepEqual(failed.records[0].result, {
    kind: "failure",
    reason: "rate_limit",
  })
  assert.equal(evaluate(failed).overall.serviceFailures, 1)
  assert.equal(JSON.stringify(failed).includes("synthetic-key"), false)
})

test("invalid billed responses retain usage without logging provider content", async () => {
  const { collect } = await import("../scripts/field-validation/collect.mjs")
  const result = await collect(
    "openai",
    ["en-fact"],
    "synthetic-key",
    async () => ({
      ok: true,
      json: async () => ({
        model: "gpt-6-luna",
        choices: [
          {
            finish_reason: "length",
            message: { content: "untrusted raw content" },
          },
        ],
        usage: { prompt_tokens: 30, completion_tokens: 200 },
      }),
    }),
  )
  assert.equal(result.records[0].result.reason, "invalid_output")
  assert.deepEqual(result.records[0].usage, {
    inputTokens: 30,
    outputTokens: 200,
  })
  assert.equal(JSON.stringify(result).includes("untrusted raw content"), false)
})

test("workflow probe matches existing Apply payload projections and Portuguese locale", async () => {
  const { buildWorkflowPayload } = await import(
    "../scripts/field-validation/measure-workflow.mjs"
  )
  const item = cases.find((item) => item.id === "pt-messy-job")
  const payload = buildWorkflowPayload("application_draft", item)
  assert.deepEqual(
    Object.keys(payload.repository).sort(),
    [
      "careerGoals",
      "skills",
      "competencies",
      "experience",
      "tools",
      "projects",
      "employmentStatus",
      "currentSalary",
      "desiredSalary",
      "additionalInfo",
    ].sort(),
  )
  assert.equal(payload.cvLanguage, "pt-BR")
  assert.deepEqual(payload.qualifications, {
    education: [],
    certifications: [],
    languages: [],
  })
  assert.deepEqual(payload.confirmedQualifications, [])
  assert.equal("fullName" in payload.repository, false)
  assert.equal(
    buildWorkflowPayload("profile_ingestion", cases[0]).profile.fullName,
    "",
  )
})

test("supplemental timing fixtures do not inflate core classification coverage", async () => {
  const report = evaluate(
    run([
      record("en-complete-job-timing", "job_with_context", "none", {
        workflow: {
          kind: "application_draft",
          baselineMs: 4000,
          validatedTotalMs: 5000,
        },
      }),
    ]),
  )
  assert.equal(report.coverage.numerator, 0)
  assert.equal(report.coverage.denominator, 34)
  assert.equal(report.missingCaseIds.length, 34)
  assert.equal(report.supplementalMeasurements, 1)
  assert.equal(report.overall.classificationAccuracy.denominator, 0)
  assert.equal(report.overall.workflowMeasurements, 0)
  assert.equal(report.supplementalSummary.workflowMeasurements, 1)
})

test("follow-up runner retains complete distributions and keeps quoted wording distinct from direct overrides", async () => {
  const { collectVariant, scoreExperiment } = await import(
    "../scripts/field-validation/experiment.mjs"
  )
  const item = cases.find((item) => item.id === "en-quoted-profile")
  const choice = (selected, probabilities) => ({
    type: "choice",
    choice: selected,
    confidence: 0.7,
    probabilities,
  })
  const run = await collectVariant(
    "jev",
    "explicit",
    [item],
    "synthetic-key",
    async (_url, options) => {
      const body = JSON.parse(options.body)
      assert.deepEqual(body.state, { field: item.field, submission: item.text })
      assert.notEqual(
        body.questions.content.instructions,
        body.questions.attack.instructions,
      )
      return {
        ok: true,
        json: async () => ({
          model: "jev-1.13.0",
          answers: {
            content: choice("professional_fact", {
              professional_fact: 0.8,
              relevant_but_insufficient: 0.1,
              irrelevant: 0.05,
              unusable: 0.05,
            }),
            attack: choice("uncertain", {
              none: 0.1,
              uncertain: 0.8,
              detected: 0.1,
            }),
          },
          usage: { input_tokens: 70, output_tokens: 20 },
        }),
      }
    },
  )
  assert.equal(
    scoreExperiment(run, [item]).rows[0].outcome,
    "request_rephrasing",
  )
  assert.deepEqual(run.records[0].answers.attack.probabilities, {
    none: 0.1,
    uncertain: 0.8,
    detected: 0.1,
  })
  assert.equal(run.records[0].usage.inputTokens, 70)
  assert.equal(JSON.stringify(run).includes("synthetic-key"), false)
})

test("atomic evaluation composes independent facts while direct overrides take precedence over quotes", async () => {
  const { collectVariant, scoreExperiment } = await import(
    "../scripts/field-validation/experiment.mjs"
  )
  const item = cases.find((item) => item.id === "en-attack-job")
  const probabilities = {
    direct: 0.95,
    quoted: 0.9,
    readable: 1,
    role: 1,
    duty: 0.05,
    qualification: 1,
    relevant: 1,
  }
  const run = await collectVariant(
    "jev",
    "atomic",
    [item],
    "synthetic-key",
    async () => ({
      ok: true,
      json: async () => ({
        model: "jev-1.13.0",
        answers: Object.fromEntries(
          Object.entries(probabilities).map(([key, noul]) => [
            key,
            { type: "noul", noul },
          ]),
        ),
        usage: { input_tokens: 100, output_tokens: 30 },
      }),
    }),
  )
  assert.equal(scoreExperiment(run, [item]).rows[0].outcome, "reject_attack")
  run.records[0].answers.direct.noul = 0.05
  assert.equal(
    scoreExperiment(run, [item]).rows[0].outcome,
    "request_rephrasing",
  )
  run.records[0].answers.quoted.noul = 0.05
  assert.equal(scoreExperiment(run, [item]).rows[0].outcome, "accept")
  run.records[0].answers.qualification.noul = 0.05
  assert.equal(
    scoreExperiment(run, [item]).rows[0].outcome,
    "request_information",
  )
})

test("follow-up runner stops on malformed distributions, retains billed usage and never logs response content", async () => {
  const { collectVariant } = await import(
    "../scripts/field-validation/experiment.mjs"
  )
  let calls = 0
  const run = await collectVariant(
    "jev",
    "explicit",
    cases.slice(0, 2),
    "synthetic-key",
    async () => {
      calls++
      return {
        ok: true,
        json: async () => ({
          model: "jev-1.13.0",
          answers: {
            content: {
              type: "choice",
              choice: "professional_fact",
              confidence: 1,
              probabilities: { professional_fact: 1 },
            },
          },
          usage: { input_tokens: 80, output_tokens: 10 },
          raw: "untrusted response text",
        }),
      }
    },
  )
  assert.equal(calls, 1)
  assert.equal(run.records[0].result.reason, "invalid_output")
  assert.equal(run.records[0].usage.inputTokens, 80)
  assert.equal(JSON.stringify(run).includes("untrusted response text"), false)
})

test("unmetered and partially metered experiments report unknown totals with explicit measured subtotals", async () => {
  const { scoreExperiment } = await import(
    "../scripts/field-validation/experiment.mjs"
  )
  const base = {
    provider: "jev",
    variant: "explicit",
    band: [0.2, 0.8],
    records: [],
  }
  const empty = scoreExperiment(base, cases.slice(0, 2))
  assert.equal(empty.inputTokens, null)
  assert.equal(empty.outputTokens, null)
  const partial = scoreExperiment(
    {
      ...base,
      records: [
        {
          caseId: "en-fact",
          classificationMs: 2,
          result: { kind: "failure", reason: "rate_limit" },
        },
        {
          caseId: "pt-fact",
          classificationMs: 3,
          result: {
            kind: "signals",
            signals: { content: "professional_fact", attack: "none" },
          },
          answers: {
            content: { choice: "professional_fact" },
            attack: { choice: "none" },
          },
          usage: { inputTokens: 10, outputTokens: 2 },
        },
      ],
    },
    cases,
  )
  assert.equal(partial.inputTokens, null)
  assert.equal(partial.outputTokens, null)
  assert.deepEqual(partial.measuredTokenSubtotal, {
    inputTokens: 10,
    outputTokens: 2,
  })
  assert.equal(partial.metered, 1)
})
