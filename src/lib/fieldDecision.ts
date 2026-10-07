/** Evaluation contract v1. No provider call or production workflow integration. */
export type ValidationField = "professional_information" | "job_posting"
export type ContentSignal = "professional_fact" | "job_with_context" | "job_title_only" | "relevant_but_insufficient" | "irrelevant" | "unusable"
export type AttackSignal = "none" | "uncertain" | "detected"

/** Semantic assertions from an adapter, never proof that input is safe or true. */
export interface FieldSignals {
  content: ContentSignal
  attack: AttackSignal
}
export type ServiceFailure = "key" | "rate_limit" | "timeout" | "outage" | "invalid_output"
export type FieldOutcome = { kind: "accept" } | {
  kind: "request_information"
  needs: "professional_fact" | "responsibilities_or_qualifications"
} | { kind: "request_rephrasing" } | { kind: "reject_attack" } | {
  kind: "irrelevant"
} | { kind: "unusable" } | { kind: "service_failure"; reason: ServiceFailure }
export interface FieldDecision {
  version: 1
  field: ValidationField
  outcome: FieldOutcome
}
export type ClassificationResult = { kind: "signals"; signals: unknown } | {
  kind: "failure"
  reason: ServiceFailure
}

const contents: readonly ContentSignal[] = [
  "professional_fact",
  "job_with_context",
  "job_title_only",
  "relevant_but_insufficient",
  "irrelevant",
  "unusable",
]
const attacks: readonly AttackSignal[] = ["none", "uncertain", "detected"]

/** Fail closed on malformed or field-incompatible adapter output. */
export function parseFieldSignals(
  field: ValidationField,
  raw: unknown,
): FieldSignals | null {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return null
  const value = raw as Record<string, unknown>
  if (
    Object.keys(value).length !== 2 ||
    !contents.includes(value.content as ContentSignal) ||
    !attacks.includes(value.attack as AttackSignal)
  )
    return null
  if (
    field === "professional_information" &&
    (value.content === "job_with_context" || value.content === "job_title_only")
  )
    return null
  if (field === "job_posting" && value.content === "professional_fact")
    return null
  return {
    content: value.content as ContentSignal,
    attack: value.attack as AttackSignal,
  }
}

/** Policy precedence applies to the entire submission, including its useful parts. */
export function decideField(
  field: ValidationField,
  result: ClassificationResult,
): FieldDecision {
  const decision = (outcome: FieldOutcome): FieldDecision => ({
    version: 1,
    field,
    outcome,
  })
  if (result.kind === "failure")
    return decision({ kind: "service_failure", reason: result.reason })
  const signals = parseFieldSignals(field, result.signals)
  if (!signals)
    return decision({ kind: "service_failure", reason: "invalid_output" })
  if (signals.attack === "detected") return decision({ kind: "reject_attack" })
  if (signals.attack === "uncertain")
    return decision({ kind: "request_rephrasing" })
  if (signals.content === "unusable" || signals.content === "irrelevant") {
    return decision({ kind: signals.content })
  }
  if (
    signals.content === "relevant_but_insufficient" ||
    signals.content === "job_title_only"
  ) {
    return decision({
      kind: "request_information",
      needs:
        field === "job_posting"
          ? "responsibilities_or_qualifications"
          : "professional_fact",
    })
  }
  return decision({ kind: "accept" })
}
