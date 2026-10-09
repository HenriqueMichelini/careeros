// Assemble explicit accounting for controlled change/clarification fixtures.
// Matching and conflict tests supply their own outcomes and stable references.
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
