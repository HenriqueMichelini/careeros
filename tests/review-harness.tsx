// Controlled browser state for the compact destination review (#190). The
// harness owns a `ReviewPlan` and wires every presenter callback back to the
// pure review model. It imports no provider, storage or canonical writer.
import { useEffect, useState } from "react"
import { createRoot } from "react-dom/client"
import "../src/index.css"
import { SubmissionReview } from "../src/components/SubmissionReview"
import {
  buildReviewPlan,
  clearReviewSelection,
  editReviewUnit,
  reviewPreview,
  reviewSummary,
  selectReviewAdditions,
  selectReviewGroup,
  selectReviewUnit,
  transferReviewState,
  type ReviewPlan,
} from "../src/lib/ingestionReview"
import {
  IngestionError,
  type IngestionClaim,
  type IngestionOperation,
  type IngestionOutcome,
} from "../src/lib/ingestion"
import {
  emptyProfileView,
  migrateProfile,
  type ProfileDocument,
} from "../src/lib/profileDocument"
import { StoreProvider } from "../src/lib/store"
import type { ProfessionalRepository } from "../src/lib/types"

type Scenario = "mixed" | "empty" | "duplicates" | "clarification"
interface ReviewSubmissionData {
  protocolVersion: string
  submissionId: string
  runId: string
  sourceId: string
  profileId: string
  planVersion: string
  batches: {
    index: number
    claimIds: string[]
  }[]
  claims: IngestionClaim[]
  operations: IngestionOperation[]
  outcomes: IngestionOutcome[]
  unresolvedClaimIds: string[]
}

const params = new URLSearchParams(location.search)
const scenario = (params.get("scenario") ?? "mixed") as Scenario
const locale = params.get("locale")
if (locale === "en" || locale === "pt-BR")
  localStorage.setItem("careeros_locale", locale)

const base = emptyProfileView()
const populated: ProfessionalRepository = {
  ...base,
  skills: "React",
  additionalInfo: "Kept note",
  experience: [
    {
      id: "job",
      company: "Harbor Works",
      title: "Product Lead",
      startDate: "2021",
      endDate: "2024",
      current: false,
      location: "Lisbon",
      description: "Led a platform.",
      responsibilities: "Owned discovery.",
      achievements: "Improved onboarding.",
    },
  ],
  projects: [
    {
      id: "project",
      name: "Service Atlas",
      description: "Mapped service journeys.",
      technologies: "Figma",
      url: "",
      highlights: "Improved task completion.",
    },
  ],
}
const snapshotOf = (view: ProfessionalRepository): ProfileDocument =>
  migrateProfile(view, "profile")

const claim = (
  id: string,
  overrides: Partial<IngestionClaim> = {},
): IngestionClaim => ({
  id,
  source: id,
  text: id,
  targets: [],
  question: "",
  ...overrides,
})
const operation = (
  claimId: string,
  overrides: Partial<IngestionOperation> = {},
): IngestionOperation => ({
  claimId,
  target: "skills",
  entryId: "",
  field: "skills",
  action: "add",
  value: "Value",
  finding: "addition",
  approved: false,
  ...overrides,
})
const submission = (
  overrides: Partial<ReviewSubmissionData> = {},
): ReviewSubmissionData => ({
  protocolVersion: "profile-submission-v1",
  submissionId: "sub-review-harness",
  runId: "run",
  sourceId: "source-1",
  profileId: "profile",
  planVersion: "portions-v1",
  batches: [],
  claims: [],
  operations: [],
  outcomes: [],
  unresolvedClaimIds: [],
  ...overrides,
})
const planFor = (
  submission: ReviewSubmissionData,
  snapshot: ProfileDocument,
): ReviewPlan => buildReviewPlan({ submission, snapshot })

const skillFact = (snapshot: ProfileDocument) =>
  snapshot.facts.find((fact) => fact.field === "skills")!

