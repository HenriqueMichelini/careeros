import {
  editProfile,
  validateProfileDocument,
  type ProfileDocument,
  type ProfileFact,
} from "./profileDocument"
import type { Locale } from "./i18n"

export interface SectionReviewRequest {
  document: ProfileDocument
  section: string
  locale: Locale
}
export interface SectionPatch {
  factId: string
  revision: number
  wording: string
  supporting: { id: string; revision: number }[]
}
export interface SectionProposal {
  profileId: string
  revision: number
  section: string
  summary: string
  patches: SectionPatch[]
}
const fields: Record<string, string[]> = {
  goals: ["careerGoals"],
  skills: ["skills", "competencies", "tools"],
  experience: ["description", "responsibilities", "achievements"],
  projects: ["description", "technologies", "highlights"],
  compensation: ["employmentStatus", "currentSalary", "desiredSalary"],
  other: ["additionalInfo"],
}
export function reviewableFact(
  doc: ProfileDocument,
  section: string,
  fact: ProfileFact,
): boolean {
  const owner = doc.entities.find((e) => e.id === fact.owner.id)
  return (
    !!fields[section]?.includes(fact.field) &&
    typeof fact.value === "string" &&
    !!fact.value.trim() &&
    (["experience", "projects"].includes(section)
      ? owner?.kind === section
      : fact.owner.id === doc.id)
  )
}
export function sectionReviewRequest(
  doc: ProfileDocument,
  section: string,
  locale: Locale,
): SectionReviewRequest {
  if (!validateProfileDocument(doc) || !fields[section])
    throw new Error("input")
  const selected = doc.facts.filter((f) => reviewableFact(doc, section, f))
  if (!selected.length || selected.length > 80) throw new Error("input")
  const owners = new Set(
    selected.flatMap((f) => [f.owner.id, ...f.context.map((c) => c.id)]),
  )
  for (const link of doc.links.filter(
    (l) =>
      l.state === "active" &&
      selected.some((f) => f.id === l.from.id) &&
      l.kind !== "supports",
  ))
    owners.add(link.to.id)
  // Context identities are read-only. No unrelated Profile-level fields travel.
  const facts = doc.facts.filter(
    (f) =>
      selected.includes(f) ||
      (f.owner.id !== doc.id &&
        owners.has(f.owner.id) &&
        ![
          "description",
          "responsibilities",
          "achievements",
          "highlights",
          "details",
        ].includes(f.field)),
  )
  const factIds = new Set(facts.map((f) => f.id))
  const links = doc.links.filter(
    (l) => l.state === "active" && factIds.has(l.from.id),
  )
  for (const f of facts) for (const context of f.context) owners.add(context.id)
  for (const l of links) if (l.kind !== "supports") owners.add(l.to.id)
  const evidenceIds = new Set(
    links.filter((l) => l.kind === "supports").map((l) => l.to.id),
  )
  const document = structuredClone({
    ...doc,
    facts,
    entities: doc.entities.filter((e) => owners.has(e.id)),
    links,
    evidence: doc.evidence.filter((e) => evidenceIds.has(e.id)),
  })
  if (
    !validateProfileDocument(document) ||
    new TextEncoder().encode(JSON.stringify(document)).length > 48_000
  )
    throw new Error("input")
  return { document, section, locale }
}
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b)
export function validateSectionProposal(
  request: SectionReviewRequest,
  value: unknown,
): value is SectionProposal {
  if (!value || typeof value !== "object") return false
  const p = value as SectionProposal
  const doc = request.document
  const targets = doc.facts.filter((f) =>
    reviewableFact(doc, request.section, f),
  )
  if (
    Object.keys(p).sort().join() !==
      "patches,profileId,revision,section,summary" ||
    p.profileId !== doc.id ||
    p.revision !== doc.revision ||
    p.section !== request.section ||
    typeof p.summary !== "string" ||
    !p.summary.trim() ||
    p.summary.length > 1000 ||
    !Array.isArray(p.patches) ||
    p.patches.length !== targets.length
  )
    return false
  const seen = new Set<string>()
  return p.patches.every((patch) => {
    if (
      !patch ||
      typeof patch !== "object" ||
      Object.keys(patch).sort().join() !== "factId,revision,supporting,wording"
    )
      return false
    const target = targets.find(
      (f) => f.id === patch.factId && f.revision === patch.revision,
    )
    if (
      !target ||
      seen.has(target.id) ||
      typeof patch.wording !== "string" ||
      !patch.wording.trim() ||
      patch.wording.length > 12000 ||
      !Array.isArray(patch.supporting) ||
      !patch.supporting.length ||
      patch.supporting.length > 80
    )
      return false
    seen.add(target.id)
    const refs = new Set<string>()
    const sourceText = patch.supporting.map(r => targets.find(f => f.id === r.id)?.value ?? "").join("\n")
    const numbers: string[] = sourceText.match(/[0-9]+(?:[.,][0-9]+)*(?:%|[kKmM])?/g) ?? []
    if ((patch.wording.match(/[0-9]+(?:[.,][0-9]+)*(?:%|[kKmM])?/g) ?? []).some(n => !numbers.includes(n))) return false
    return (
      patch.supporting.some((r) => r.id === target.id) &&
      patch.supporting.every((r) => {
        if (
          !r ||
          Object.keys(r).sort().join() !== "id,revision" ||
          refs.has(r.id)
        )
          return false
        refs.add(r.id)
        const source = targets.find(
          (f) => f.id === r.id && f.revision === r.revision,
        )
        return (
          !!source &&
          source.owner.id === target.owner.id &&
          same(source.context, target.context) &&
          source.assertion === target.assertion &&
          source.intent === target.intent &&
          source.certainty === target.certainty &&
          same(source.temporal, target.temporal)
        )
      })
    )
  })
}
// Only explicit apply calls this boundary. User-edited wording receives user
// authorship and invalidates excerpt support through the canonical edit command.
export function applySectionProposal(
  doc: ProfileDocument,
  request: SectionReviewRequest,
  proposal: SectionProposal,
  edits: Record<string, string>,
  removals: string[],
): ProfileDocument {
  if (
    !validateSectionProposal(request, proposal) ||
    doc.id !== request.document.id ||
    doc.revision !== request.document.revision ||
    !same(sectionReviewRequest(doc, request.section, request.locale), request)
  )
    throw new Error("stale")
  if (
    Object.keys(edits).some(
      (id) => !proposal.patches.some((p) => p.factId === id),
    ) ||
    removals.some((id) => !proposal.patches.some((p) => p.factId === id))
  )
    throw new Error("input")
  let next = structuredClone(doc)
  for (const patch of proposal.patches) {
    const wording = edits[patch.factId] ?? patch.wording
    if (!wording.trim() || wording.length > 12000) throw new Error("input")
    if (removals.includes(patch.factId) || wording !== patch.wording) continue
    const fact = next.facts.find((f) => f.id === patch.factId)!
    if (fact.value === wording) continue
    const support = doc.links.filter(
      (l) =>
        l.kind === "supports" &&
        l.state === "active" &&
        patch.supporting.some(
          (r) => r.id === l.from.id && r.revision === l.from.revision,
        ),
    )
    fact.value = wording
    fact.revision++
    fact.origin = { kind: "ai_review", original: "unknown" }
    fact.approval = "approved"
    fact.normalization = {
      observed: wording,
      canonical: null,
      policy: next.normalizationPolicy,
    }
    for (const link of next.links.filter(
      (l) => l.from.id === fact.id && l.state === "active",
    ))
      link.from.revision = fact.revision
    for (const link of support)
      if (
        !next.links.some(
          (l) =>
            l.state === "active" &&
            l.kind === "supports" &&
            l.from.id === fact.id &&
            l.to.id === link.to.id,
        )
      )
        next.links.push({
          ...structuredClone(link),
          id: crypto.randomUUID(),
          from: { profileId: next.id, id: fact.id, revision: fact.revision },
        })
    fact.support =
      support.length &&
      patch.supporting.every(
        (r) => doc.facts.find((f) => f.id === r.id)?.support === "supported",
      )
        ? "supported"
        : fact.support === "supported"
          ? "invalidated"
          : fact.support
  }
  // Apply user corrections/removals last so patch order cannot resurrect
  // excerpt support withdrawn by another fact in the same acceptance.
  for (const patch of proposal.patches) {
    if (removals.includes(patch.factId)) {
      next = editProfile(next, { type: "remove_fact", id: patch.factId })
    } else if (edits[patch.factId] !== undefined && edits[patch.factId] !== patch.wording) {
      next = editProfile(next, { type: "fact", id: patch.factId, patch: {
        value: edits[patch.factId], assertion: "unknown", intent: "unknown", certainty: "unknown",
        temporal: { wording: "", precision: "unknown" },
      } })
    }
  }
  // One atomic Profile revision regardless of the number of edited facts.
  next.revision = doc.revision + 1
  for (const f of next.facts) {
    if (f.owner.id === next.id) f.owner.revision = next.revision
    for (const c of f.context) if (c.id === next.id) c.revision = next.revision
  }
  if (!validateProfileDocument(next)) throw new Error("input")
  return next
}

export class SectionReviewError extends Error {
  constructor(public code: string) {
    super(code)
  }
}
export async function requestSectionProposal(
  request: SectionReviewRequest,
  apiKey: string,
  signal?: AbortSignal,
): Promise<SectionProposal> {
  let response: Response
  try {
    response = await fetch("/api/profile/review", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-OpenAI-Api-Key": apiKey,
      },
      body: JSON.stringify(request),
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(30_000)])
        : AbortSignal.timeout(30_000),
    })
  } catch (error) {
    throw new SectionReviewError(
      error instanceof DOMException && error.name === "TimeoutError"
        ? "timeout"
        : "outage",
    )
  }
  const value = await response.json().catch(() => null)
  if (!response.ok)
    throw new SectionReviewError(
      [
        "input",
        "key",
        "rate_limit",
        "outage",
        "timeout",
        "invalid_output",
        "refused",
        "truncated",
      ].includes(value?.error)
        ? value.error
        : "outage",
    )
  if (!validateSectionProposal(request, value))
    throw new SectionReviewError("invalid_output")
  return value
}
