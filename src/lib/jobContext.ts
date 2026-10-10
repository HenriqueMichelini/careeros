export const JOB_CONTEXT_VERSION =
  "job-context-v1/prompt-v1/schema-v1/gpt-6-luna/none/jev-rubric-v1/policy-v1"
export interface OriginalExcerpt {
  start: number
  end: number
  text: string
}
export interface JobEvidence {
  segmentId: string
  quote: string
  occurrence: number
  original: OriginalExcerpt[]
}
export interface JobItem {
  source: JobEvidence
  importance: "required" | "preferred" | "unspecified"
}
export interface JobContext {
  version: string
  sourceId: string
  inputsId: string
  job: {
    jobTitle: JobEvidence | null
    company: JobEvidence | null
    seniority: JobEvidence | null
    location: JobEvidence | null
    responsibilities: JobItem[]
    qualifications: JobItem[]
    applicationRequirements: JobItem[]
  }
}

// Client checks support inspection and recovery only. The server revalidates all
// references and versions after its whole-original field gate on every draft.
export function parseJobContext(
  value: unknown,
  original: string,
): JobContext | null {
  if (!value || typeof value !== "object") return null
  const context = value as JobContext
  if (
    context.version !== JOB_CONTEXT_VERSION ||
    !/^[a-f0-9]{64}$/.test(context.sourceId) ||
    !/^[a-f0-9]{64}$/.test(context.inputsId) ||
    !context.job ||
    typeof context.job !== "object"
  )
    return null
  const bytes = new TextEncoder().encode(original)
  function evidence(ref: JobEvidence): boolean {
    if (
      !ref ||
      typeof ref !== "object" ||
      typeof ref.segmentId !== "string" ||
      !ref.segmentId ||
      typeof ref.quote !== "string" ||
      !ref.quote.trim() ||
      !Number.isInteger(ref.occurrence) ||
      ref.occurrence < 0 ||
      !Array.isArray(ref.original) ||
      !ref.original.length
    )
      return false
    let previous = -1
    return ref.original.every((range) => {
      if (
        !range ||
        !Number.isInteger(range.start) ||
        !Number.isInteger(range.end) ||
        range.start < 0 ||
        range.start < previous ||
        range.end <= range.start ||
        range.end > bytes.length ||
        typeof range.text !== "string"
      )
        return false
      previous = range.end
      try {
        return (
          new TextDecoder("utf-8", { fatal: true }).decode(
            bytes.slice(range.start, range.end),
          ) === range.text
        )
      } catch {
        return false
      }
    })
  }
  for (const name of [
    "jobTitle",
    "company",
    "seniority",
    "location",
  ] as const) {
    const ref = context.job[name]
    if (ref !== null && !evidence(ref)) return null
  }
  for (const name of [
    "responsibilities",
    "qualifications",
    "applicationRequirements",
  ] as const) {
    const items = context.job[name]
    if (
      !Array.isArray(items) ||
      items.length > 100 ||
      items.some(
        (item) =>
          !item ||
          !["required", "preferred", "unspecified"].includes(item.importance) ||
          !evidence(item.source),
      )
    )
      return null
  }
  return context
}