function mixedPlan(): ReviewPlan {
  const snapshot = snapshotOf(populated)
  const fact = skillFact(snapshot)
  const ids = Array.from({ length: 11 }, (_, index) => `run/0/c${index + 1}`)
  const claims: IngestionClaim[] = [
    claim(ids[0], {
      source: "I use TypeScript",
      text: "TypeScript",
      targets: ["skills"],
      supportingSources: [
        { source: "At Acme I used TypeScript." },
        {
          source: "I used it in production.",
          origin: "clarification_answer",
        },
        { source: "I use TypeScript" },
      ],
      sourceReference: {
        version: 1,
        sourceId: "source-1",
        preparationVersion: "structure-v1",
        segmentId: "segment-1",
        occurrenceId: "occurrence-1",
        originalStart: 0,
        originalEnd: 18,
      },
    }),
    claim(ids[1], {
      source: "React",
      text: "React",
      targets: ["skills"],
    }),
    claim(ids[2], {
      source: "I led the platform at Harbor Works.",
      text: "Led the Harbor Works platform",
      targets: ["experience"],
    }),
    claim(ids[3], {
      source: "I work in Figma every day.",
      text: "Figma",
      targets: ["tools"],
    }),
    claim(ids[4], {
      source: "The kept note is no longer current.",
      text: "Remove the kept note",
      targets: ["additionalInfo"],
    }),
    claim(ids[5], {
      source: "I used it in production.",
      text: "Production TypeScript",
      targets: ["skills"],
    }),
    claim(ids[6], {
      source: "I don't use React anymore.",
      text: "No React",
      targets: ["skills"],
      question: "Do you still use React?",
    }),
    claim(ids[7], {
      source: "I worked at Acme.",
      text: "Acme role needs details",
      targets: ["experience"],
      question: "Was this full-time or contract?",
    }),
    claim(ids[8], {
      source: "I speak Japanese.",
      text: "Japanese",
      targets: ["languages"],
    }),
    claim(ids[9], {
      source: "I did something.",
      text: "Something",
      targets: ["languages"],
    }),
    claim(ids[10], {
      source: "I joined Nova Labs in 2025.",
      text: "Senior Engineer at Nova Labs",
      targets: ["experience"],
    }),
  ]
  const operations: IngestionOperation[] = [
    operation(ids[0], { value: "TypeScript" }),
    operation(ids[2], {
      target: "experience",
      entryId: "job",
      field: "description",
      action: "update",
      value: "Led a global platform.",
      finding: "overlap",
    }),
    operation(ids[3], {
      target: "tools",
      field: "tools",
      value:
        "Figma, Sketch, Adobe XD, Framer, Miro, Notion, Linear, Jira, Confluence",
    }),
    operation(ids[4], {
      target: "additionalInfo",
      field: "additionalInfo",
      action: "remove",
      value: "Kept note",
      finding: "in_place",
    }),
    operation(ids[5], {
      action: "evidence",
      value: "TypeScript",
      finding: "in_place",
      supportingClaimIds: [ids[0]],
    }),
    operation(ids[10], {
      target: "experience",
      entryId: "new:run/0/c11",
      field: "company",
      value: "Nova Labs",
    }),
    operation(ids[10], {
      target: "experience",
      entryId: "new:run/0/c11",
      field: "title",
      value: "Senior Engineer",
    }),
    operation(ids[10], {
      target: "experience",
      entryId: "new:run/0/c11",
      field: "startDate",
      value: "2025",
    }),
    operation(ids[10], {
      target: "experience",
      entryId: "new:run/0/c11",
      field: "description",
      value: "Built a billing service used across three regions.",
    }),
  ]
  const outcomes: IngestionOutcome[] = [
    {
      claimId: ids[0],
      kind: "change",
      reason: "Not in your Profile.",
      relatedFacts: [],
      relatedClaimIds: [],
      operationIndexes: [0],
    },
    {
      claimId: ids[1],
      kind: "exact_duplicate",
      reason: "Same assertion.",
      relatedFacts: [
        { profileId: snapshot.id, id: fact.id, revision: fact.revision },
      ],
      relatedClaimIds: [],
      operationIndexes: [],
    },
    {
      claimId: ids[2],
      kind: "change",
      reason: "The saved description is narrower.",
      relatedFacts: [],
      relatedClaimIds: [],
      operationIndexes: [1],
    },
    {
      claimId: ids[3],
      kind: "change",
      reason: "Not in your Profile.",
      relatedFacts: [],
      relatedClaimIds: [],
      operationIndexes: [2],
    },
    {
      claimId: ids[4],
      kind: "change",
      reason: "No longer current.",
      relatedFacts: [],
      relatedClaimIds: [],
      operationIndexes: [3],
    },
    {
      claimId: ids[5],
      kind: "additional_support",
      reason: "Adds production evidence.",
      relatedFacts: [
        { profileId: snapshot.id, id: fact.id, revision: fact.revision },
      ],
      relatedClaimIds: [],
      operationIndexes: [4],
    },
    {
      claimId: ids[6],
      kind: "contradiction",
      reason: "Competing statements.",
      relatedFacts: [
        { profileId: snapshot.id, id: fact.id, revision: fact.revision },
      ],
      relatedClaimIds: [ids[1]],
      operationIndexes: [],
    },
    {
      claimId: ids[7],
      kind: "clarification",
      reason: "Role type missing.",
      relatedFacts: [],
      relatedClaimIds: [],
      operationIndexes: [],
    },
    {
      claimId: ids[8],
      kind: "unsupported",
      reason: "No supporting excerpt.",
      relatedFacts: [],
      relatedClaimIds: [],
      operationIndexes: [],
    },
    {
      claimId: ids[9],
      kind: "unresolved",
      reason: "Could not compare.",
      relatedFacts: [],
      relatedClaimIds: [],
      operationIndexes: [],
    },
    {
      claimId: ids[10],
      kind: "change",
      reason: "New role.",
      relatedFacts: [],
      relatedClaimIds: [],
      operationIndexes: [5, 6, 7, 8],
    },
  ]
  return planFor(
    submission({
      batches: [{ index: 0, claimIds: ids }],
      claims,
      operations,
      outcomes,
      unresolvedClaimIds: [ids[9]],
    }),
    snapshot,
  )
}

