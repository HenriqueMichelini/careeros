import React from "react"
import type { CaseEvidence, Metric, Definition } from "./types"

export function Json({
  label,
  value,
  open = false,
}: {
  label: string
  value: unknown
  open?: boolean
}) {
  return (
    <details open={open}>
      <summary>{label}</summary>
      <pre>
        {value === undefined || value === null
          ? "Unavailable: no observation retained."
          : JSON.stringify(value, null, 2)}
      </pre>
    </details>
  )
}
export function Select({
  label,
  value,
  values,
  onChange,
}: {
  label: string
  value: string
  values: string[]
  onChange: (value: string) => void
}) {
  return (
    <label>
      <span>{label}</span>
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">All / unspecified</option>
        {values.map((v) => (
          <option key={v}>{v}</option>
        ))}
      </select>
    </label>
  )
}
export function MetricTable({
  metrics,
  definitions,
}: {
  metrics: Metric[]
  definitions: Definition[]
}) {
  return (
    <>
      <p className="muted">
        Finite observed opportunities. Empty denominators and missing
        observations remain unavailable. No overall AI score.
      </p>
      <div className="table">
        <table>
          <thead>
            <tr>
              <th>Measure / case / field</th>
              <th>Language / origin</th>
              <th>Value and counts</th>
              <th>Coverage and limits</th>
            </tr>
          </thead>
          <tbody>
            {metrics.map((m, i) => (
              <tr key={i}>
                <td>
                  <strong>
                    {m.id} {m.measure}
                  </strong>
                  <br />
                  {m.caseId || "Cohort"} / {m.field || "unspecified"}
                </td>
                <td>
                  {m.language || "unspecified"}
                  <br />
                  {m.mode}
                </td>
                <td>
                  {m.value === null || m.value === undefined
                    ? "Unavailable"
                    : Number(m.value.toFixed(4))}{" "}
                  {m.units}
                  <br />
                  {m.numerator ?? "?"} / {m.denominator ?? "?"} opportunities
                </td>
                <td>
                  Eligible {m.eligible}; evaluated {m.evaluated}; missing{" "}
                  {m.missing}
                  <br />
                  {m.unavailableReason || m.estimator}
                  <br />
                  {m.scope}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <details>
        <summary>Metric definitions and sources ({definitions.length})</summary>
        {definitions.map((d) => (
          <article key={d.id} className="my-3">
            <h3>
              {d.id} {d.name}
            </h3>
            <p>{d.definition}</p>
            <p className="muted">
              {d.version} · {d.source}
            </p>
          </article>
        ))}
      </details>
    </>
  )
}
export function CaseDetail({ evidence }: { evidence: CaseEvidence }) {
  const actual =
    evidence.actual &&
    typeof evidence.actual === "object" &&
    !Array.isArray(evidence.actual)
      ? evidence.actual as Record<string, unknown>
      : null
  return (
    <article aria-label="Case evidence">
      <h3>
        {evidence.id} · {evidence.language}
      </h3>
      <p>
        Engine {evidence.engineState}; assertion {evidence.assertionVerdict};
        operational outcome {evidence.operationalOutcome}
      </p>
      <p className="notice">
        Curated matches and unmatched candidates are label evidence. An
        unmatched regex candidate is not a confirmed hallucination. Model
        judgments and explicit client decisions are separate; absent review
        stays unavailable.
      </p>
      <Json
        label="Original synthetic source and context"
        value={evidence.source}
        open
      />
      <Json
        label="Curated gold / required and optional labels"
        value={evidence.expected}
      />
      <Json
        label="Actual material output and source references"
        value={actual?.output ?? evidence.actual}
      />
      <Json
        label="Curated missing / unmatched units"
        value={
          actual?.labelFacetScores ??
          actual?.score ??
          actual?.currentControlledReplay
        }
      />
      <Json
        label="Captured model outputs / judgments"
        value={actual?.providerOutputs}
      />
      <Json label="Explicit client review decision" value={evidence.review} />
      <Json label="Complete retained actual artifact" value={evidence.actual} />
      <Json
        label="Unavailable evidence and expected negative controls"
        value={{
          unavailable: evidence.unavailableReasons,
          category: evidence.failureFamilies,
        }}
      />
      {evidence.children?.map((c) => (
        <CaseDetail key={c.identity} evidence={c} />
      ))}
    </article>
  )
}
