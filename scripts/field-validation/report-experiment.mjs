// Offline report only: reads recorded metadata, never contacts providers.
import { readFileSync, readdirSync } from "node:fs"
import { fileURLToPath } from "node:url"
import { cases } from "./evaluate.mjs"
import { hash, scoreExperiment } from "./experiment.mjs"
const phase = process.argv[2] ?? "followup"
if (!["followup", "confirmatory"].includes(phase))
  throw new Error("Select followup or confirmatory report")
const folder = new URL(
  `../../docs/evaluations/results/2026-10-07/${phase}/`,
  import.meta.url,
)
const followup = JSON.parse(
  readFileSync(
    new URL(
      `../../docs/evaluations/field-validation-${phase}-cases.json`,
      import.meta.url,
    ),
    "utf8",
  ),
)
const files = readdirSync(folder)
  .filter((name) => /^(jev|openai)-.*\.json$/.test(name))
  .sort()
const runs = files.map((name) => {
  const raw = readFileSync(new URL(name, folder), "utf8")
  const run = JSON.parse(raw)
  const all =
    run.split === "development" || ["repeat", "smoke"].includes(run.split)
      ? cases
      : followup.filter((item) => item.split === run.split)
  const fixedIds =
    run.split === "smoke"
      ? ["en-fact", "pt-quoted-job"]
      : [
          "en-quoted-profile",
          "pt-quoted-job",
          "en-unusable-profile",
          "pt-applicant",
        ]
  const selected = ["repeat", "smoke"].includes(run.split)
    ? all.filter((item) => fixedIds.includes(item.id))
    : all
  const report = scoreExperiment(run, selected)
  const dimensions = (key) =>
    Object.fromEntries(
      [...new Set(report.rows.map((row) => row[key]))].map((value) => {
        const rows = report.rows.filter((row) => row[key] === value)
        return [
          value,
          {
            correct: rows.filter((row) => row.correct).length,
            attempts: rows.length,
          },
        ]
      }),
    )
  const route = (expected) => {
    const rows = report.rows.filter((row) => row.expected === expected)
    return {
      correct: rows.filter((row) => row.correct).length,
      cases: rows.length,
      accepted: rows.filter((row) => row.outcome === "accept").length,
    }
  }
  const referenceRateEstimate =
    report.inputTokens === null || report.outputTokens === null
      ? null
      : run.provider === "jev"
        ? (report.inputTokens * 0.042) / 1e6
        : (report.inputTokens * 0.1 + report.outputTokens * 0.5) / 1e6
  const { rows, ...summary } = report
  return {
    file: name,
    runId: run.runId,
    hash: hash(raw),
    split: run.split,
    reverse: run.reverse,
    ...summary,
    byLanguage: dimensions("language"),
    byField: dimensions("field"),
    byCategory: dimensions("category"),
    routing: Object.fromEntries(
      [
        "accept",
        "reject_attack",
        "request_rephrasing",
        "request_information",
        "irrelevant",
        "unusable",
      ].map((value) => [value, route(value)]),
    ),
    rephrasingRequests: rows.filter(
      (row) => row.outcome === "request_rephrasing",
    ).length,
    clarificationRequests: rows.filter(
      (row) => row.outcome === "request_information",
    ).length,
    referenceRateEstimate,
    billedCost: null,
    mismatches: rows.filter((row) => !row.correct),
  }
})
const workflows = readdirSync(folder)
  .filter((name) => name.startsWith("workflow-") && name.endsWith(".json"))
  .sort()
  .map((name) => {
    const run = JSON.parse(readFileSync(new URL(name, folder), "utf8"))
    const record = run.records?.[0]
    return {
      file: name,
      variant: run.variant,
      caseId: record?.caseId ?? run.caseId,
      workflow: record?.workflow,
      attempt: record?.workflowAttempt ?? {
        baseline: run.baseline,
        baselineMs: run.baselineMs,
      },
      classifierMs: record?.classificationMs,
      result: record?.result,
      usage: record?.usage,
    }
  })
const usage = (provider) => {
  const included = runs.filter((run) => run.provider === provider)
  const probes =
    provider === "jev" ? workflows.filter((workflow) => workflow.result) : []
  const meteredProbes = probes.filter((workflow) => workflow.usage)
  const classifierCalls =
    included.reduce((sum, run) => sum + run.attempts, 0) + probes.length
  const measuredTokenSubtotal = {
    inputTokens:
      included.reduce(
        (sum, run) => sum + run.measuredTokenSubtotal.inputTokens,
        0,
      ) + meteredProbes.reduce((sum, run) => sum + run.usage.inputTokens, 0),
    outputTokens:
      included.reduce(
        (sum, run) => sum + run.measuredTokenSubtotal.outputTokens,
        0,
      ) + meteredProbes.reduce((sum, run) => sum + run.usage.outputTokens, 0),
  }
  const complete =
    classifierCalls > 0 &&
    included.every(
      (run) => run.inputTokens !== null && run.outputTokens !== null,
    ) &&
    probes.length === meteredProbes.length
  const measuredReferenceSubtotal =
    provider === "jev"
      ? (measuredTokenSubtotal.inputTokens * 0.042) / 1e6
      : (measuredTokenSubtotal.inputTokens * 0.1 +
          measuredTokenSubtotal.outputTokens * 0.5) /
        1e6
  return {
    classifierCalls,
    inputTokens: complete ? measuredTokenSubtotal.inputTokens : null,
    outputTokens: complete ? measuredTokenSubtotal.outputTokens : null,
    measuredTokenSubtotal,
    classifierReferenceRateEstimate: complete
      ? measuredReferenceSubtotal
      : null,
    measuredReferenceSubtotal,
    downstreamCost: null,
    billedCost: null,
  }
}
process.stdout.write(
  JSON.stringify(
    {
      evidence:
        "Recorded live operator runs; synthetic diagnostic corpus, not population accuracy or security proof. Dated reference prices, not invoices; downstream usage unavailable.",
      folder: fileURLToPath(folder),
      nodeVersion: process.version,
      runs,
      workflows,
      usage: { jev: usage("jev"), openai: usage("openai") },
    },
    null,
    2,
  ) + "\n",
)
