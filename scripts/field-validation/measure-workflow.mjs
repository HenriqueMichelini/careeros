// Controlled local HTTP workflow probe, not a deployed/browser measurement.
import { pathToFileURL } from "node:url"
import { performance } from "node:perf_hooks"
import { collect } from "./collect.mjs"
import { collectVariant } from "./experiment.mjs"
import { lookupCase } from "./evaluate.mjs"
import { decideField, careerProfile, cvQualifications } from "./contract.mjs"
import { evaluationKey } from "./credentials.mjs"

export function buildWorkflowPayload(kind, item) {
  const profile = {
    fullName: "",
    email: "",
    phone: "",
    location: "",
    professionalLinks: "",
    careerGoals: "",
    skills: "Java",
    competencies: "",
    tools: "",
    experience: [],
    projects: [],
    employmentStatus: "looking",
    currentSalary: "",
    desiredSalary: "",
    additionalInfo: "",
    education: [],
    certifications: [],
    languages: [],
  }
  return kind === "profile_ingestion"
    ? { input: item.text, profile }
    : {
        repository: careerProfile(profile),
        jobPosting: item.text,
        qualifications: cvQualifications(profile),
        ...(kind === "application_draft"
          ? {
              confirmedQualifications: [],
              cvLanguage: item.language === "pt" ? "pt-BR" : "en",
            }
          : {}),
      }
}

async function main() {
  const [provider, caseId, kind, origin = "http://127.0.0.1:8787", variant] =
    process.argv.slice(2)
  if (!["openai", "jev"].includes(provider))
    throw new Error("Select openai or jev explicitly")
  if (variant && !["explicit", "atomic"].includes(variant))
    throw new Error("Select a frozen evaluation variant")
  const item = lookupCase(caseId)
  const configs = {
    profile_ingestion: { path: "/api/profile/ingest", bound: 55000 },
    qualification_gaps: { path: "/api/qualification-gaps", bound: 30000 },
    application_draft: { path: "/api/application-draft", bound: 30000 },
  }
  const config = configs[kind]
  const url = new URL(origin)
  if (
    !["127.0.0.1", "localhost"].includes(url.hostname) ||
    url.protocol !== "http:" ||
    url.username ||
    url.password
  )
    throw new Error("Use a local loopback HTTP origin only")
  if (
    !config ||
    !item ||
    item.expected !== "accept" ||
    (item.field === "professional_information") !==
      (kind === "profile_ingestion")
  )
    throw new Error("Select an accepted synthetic case and matching workflow")
  const providerKey = evaluationKey(provider),
    openaiKey = evaluationKey("openai")
  if (!providerKey || !openaiKey)
    throw new Error("Both classifier and workflow credentials are required")
  const payload = buildWorkflowPayload(kind, item)
  async function downstream(timeout) {
    try {
      const response = await fetch(new URL(config.path, url), {
        method: "POST",
        signal: AbortSignal.timeout(Math.max(1, Math.floor(timeout))),
        headers: {
          "Content-Type": "application/json",
          "X-OpenAI-Api-Key": openaiKey,
        },
        body: JSON.stringify(payload),
      })
      await response.arrayBuffer() // include complete response consumption; never log the body
      return { status: response.status, ok: response.ok }
    } catch (error) {
      return {
        ok: false,
        error: error?.name === "TimeoutError" ? "timeout" : "outage",
      }
    }
  }
  const baselineStart = performance.now()
  const baseline = await downstream(config.bound)
  const baselineMs = performance.now() - baselineStart
  if (!baseline.ok) {
    process.stdout.write(
      `${JSON.stringify({ provider, variant, caseId, kind, baseline, baselineMs, comparison: "unavailable: baseline failed" }, null, 2)}\n`,
    )
  } else {
    const start = performance.now()
    const run = variant
      ? await collectVariant(provider, variant, [item], providerKey)
      : await collect(provider, [caseId], providerKey)
    const record = run.records[0]
    const decision = decideField(item.field, record.result)
    const validated =
      decision.outcome.kind === "accept"
        ? await downstream(config.bound - (performance.now() - start))
        : { ok: false, error: "classification_stopped_workflow" }
    const validatedTotalMs = performance.now() - start
    // Only successful paired runs enter latency comparison; failures remain explicit metadata.
    if (validated.ok) record.workflow = { kind, baselineMs, validatedTotalMs }
    record.workflowAttempt = {
      kind,
      baseline,
      validated,
      baselineMs,
      validatedTotalMs,
    }
    run.measurementScope =
      "local HTTP probe with browser-equivalent deadline; not live UI or deployment"
    process.stdout.write(`${JSON.stringify(run, null, 2)}\n`)
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href)
  await main()