function duplicatePlan(): ReviewPlan {
  const snapshot = snapshotOf(populated)
  const fact = skillFact(snapshot)
  const id = "run/0/c1"
  return planFor(
    submission({
      batches: [{ index: 0, claimIds: [id] }],
      claims: [
        claim(id, { source: "React", text: "React", targets: ["skills"] }),
      ],
      outcomes: [
        {
          claimId: id,
          kind: "exact_duplicate",
          reason: "Same assertion.",
          relatedFacts: [
            { profileId: snapshot.id, id: fact.id, revision: fact.revision },
          ],
          relatedClaimIds: [],
          operationIndexes: [],
        },
      ],
    }),
    snapshot,
  )
}

function emptyPlan(): ReviewPlan {
  const snapshot = snapshotOf(emptyProfileView())
  const ids = ["run/0/c1", "run/0/c2"]
  return planFor(
    submission({
      batches: [{ index: 0, claimIds: ids }],
      claims: [
        claim(ids[0], {
          source: "I use TypeScript.",
          text: "TypeScript",
          targets: ["skills"],
        }),
        claim(ids[1], {
          source: "I joined Nova Labs in 2025.",
          text: "Senior Engineer at Nova Labs",
          targets: ["experience"],
        }),
      ],
      operations: [
        operation(ids[0], { value: "TypeScript" }),
        operation(ids[1], {
          target: "experience",
          entryId: "new:run/0/c2",
          field: "company",
          value: "Nova Labs",
        }),
        operation(ids[1], {
          target: "experience",
          entryId: "new:run/0/c2",
          field: "title",
          value: "Senior Engineer",
        }),
        operation(ids[1], {
          target: "experience",
          entryId: "new:run/0/c2",
          field: "startDate",
          value: "2025",
        }),
      ],
      outcomes: [
        {
          claimId: ids[0],
          kind: "change",
          reason: "Not in your Profile.",
          relatedFacts: [],
          relatedClaimIds: [],
          operationIndexes: [0],
        },
        {
          claimId: ids[1],
          kind: "change",
          reason: "New role.",
          relatedFacts: [],
          relatedClaimIds: [],
          operationIndexes: [1, 2, 3],
        },
      ],
    }),
    snapshot,
  )
}

