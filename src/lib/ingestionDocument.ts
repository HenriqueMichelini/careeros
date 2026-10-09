import {
  applyIngestion,
  beforeValue,
  IngestionError,
  validateIngestionResult,
  type IngestionOperation,
  type IngestionResult,
} from "./ingestion"
import {
  normalizeTerm,
  profileView,
  replaceProfileView,
  validateProfileDocument,
  type ProfileDocument,
} from "./profileDocument"

// Pointers establish traceability, never semantic truth. Only explicit apply
// enters this transaction; pending claims and the complete paste are transient.
export function applyIngestionDocument(
  doc: ProfileDocument,
  snapshot: ProfileDocument,
  input: string,
  result: IngestionResult,
): ProfileDocument {
  if (
    !validateProfileDocument(doc) ||
    JSON.stringify(doc) !== JSON.stringify(snapshot)
  )
    throw new IngestionError("stale")
  const view = profileView(doc)
  validateIngestionResult(
    {
      ...result,
      operations: result.operations.map(
        ({ approved: _approved, proposedValue, ...op }) => ({
          ...op,
          value: proposedValue ?? op.value,
        }),
      ),
    },
    input,
    view,
  )
  const approved = result.operations.filter((o) => o.approved)
  for (const op of approved) {
    const ids = op.supportingClaimIds ?? [op.claimId]
    const source =
      ids
        .flatMap((id) => {
          const claim = result.claims.find((c) => c.id === id)
          return claim
            ? [
                claim.source,
                ...(claim.supportingSources ?? []).map((s) => s.source),
              ]
            : []
        })
        .join("\n") +
      "\n" +
      beforeValue(view, op)
    if (!protectedSupported(op.proposedValue ?? op.value, source))
      throw new IngestionError("invalid_output")
  }
  const updated = applyIngestion(view, JSON.stringify(view), result.operations)
  const next = replaceProfileView(doc, updated, {
    kind: "accepted_proposal",
    original: "unknown",
  })
  const ref = (id: string, revision: number) => ({
    profileId: next.id,
    id,
    revision,
  })
  const newOwners = new Map<string, string>()
  for (const op of approved) {
    if (!op.entryId.startsWith("new:")) continue
    const key = op.target + "/" + op.entryId
    if (newOwners.has(key)) continue
    const entity = next.entities.find(
      (e) =>
        e.kind === op.target &&
        !doc.entities.some((old) => old.id === e.id) &&
        ![...newOwners.values()].includes(e.id),
    )
    if (!entity) throw new IngestionError("incomplete")
    newOwners.set(key, entity.id)
  }
  const groups = new Map<string, typeof approved>()
  for (const op of approved) {
    const owner = op.entryId
      ? (newOwners.get(op.target + "/" + op.entryId) ??
        next.entities.find(
          (e) => e.kind === op.target && e.legacyId === op.entryId,
        )?.id)
      : next.id
    if (!owner) throw new IngestionError("invalid_output")
    const key = owner + "/" + op.field
    groups.set(key, [...(groups.get(key) ?? []), op])
  }
  for (const [key, ops] of groups) {
    const last = ops.at(-1)!
    const fact = next.facts.find((f) => f.owner.id + "/" + f.field === key)!
    const old = doc.facts.find((f) => f.id === fact.id)
    if (old && JSON.stringify(old.value) === JSON.stringify(fact.value))
      continue
    // Every edit is user-authored; it cannot inherit an AI support claim.
    if (
      ops.some(
        (op) => op.proposedValue !== undefined && op.value !== op.proposedValue,
      )
    ) {
      fact.origin = { kind: "manual_edit", original: "user" }
      continue
    }
    if (last.action === "remove") continue
    let start = 0
    ops.forEach((op, index) => {
      if (op.action !== "add") start = index
    })
    const relevant = ops.slice(start)
    const claims = new Map<string, IngestionResult["claims"][number]>()
    for (const op of relevant) {
      const ids = op.supportingClaimIds ?? [op.claimId]
      for (const id of ids) {
        const claim = result.claims.find((c) => c.id === id)
        if (
          !claim ||
          !claim.targets.includes(op.target) ||
          !approved.some((o) => o.claimId === id && o.action !== "remove")
        )
          throw new IngestionError("invalid_output")
        if (
          op.entryId &&
          result.operations.some(
            (other) =>
              other.claimId === id &&
              other.target === op.target &&
              other.entryId !== op.entryId,
          )
        )
          throw new IngestionError("invalid_output")
        claims.set(id, claim)
      }
    }
    // Supporting identity claims supply context, not the narrative's semantic
    // qualifiers. Mixed narrative qualifiers stay unknown rather than forcing
    // one assertion onto the entire composite field.
    const meanings = relevant.map(op => result.claims.find(c => c.id === op.claimId)?.meaning)
    const meaning = meanings.every(m => JSON.stringify(m) === JSON.stringify(meanings[0])) ? meanings[0] : undefined
    const appending =
      relevant.every((op) => op.action === "add") && !!old?.value
    fact.kind = appending ? "legacy_block" : "statement"
    fact.normalization =
      fact.kind === "statement" && relevant.length === 1
        ? ingestionNormalization(relevant[0], result)
        : {
            observed: String(fact.value),
            canonical: null,
            policy: next.normalizationPolicy,
          }
    if (meaning && !appending) {
      // Claim qualifiers describe narrative meaning; a negative technology
      // claim does not negate its employer/name identity fields.
      if (
        ["startDate", "endDate", "graduationDate", "date"].includes(fact.field)
      )
        fact.temporal = structuredClone(meaning.temporal)
      else if (
        ![
          "company",
          "title",
          "name",
          "degree",
          "institution",
          "issuer",
          "location",
          "url",
          "credentialId",
          "current",
          "fullName",
          "email",
          "phone",
          "professionalLinks",
        ].includes(fact.field)
      )
        Object.assign(fact, structuredClone(meaning))
    }
    const owner = next.entities.find((e) => e.id === fact.owner.id)
    const identityFields = owner?.kind === "experience" ? ["company","title","startDate","endDate","current"] : owner?.kind === "projects" ? ["name"] : []
    const identityUnchanged = identityFields.every(field => JSON.stringify(doc.facts.find(f => f.owner.id === owner?.id && f.field === field)?.value) === JSON.stringify(next.facts.find(f => f.owner.id === owner?.id && f.field === field)?.value))
    const contextUnchanged = (old?.context ?? []).every(c => c.id === owner?.id || next.entities.some(e => e.id === c.id && e.revision === c.revision))
    const retainsOld = !!old?.value && retainsWording(String(fact.value),String(old.value))
    const preserveOldSupport = appending && retainsOld && old?.support === "supported" && identityUnchanged && contextUnchanged
    if (old?.value && preserveOldSupport) {
      for (const link of doc.links.filter(
        (l) =>
          l.from.id === old.id && l.kind === "supports" && l.state === "active",
      ))
        next.links.push({
          ...structuredClone(link),
          id: crypto.randomUUID(),
          from: ref(fact.id, fact.revision),
        })
    }
    const excerpts = new Set<string>()
    for (const claim of claims.values()) {
      for (const source of [
        { source: claim.source, sourceReference: claim.sourceReference },
        ...(claim.supportingSources ?? []),
      ]) {
        // Do not persist source IDs derived from the full paste; the retained
        // excerpt itself and its accepted origin are sufficient for inspection.
        if (excerpts.has(source.source)) continue
        excerpts.add(source.source)
        const evidence = {
          id: crypto.randomUUID(),
          revision: 1,
          excerpt: source.source,
          origin: "professional_information",
          approval: "approved" as const,
        }
        next.evidence.push(evidence)
        next.links.push({
          id: crypto.randomUUID(),
          kind: "supports",
          state: "active",
          from: ref(fact.id, fact.revision),
          to: ref(evidence.id, 1),
        })
      }
    }
    // Existing unsupported wording preserved by a composite replacement must
    // not be upgraded merely because its new clause has an exact pointer.
    const sourceText = [...excerpts].join("\n")
    fact.support =
      retainsOld &&
      !preserveOldSupport &&
      !sourceText.includes(String(old?.value))
        ? "unsupported"
        : "supported"
    if (owner && ["experience", "projects"].includes(owner.kind)) {
      fact.context = [ref(owner.id, owner.revision)]
      const kinds =
        owner.kind === "experience"
          ? next.facts.some(f => f.owner.id === owner.id && ["startDate","endDate","current"].includes(f.field) && !!f.value)
            ? ["role_context", "period_context"] as const : ["role_context"] as const
          : ["project_context"] as const
      for (const kind of kinds)
        next.links.push({
          id: crypto.randomUUID(),
          kind,
          state: "active",
          from: ref(fact.id, fact.revision),
          to: ref(owner.id, owner.revision),
        })
    }
  }
  if (!validateProfileDocument(next)) throw new IngestionError("invalid_output")
  return next
}

