import type { ProfileDocument } from "./profileDocument"

import fields from "../../internal/qualificationmatching/fields.json"
// A transient, valid Profile subdocument. Approved professional facts and existing migrated blocks with unknown provenance and
// their active accepted evidence leave the browser; no saved Profile is changed.
export function qualificationProjection(doc: ProfileDocument): ProfileDocument {
  const facts = doc.facts.filter((f) => {
    const kind =
      doc.entities.find((e) => e.id === f.owner.id)?.kind ?? "profile"
    return (
      (f.approval === "approved" ||
        (f.kind === "legacy_block" && f.origin.kind === "existing_profile")) &&
      (fields as Record<string, string[]>)[kind]?.includes(f.field) &&
      (typeof f.value === "string"
        ? !!f.value.trim()
        : typeof f.value === "boolean")
    )
  })
  const ids = new Set(facts.map((f) => f.id))
  const links = doc.links.filter(
    (l) => l.state === "active" && ids.has(l.from.id),
  )
  const evidenceIds = new Set(
    links.filter((l) => l.kind === "supports").map((l) => l.to.id),
  )
  return structuredClone({
    ...doc,
    facts,
    links,
    evidence: doc.evidence.filter((e) => evidenceIds.has(e.id)),
  })
}
export function hasQualificationEvidence(doc: ProfileDocument | null): boolean {
  return (
    !!doc &&
    qualificationProjection(doc).facts.some((f) =>
      typeof f.value === "string" ? !!f.value.trim() : f.value === true,
    )
  )
}

export interface QualificationFact {
  id: string
  revision: number
  owner: {
    profileId: string
    id: string
    revision: number
  }
  context: {
    profileId: string
    id: string
    revision: number
  }[]
  field: string
  value: string | boolean
  section: string
  assertion: string
  intent: string
  certainty: string
  temporal: {
    wording: string
    precision: string
  }
  evidence: {
    id: string
    revision: number
    excerpt: string
  }[]
}
export interface RequirementMatch {
  requirementIndex: number
  state: "supported" | "partially_supported" | "not_evidenced" | "needs_clarification"
  factIds: string[]
  explanation: string
  question: string
  requirement: import("./jobContext").JobItem
  facts: QualificationFact[]
  complete: boolean
  excluded: number
}
function matchesProfileFact(
  fact: QualificationFact,
  doc: ProfileDocument,
): boolean {
  const original = doc.facts.find(
    (f) => f.id === fact.id && f.revision === fact.revision,
  )
  return !(
    !original ||
    (original.approval !== "approved" &&
      !(
        original.kind === "legacy_block" &&
        original.origin.kind === "existing_profile"
      )) ||
    JSON.stringify(original.value) !== JSON.stringify(fact.value) ||
    JSON.stringify(original.owner) !== JSON.stringify(fact.owner) ||
    JSON.stringify(original.context) !== JSON.stringify(fact.context) ||
    JSON.stringify(original.temporal) !== JSON.stringify(fact.temporal) ||
    original.field !== fact.field ||
    original.assertion !== fact.assertion ||
    original.intent !== fact.intent ||
    original.certainty !== fact.certainty ||
    !Array.isArray(fact.evidence) ||
    fact.evidence.some(
      (e) =>
        !doc.evidence.some(
          (o) =>
            o.id === e.id &&
            o.revision === e.revision &&
            o.excerpt === e.excerpt,
        ),
    )
  )
}

export function parseRequirementMatches(
  value: unknown,
  doc: ProfileDocument,
  requirements: import("./jobContext").JobItem[],
): RequirementMatch[] | null {
  if (!Array.isArray(value) || value.length !== requirements.length) return null
  const seen = new Set<number>()
  for (const m of value as RequirementMatch[]) {
    if (
      !m ||
      !Number.isInteger(m.requirementIndex) ||
      !requirements[m.requirementIndex] ||
      seen.has(m.requirementIndex) ||
      ![
        "supported",
        "partially_supported",
        "not_evidenced",
        "needs_clarification",
      ].includes(m.state) ||
      typeof m.explanation !== "string" ||
      !m.explanation.trim() ||
      m.explanation.length > 1500 ||
      typeof m.question !== "string" ||
      m.question.length > 500 ||
      (m.state === "needs_clarification" && !m.question.trim()) ||
      typeof m.complete !== "boolean" ||
      !Number.isInteger(m.excluded) ||
      m.excluded < 0 ||
      m.complete !== (m.excluded === 0) ||
      (!m.complete && m.state === "not_evidenced") ||
      JSON.stringify(m.requirement) !==
        JSON.stringify(requirements[m.requirementIndex]) ||
      !Array.isArray(m.facts) ||
      m.facts.length > 500 ||
      !Array.isArray(m.factIds) ||
      m.factIds.length > 100
    )
      return null
    seen.add(m.requirementIndex)
    const ids = new Set<string>()
    for (const fact of m.facts) {
      if (ids.has(fact.id) || !matchesProfileFact(fact, doc)) return null
      ids.add(fact.id)
    }
    if (
      new Set(m.factIds).size !== m.factIds.length ||
      m.factIds.some((id) => !ids.has(id)) ||
      (["supported", "partially_supported"].includes(m.state) &&
        !m.factIds.length)
    )
      return null
  }
  return value as RequirementMatch[]
}