function clarificationPlan(answered: boolean): ReviewPlan {
  const snapshot = snapshotOf(populated)
  const fact = skillFact(snapshot)
  const ids = ["run/0/c1", "run/0/c2", "run/0/c3"]
  const operations: IngestionOperation[] = [
    ...(answered ? [operation(ids[0], { value: "Contract" })] : []),
    operation(ids[1], { value: "Svelte" }),
    operation(ids[2], { value: "Vue" }),
  ]
  const offset = answered ? 1 : 0
  return planFor(
    submission({
      batches: [{ index: 0, claimIds: ids }],
      claims: [
        claim(ids[0], {
          source: "I worked at Acme.",
          text: "Acme role needs details",
          targets: answered ? ["skills"] : ["experience"],
          question: answered ? "" : "Was this full-time or contract?",
        }),
        claim(ids[1], {
          source: "I use Svelte.",
          text: "Svelte",
          targets: ["skills"],
        }),
        claim(ids[2], {
          source: "I use Vue.",
          text: "Vue",
          targets: ["skills"],
        }),
      ],
      operations,
      outcomes: [
        {
          claimId: ids[0],
          kind: answered ? "change" : "clarification",
          reason: answered ? "Resolved answer." : "Role type missing.",
          relatedFacts: answered
            ? []
            : [
                {
                  profileId: snapshot.id,
                  id: fact.id,
                  revision: fact.revision,
                },
              ],
          relatedClaimIds: answered ? [] : [ids[1]],
          operationIndexes: answered ? [0] : [],
        },
        {
          claimId: ids[1],
          kind: "change",
          reason: "Not in your Profile.",
          relatedFacts: [],
          relatedClaimIds: [],
          operationIndexes: [offset],
        },
        {
          claimId: ids[2],
          kind: "change",
          reason: "Not in your Profile.",
          relatedFacts: [],
          relatedClaimIds: [],
          operationIndexes: [offset + 1],
        },
      ],
    }),
    snapshot,
  )
}

const initial =
  scenario === "duplicates"
    ? duplicatePlan()
    : scenario === "empty"
      ? emptyPlan()
      : scenario === "clarification"
        ? clarificationPlan(false)
        : mixedPlan()

function Harness() {
  const [plan, setPlan] = useState(initial)
  const [leftOut, setLeftOut] = useState<string[]>([])
  useEffect(() => {
    const windowWithState = window as unknown as {
      __reviewState: unknown
    }
    const preview = reviewPreview(plan)
    windowWithState.__reviewState = {
      scenario,
      locale,
      leftOut,
      summary: reviewSummary(plan),
      preview: {
        error: preview.error,
        complete: preview.complete,
        selectedUnitCount: preview.selectedUnitCount,
        selectedOperationCount: preview.selectedOperationCount,
        fields: preview.fields,
      },
      units: plan.units.map((unit) => ({
        id: unit.id,
        kind: unit.kind,
        status: unit.status,
        selectable: unit.selectable,
        blockedReason: unit.blockedReason,
        sources: unit.sources,
        reason: unit.reason,
        relatedClaims: unit.relatedClaims,
        relatedFacts: unit.relatedFacts,
      })),
      operations: plan.operations.map((operation) => ({
        claimId: operation.claimId,
        field: operation.field,
        action: operation.action,
        value: operation.value,
        proposedValue: operation.proposedValue,
      })),
    }
  }, [plan, leftOut])
  return (
    <main className="p-4 max-w-5xl mx-auto" data-harness-scenario={scenario}>
      <SubmissionReview
        plan={plan}
        onSelectUnit={(unitId, selected) =>
          setPlan((current) =>
            selectReviewUnit(
              current,
              unitId,
              selected ? "selected" : "pending",
            ),
          )
        }
        onSelectGroup={(destinationId, selected) =>
          setPlan((current) =>
            selectReviewGroup(
              current,
              destinationId,
              selected ? "selected" : "pending",
            ),
          )
        }
        onSelectAdditions={() =>
          setPlan((current) => selectReviewAdditions(current))
        }
        onClearSelection={() =>
          setPlan((current) => clearReviewSelection(current))
        }
        onEditUnit={(unitId, field, value) =>
          setPlan((current) => editReviewUnit(current, unitId, field, value))
        }
        onAnswerUnit={
          scenario === "clarification"
            ? async (unitId, answer) => {
                const unit = plan.units.find((entry) => entry.id === unitId)
                if (!unit) throw new IngestionError("state")
                if (answer.trim() === "blocked answer")
                  throw new IngestionError("field_decision", {
                    version: 1,
                    field: "professional_information",
                    outcome: { kind: "request_rephrasing" },
                  })
                await new Promise((resolve) => setTimeout(resolve, 20))
                setPlan((current) =>
                  transferReviewState(current, clarificationPlan(true), [
                    unit.claimIds[0],
                  ]),
                )
              }
            : undefined
        }
        onLeaveUnitOut={
          scenario === "clarification"
            ? (unitId) =>
                setLeftOut((current) =>
                  current.includes(unitId) ? current : [...current, unitId],
                )
            : undefined
        }
      />
    </main>
  )
}

createRoot(document.getElementById("review-root")!).render(
  <StoreProvider>
    <Harness />
  </StoreProvider>,
)