// Keep the bounded literal policy aligned with backend/profile-ingestion/evidence.go.
// Passing this guard is not evidence of semantic truth.
function protectedSupported(value: string, source: string): boolean {
  const aliases: Record<string, string> = {
    Javascript: "JavaScript",
    Typescript: "TypeScript",
  }
  const words = new Set(
    (source.match(/[\p{L}\p{N}_+#]+(?:[.,][0-9]+)*(?:%|[kKmM])?/gu) ?? []).map(
      (w) => aliases[w] ?? w.replace(/^\+(?=\d)/, ""),
    ),
  )
  const tokens =
    value.match(
      /\b(?:[0-9]+(?:[.,][0-9]+)*(?:%|[kKmM])?|(?:JavaScript|Javascript|TypeScript|Typescript|Java|Python|Go|Rust|Ruby|React|Angular|Vue|Node|PostgreSQL|MySQL|SQL|AWS|Azure|Docker|Kubernetes|Spring|Boot|[A-Z]{2,}[A-Za-z0-9+#]*|[A-Z][a-z]+[A-Z][A-Za-z0-9]*)\b)/g,
    ) ?? []
  return tokens.every((t) => words.has(aliases[t] ?? t))
}

export function ingestionNormalization(
  op: IngestionOperation,
  result: IngestionResult,
) {
  const value = op.proposedValue ?? op.value
  const proposed = normalizeTerm(value)
  const claims = (op.supportingClaimIds ?? [op.claimId]).map(
    (id) => result.claims.find((c) => c.id === id)!,
  )
  const source = claims
    .flatMap((c) =>
      c ? [c.source, ...(c.supportingSources ?? []).map((s) => s.source)] : [],
    )
    .join("\n")
  if (proposed.canonical) {
    const observed = (source.match(/[\p{L}\p{N}_+#]+/gu) ?? []).find(
      (word) => normalizeTerm(word).canonical === proposed.canonical,
    )
    if (observed) return normalizeTerm(observed)
  }
  return { observed: source, canonical: null, policy: proposed.policy }
}

function retainsWording(value:string, prior:string):boolean {
  const at = value.indexOf(prior)
  if (at < 0) return false
  const adjacent = [value.slice(0,at).slice(-1),value.slice(at+prior.length,at+prior.length+1)]
  return adjacent.every(c => !/[\p{L}\p{N}_+#]/u.test(c))
}
