// Assemble explicit accounting for controlled change/clarification fixtures.
// Matching and conflict tests supply their own outcomes and stable references.
// Adapt older domain fixtures to the new two-request transport. A successful
// fixture is held for reconciliation; rejection still terminates extraction.
// Dedicated staged tests use raw fetch doubles to assert every request.
export function stagedFixtureFetch(handler) {
  const results = new Map()
  let next = 0
  return async (url, options) => {
    if (url !== "/api/profile/ingest") return handler(url, options)
    const request = JSON.parse(options.body)
    if (request.stage?.kind === "reconcile") {
      const result = results.get(request.receipt)
      if (!result) throw new Error("No controlled extraction for reconciliation")
      results.delete(request.receipt)
      return result
    }
    const response = await handler(url, options)
    const raw = await (response.clone?.() ?? response).json().catch(() => null)
    if (!response.ok || request.stage?.kind !== "extract" || !raw?.outcomes || raw.decision?.outcome.kind !== "accept") return response
    const receipt = `fixture${++next}.${"s".repeat(43)}`
    results.set(receipt, new Response(JSON.stringify(raw), { status: 200 }))
    return new Response(JSON.stringify({ stage: { version: 1, kind: "extract" }, receipt, expiresAt: Date.now() + 600000, decision: raw.decision, claims: raw.claims, coverage: raw.coverage }))
  }
}

export const stagedBrowserFixtures = `(() => {
  const adapt = ${stagedFixtureFetch.toString()};
  let original = window.fetch, adapted = original;
  Object.defineProperty(window, 'fetch', { configurable: true,
    get() { return window.__rawStagedFixtures ? original : adapted },
    set(handler) { original = handler; adapted = adapt(handler) }
  });
})()`

export function withIngestionLedger(data) {
  const unresolvedClaimIds = [
    ...new Set([
      ...(data.unresolvedClaimIds ?? []),
      ...data.claims
        .filter(
          (c) =>
            !c.question && !data.operations.some((op) => op.claimId === c.id),
        )
        .map((c) => c.id),
    ]),
  ]
  return {
    ...data,
    unresolvedClaimIds,
    outcomes: data.claims.map((c) => ({
      claimId: c.id,
      kind: c.question
        ? "clarification"
        : unresolvedClaimIds.includes(c.id)
          ? "unresolved"
          : "change",
      reason: c.question
        ? "source_ambiguity"
        : unresolvedClaimIds.includes(c.id)
          ? "no_validated_disposition"
          : "validated_operation",
      relatedFacts: [],
      relatedClaimIds: [],
      operationIndexes: data.operations.flatMap((op, index) =>
        op.claimId === c.id ? [index] : [],
      ),
    })),
    coverage: {
      validClaims: data.claims.length,
      invalidClaims: data.unverifiedClaimCount ?? 0,
      discoveryComplete: false,
      capacity:
        data.claims.length + (data.unverifiedClaimCount ?? 0) === 30
          ? "possibly_exhausted"
          : "within_limit",
    },
    skippedClaims: Array.from(
      { length: data.unverifiedClaimCount ?? 0 },
      (_, index) => ({
        index: data.claims.length + index + 1,
        reason: "source",
        text: "Unvalidated extracted claim",
        source: "",
        shortened: false,
      }),
    ),
  }
}
