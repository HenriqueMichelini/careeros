// Deterministic whole-submission responses shared by local browser checks.
export function journeyFetch(url, options) {
  if (url !== "/api/profile/ingest") return window.__realFetch(url, options)
  const body = JSON.parse(options.body)
  const calls = (window.__journeyCalls ??= [])
  calls.push({ kind: body.stage?.kind, body })
  const scenario = window.__journeyScenario || "normal"
  const runId = "run-journey"
  const bytes = new TextEncoder().encode(body.input)
  const splitAt = body.input.indexOf("I led")
  const split =
    splitAt > 0
      ? new TextEncoder().encode(body.input.slice(0, splitAt)).length
      : bytes.length
  const ranges =
    scenario === "partial"
      ? [
          [0, split],
          [split, bytes.length],
        ]
      : [[0, bytes.length]]
  const batches = ranges.map(([Start, End], index) => ({
    index,
    primary: [{ Start, End }],
    context: [],
    preparedBytes: 200,
    payloadBytes: 128,
  }))
  const receipt = (label) => `${label}.${"s".repeat(43)}`
  const expiresAt = Date.now() + 600000
  const ok = (raw) =>
    new Response(JSON.stringify(raw), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })

  if (body.stage?.kind === "plan")
    return ok({
      stage: { version: 2, kind: "plan" },
      planReceipt: receipt("plan"),
      expiresAt,
      plan: {
        protocolVersion: "profile-submission-v1",
        sourceId: "source-journey",
        profileId: "profile-journey",
        runId,
        planVersion: "portions-v1",
        status: "ready",
        complete: true,
        originalBytes: bytes.length,
        preparedBytes: batches.length * 200,
        batches,
        remaining: [],
      },
    })

  if (body.stage?.kind === "extract") {
    if (
      scenario === "partial" &&
      body.batchIndex === 1 &&
      window.__failBatchOnce
    ) {
      window.__failBatchOnce = false
      return new Response(JSON.stringify({ error: "outage" }), { status: 502 })
    }
    const claim = {
      id: "c0",
      source: body.input,
      text:
        scenario === "clarification"
          ? "TypeScript version is unclear"
          : body.input,
      targets: scenario === "clarification" ? [] : ["skills"],
      question:
        scenario === "clarification"
          ? "Which TypeScript version do you use?"
          : "",
    }
    const richClaims = [
      ["I use Kubernetes.", ["skills"], ""],
      ["I also use Go.", ["careerGoals"], ""],
      ["I build with Terraform.", ["tools"], ""],
      ["I lead cross-functional delivery.", ["competencies"], ""],
      ["Build useful products.", ["careerGoals"], ""],
      ["Python", ["skills"], ""],
      ["French level is unclear.", [], "What is your French level?"],
    ].map(([text, targets, question], index) => ({
      id: `c${index}`,
      source: body.input,
      text,
      targets,
      question,
    }))
    const claims = scenario === "rich" ? richClaims : [claim]
    const skipped =
      scenario === "partial" && body.batchIndex === 0
        ? [
            {
              index: 1,
              reason: "source",
              text: "Unvalidated extracted claim",
              source: "",
              shortened: false,
            },
          ]
        : []
    const response = () =>
      ok({
        stage: { version: 2, kind: "extract" },
        batchReceipt: receipt(`batch${body.batchIndex}`),
        expiresAt,
        batchIndex: body.batchIndex,
        claims,
        unverifiedCount: skipped.length,
        skippedClaims: skipped,
        coverage: {
          validClaims: claims.length,
          invalidClaims: skipped.length,
          discoveryComplete: false,
          capacity: "within_limit",
        },
      })
    if (window.__holdExtract) {
      window.__holdExtract = false
      window.__extractHeld = true
      return new Promise((resolve, reject) => {
        window.__releaseExtract = () => resolve(response())
        options.signal?.addEventListener(
          "abort",
          () => {
            window.__extractAborted = true
            reject(new DOMException("Aborted", "AbortError"))
          },
          { once: true },
        )
      })
    }
    return response()
  }

  if (body.stage?.kind === "assemble") {
    const richClaimIds = Array.from(
      { length: 7 },
      (_, index) => `${runId}/0/c${index}`,
    )
    const claimIds =
      scenario === "rich"
        ? richClaimIds
        : batches.map((batch) => `${runId}/${batch.index}/c0`)
    return ok({
      stage: { version: 2, kind: "assemble" },
      contextReceipt: receipt("context"),
      expiresAt,
      context: {
        protocolVersion: "profile-submission-v1",
        sourceId: "source-journey",
        profileId: "profile-journey",
        runId,
        planVersion: "portions-v1",
        batches: batches.map((batch, index) => ({
          index: batch.index,
          primary: batch.primary,
          claimIds: scenario === "rich" ? richClaimIds : [claimIds[index]],
          claimCount: scenario === "rich" ? richClaimIds.length : 1,
          skippedCount: scenario === "partial" && index === 0 ? 1 : 0,
        })),
        coverage: {
          validClaims:
            scenario === "rich" ? richClaimIds.length : batches.length,
          invalidClaims: scenario === "partial" ? 1 : 0,
          discoveryComplete: false,
          capacity: "within_limit",
        },
      },
    })
  }

  if (body.stage?.kind === "reconcile") {
    const focused = body.focus.claimIds
    if (scenario === "rich") {
      const document = JSON.parse(
        localStorage.getItem("careeros_profile_v2") || "null",
      )
      const factReference = (field, value) => {
        const fact = document?.facts.find(
          (candidate) =>
            candidate.field === field && String(candidate.value) === value,
        )
        return fact
          ? [
              {
                profileId: document.id,
                id: fact.id,
                revision: fact.revision,
              },
            ]
          : []
      }
      const related = factReference("skills", "React")
      const duplicateRelated = factReference(
        "careerGoals",
        "Build useful products.",
      )
      const longValue = `Cross-functional delivery across product, design and engineering, with measurable outcomes for customers in multiple regions. https://example.test/${"a".repeat(140)}`
      const operation = (
        claimId,
        target,
        value,
        action = "add",
        finding = "addition",
      ) => ({
        claimId,
        target,
        entryId: "",
        field: target,
        action,
        value,
        finding,
      })
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: [
          operation(focused[0], "skills", "Kubernetes"),
          operation(
            focused[1],
            "careerGoals",
            "Build useful products and mentor.",
            "update",
            "in_place",
          ),
          operation(focused[2], "tools", "Terraform"),
          operation(
            focused[3],
            "competencies",
            longValue,
            "update",
            "in_place",
          ),
        ],
        outcomes: [
          ...focused.slice(0, 4).map((claimId) => ({
            claimId,
            kind: "change",
            reason: "validated_operation",
            relatedFacts: [],
            relatedClaimIds: [],
          })),
          {
            claimId: focused[4],
            kind: "exact_duplicate",
            reason: "verified_exact_alias_or_wording",
            relatedFacts: duplicateRelated,
            relatedClaimIds: [],
          },
          {
            claimId: focused[5],
            kind: "contradiction",
            reason: "source_ambiguity",
            relatedFacts: related,
            relatedClaimIds: [focused[4]],
          },
          {
            claimId: focused[6],
            kind: "clarification",
            reason: "source_ambiguity",
            relatedFacts: [],
            relatedClaimIds: [],
          },
        ],
      })
    }
    if (scenario === "duplicate" || scenario === "clarification")
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: [],
        outcomes: focused.map((claimId) => ({
          claimId,
          kind: scenario === "duplicate" ? "exact_duplicate" : "clarification",
          reason: scenario === "duplicate"
            ? "verified_exact_alias_or_wording"
            : "source_ambiguity",
          relatedFacts: [],
          relatedClaimIds: [],
        })),
      })
    if (scenario === "invalid")
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: focused.flatMap((claimId) => [
          {
            claimId,
            target: "experience",
            entryId: "new:experience-1",
            field: "title",
            action: "add",
            value: "Engineer",
            finding: "addition",
          },
          {
            claimId,
            target: "experience",
            entryId: "new:experience-1",
            field: "startDate",
            action: "add",
            value: "2020",
            finding: "addition",
          },
        ]),
        outcomes: focused.map((claimId) => ({
          claimId,
          kind: "change",
          reason: "validated_operation",
          relatedFacts: [],
          relatedClaimIds: [],
        })),
      })
    return ok({
      stage: { version: 2, kind: "reconcile" },
      focus: { claimIds: focused },
      operations: focused.map((claimId, index) => ({
        claimId,
        target: "skills",
        entryId: "",
        field: "skills",
        action: "add",
        value: index === 0 ? "TypeScript" : "Payments leadership",
        finding: "addition",
      })),
      outcomes: focused.map((claimId) => ({
        claimId,
        kind: "change",
        reason: "validated_operation",
        relatedFacts: [],
        relatedClaimIds: [],
      })),
    })
  }

  if (body.stage?.kind === "clarify") {
    const answer = body.answer.trim()
    const target = scenario === "rich" ? "additionalInfo" : "skills"
    const response = () =>
      ok({
        stage: { version: 2, kind: "clarify" },
        claim: {
          ...body.claim,
          targets: [target],
          question: "",
          supportingSources: [
            { source: answer, origin: "clarification_answer" },
          ],
        },
        focus: { claimIds: [body.claim.id] },
        operations: [
          {
            claimId: body.claim.id,
            target,
            entryId: "",
            field: target,
            action: "add",
            value: answer,
            finding: "addition",
          },
        ],
        outcomes: [
          {
            claimId: body.claim.id,
            kind: "change",
            reason: "validated_operation",
            relatedFacts: [],
            relatedClaimIds: [],
          },
        ],
      })
    if (window.__holdClarify) {
      window.__holdClarify = false
      window.__clarifyHeld = true
      return new Promise((resolve, reject) => {
        window.__releaseClarify = () => resolve(response())
        options.signal?.addEventListener(
          "abort",
          () => reject(new DOMException("Aborted", "AbortError")),
          { once: true },
        )
      })
    }
    return response()
  }

  throw new Error(`Unexpected stage ${body.stage?.kind}`)
}
