// Offline summaries of production-adapter evidence; never makes provider calls.
import { readFileSync } from "node:fs"
import { pathToFileURL } from "node:url"

export function summarizeIntegration(run, corpus) {
  const seen = new Set()
  const rows = run.records.map(record => {
    const item = corpus.find(item => item.id === record.id)
    if (!item || seen.has(item.id) || !Number.isFinite(record.durationMs) || record.durationMs < 0)
      throw new Error("Unknown, duplicate or invalid integration record")
    seen.add(item.id)
    const outcome = record.decision?.outcome?.kind
    if (!["accept", "request_information", "request_rephrasing", "reject_attack", "irrelevant", "unusable", "service_failure"].includes(outcome))
      throw new Error("Invalid field decision")
    return {id:item.id,field:item.field,language:item.language,expected:item.expected,outcome,correct:outcome===item.expected,durationMs:record.durationMs}
  })
  const classified = rows.filter(row => row.outcome !== "service_failure")
  const times = rows.map(row => row.durationMs).sort((a,b)=>a-b)
  const percentile = fraction => times.length ? times[Math.ceil(times.length*fraction)-1] : null
  return {
    evidence:run.scope,
    attempted:rows.length, classified:classified.length,
    correct:classified.filter(row=>row.correct).length,
    classificationAccuracy:classified.length ? classified.filter(row=>row.correct).length/classified.length : null,
    coverage:corpus.length ? rows.length/corpus.length : null,
    missingIds:corpus.filter(item=>!seen.has(item.id)).map(item=>item.id),
    serviceFailures:rows.length-classified.length,
    falseRejections:rows.filter(row=>row.expected==="accept" && row.outcome!=="accept" && row.outcome!=="service_failure").length,
    uncertainCases:rows.filter(row=>row.expected==="request_rephrasing").length,
    uncertainCorrect:rows.filter(row=>row.expected==="request_rephrasing" && row.correct).length,
    attacks:rows.filter(row=>row.expected==="reject_attack").length,
    attacksBlocked:rows.filter(row=>row.expected==="reject_attack" && row.correct).length,
    p50Ms:percentile(0.5),p95Ms:percentile(0.95),maxMs:times.at(-1)??null,
    workflows:run.workflows ?? [], rows,
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const corpus=JSON.parse(readFileSync(new URL("../../docs/evaluations/field-validation-integration-cases.json",import.meta.url),"utf8"))
  const run=JSON.parse(readFileSync(new URL("../../docs/evaluations/field-validation-integration-corrected-live.json",import.meta.url),"utf8"))
  process.stdout.write(JSON.stringify(summarizeIntegration(run,corpus),null,2)+"\n")
}
