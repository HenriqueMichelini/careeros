import { readFileSync } from "node:fs"
import { pathToFileURL } from "node:url"
import { decideField } from "./contract.mjs"

export const cases = JSON.parse(
  readFileSync(
    new URL(
      "../../docs/evaluations/field-validation-cases.json",
      import.meta.url,
    ),
    "utf8",
  ),
)
const workflows = {
  profile_ingestion: 55000,
  qualification_gaps: 30000,
  application_draft: 30000,
}
const failures = ["key", "rate_limit", "timeout", "outage", "invalid_output"]
const finite = (value) =>
  typeof value === "number" && Number.isFinite(value) && value >= 0
const percentile = (values, fraction) =>
  values.length
    ? [...values].sort((a, b) => a - b)[Math.ceil(values.length * fraction) - 1]
    : null
const rate = (numerator, denominator) => ({
  numerator,
  denominator,
  rate: denominator ? numerator / denominator : null,
})

/** Recorded adapter results only; labels are never used as provider predictions. */
export function evaluate(run) {
  if (
    !run ||
    run.version !== 1 ||
    !["openai", "jev"].includes(run.provider) ||
    typeof run.model !== "string" ||
    !run.model.trim() ||
    typeof run.runId !== "string" ||
    !run.runId.trim() ||
    !["live", "fixture"].includes(run.mode) ||
    !Array.isArray(run.records)
  )
    throw new Error("Invalid run metadata")
  const seen = new Set()
  const rows = run.records.map((record) => {
    const item = cases.find((item) => item.id === record.caseId)
    if (
      !item ||
      seen.has(item.id) ||
      !finite(record.classificationMs) ||
      !record.result ||
      !["signals", "failure"].includes(record.result.kind) ||
      (record.result.kind === "failure" &&
        !failures.includes(record.result.reason))
    )
      throw new Error("Invalid or duplicate case record")
    seen.add(item.id)
    const outcome = decideField(item.field, record.result).outcome.kind
    const row = {
      caseId: item.id,
      field: item.field,
      language: item.language,
      category: item.category,
      expected: item.expected,
      outcome,
      correct: outcome === item.expected,
      classificationMs: record.classificationMs,
    }
    if (record.usage !== undefined) {
      if (
        !record.usage ||
        !Number.isInteger(record.usage.inputTokens) ||
        !finite(record.usage.inputTokens) ||
        !Number.isInteger(record.usage.outputTokens) ||
        !finite(record.usage.outputTokens)
      )
        throw new Error("Invalid token usage")
      row.usage = record.usage
    }
    if (record.workflow !== undefined) {
      const timing = record.workflow
      if (
        !(timing.kind in workflows) ||
        !finite(timing.baselineMs) ||
        !finite(timing.validatedTotalMs) ||
        (item.field === "professional_information") !==
          (timing.kind === "profile_ingestion")
      )
        throw new Error("Invalid paired workflow timing")
      row.workflow = {
        kind: timing.kind,
        baselineMs: timing.baselineMs,
        validatedTotalMs: timing.validatedTotalMs,
        addedMs: timing.validatedTotalMs - timing.baselineMs,
        withinBrowserBound: timing.validatedTotalMs < workflows[timing.kind],
        withinHostBound: timing.validatedTotalMs < 60000,
      }
    }
    if (record.providerDiagnostics) {
      const diagnostic = record.providerDiagnostics
      if (![diagnostic.contentConfidence, diagnostic.attackConfidence].every(value => finite(value) && value <= 1)) throw new Error("Invalid provider diagnostics")
      row.providerDiagnostics = { contentConfidence: diagnostic.contentConfidence, attackConfidence: diagnostic.attackConfidence }
    }
    if (typeof record.returnedModel === "string")
      row.returnedModel = record.returnedModel
    if (Number.isInteger(record.httpStatus)) row.httpStatus = record.httpStatus
    if (record.workflowAttempt) {
      const attempt = record.workflowAttempt
      if (
        !(attempt.kind in workflows) ||
        !finite(attempt.baselineMs) ||
        !finite(attempt.validatedTotalMs) ||
        typeof attempt.baseline?.ok !== "boolean" ||
        typeof attempt.validated?.ok !== "boolean"
      )
        throw new Error("Invalid workflow attempt")
      row.workflowAttempt = {
        kind: attempt.kind,
        baselineOK: attempt.baseline.ok,
        validatedOK: attempt.validated.ok,
        baselineMs: attempt.baselineMs,
        validatedTotalMs: attempt.validatedTotalMs,
      }
    }
    return row
  })
  const summarize = (subset) => {
    const classified = subset.filter((row) => row.outcome !== "service_failure")
    const legitimate = subset.filter((row) => row.expected === "accept")
    const uncertain = subset.filter(
      (row) => row.expected === "request_rephrasing",
    )
    const detected = subset.filter((row) => row.expected === "reject_attack")
    const timed = subset.filter((row) => row.workflow)
    const metered = subset.filter((row) => row.usage)
    return {
      attempts: subset.length,
      decisionAccuracyIncludingFailures: rate(
        subset.filter((row) => row.correct).length,
        subset.length,
      ),
      classificationAccuracy: rate(
        classified.filter((row) => row.correct).length,
        classified.length,
      ),
      legitimateContentBlocked: rate(
        legitimate.filter(
          (row) =>
            row.outcome !== "accept" && row.outcome !== "service_failure",
        ).length,
        legitimate.length,
      ),
      legitimateContentServiceFailures: rate(
        legitimate.filter((row) => row.outcome === "service_failure").length,
        legitimate.length,
      ),
      uncertainCaseRouting: rate(
        uncertain.filter((row) => row.outcome === "request_rephrasing").length,
        uncertain.length,
      ),
      detectedAttackRouting: rate(
        detected.filter((row) => row.outcome === "reject_attack").length,
        detected.length,
      ),
      serviceFailures: subset.filter((row) => row.outcome === "service_failure")
        .length,
      classificationLatencyMs: {
        p50: percentile(
          subset.map((row) => row.classificationMs),
          0.5,
        ),
        p95: percentile(
          subset.map((row) => row.classificationMs),
          0.95,
        ),
      },
      workflowAttempts: subset.filter((row) => row.workflowAttempt).length,
      workflowFailures: subset.filter(
        (row) => row.workflowAttempt && !row.workflowAttempt.validatedOK,
      ).length,
      workflowMeasurements: timed.length,
      workflowAddedMsP95: percentile(
        timed.map((row) => row.workflow.addedMs),
        0.95,
      ),
      workflowTotalMsP95: percentile(
        timed.map((row) => row.workflow.validatedTotalMs),
        0.95,
      ),
      workflowBrowserBoundExceeded: timed.filter(
        (row) => !row.workflow.withinBrowserBound,
      ).length,
      workflowHostBoundExceeded: timed.filter(
        (row) => !row.workflow.withinHostBound,
      ).length,
      meteredRecords: metered.length,
      inputTokens: metered.length
        ? metered.reduce((sum, row) => sum + row.usage.inputTokens, 0)
        : null,
      outputTokens: metered.length
        ? metered.reduce((sum, row) => sum + row.usage.outputTokens, 0)
        : null,
      billedCost: null,
    }
  }
  return {
    version: 1,
    provider: run.provider,
    model: run.model,
    runId: run.runId,
    mode: run.mode,
    evidence:
      run.mode === "live"
        ? "operator-supplied live records; provenance requires review"
        : "fixture only; not provider evidence",
    coverage: rate(rows.length, cases.length),
    missingCaseIds: cases
      .filter((item) => !seen.has(item.id))
      .map((item) => item.id),
    overall: summarize(rows),
    byLanguage: Object.fromEntries(
      ["en", "pt"].map((language) => [
        language,
        summarize(rows.filter((row) => row.language === language)),
      ]),
    ),
    byField: Object.fromEntries(
      ["professional_information", "job_posting"].map((field) => [
        field,
        summarize(rows.filter((row) => row.field === field)),
      ]),
    ),
    byCategory: Object.fromEntries(
      [...new Set(cases.map((item) => item.category))].map((category) => [
        category,
        summarize(rows.filter((row) => row.category === category)),
      ]),
    ),
    rows,
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const files = process.argv.slice(2)
  const reports = files.length
    ? files.map((file) => evaluate(JSON.parse(readFileSync(file, "utf8"))))
    : ["openai", "jev"].map((provider) => ({
        provider,
        status: "unavailable",
        measuredCases: 0,
        totalCases: cases.length,
        accuracy: null,
        latencyMs: null,
        cost: null,
        workflowLatencyMs: null,
      }))
  process.stdout.write(`${JSON.stringify(reports, null, 2)}\n`)
}
