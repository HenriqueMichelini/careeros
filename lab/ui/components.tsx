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
  const artifacts = Array.isArray(actual?.artifacts) ? actual.artifacts : []
  const [artifactError, setArtifactError] = React.useState("")
  async function saveArtifact(value: unknown) {
    try {
      if (!value || typeof value !== "object")
        throw new Error("Artifact unavailable")
      const artifact = value as Record<string, unknown>
      let mime: string | null = null
      if (artifact.kind === "screenshot") mime = "image/png"
      if (artifact.kind === "cdp-produced-pdf") mime = "application/pdf"
      if (
        !mime ||
        typeof artifact.bytes !== "string" ||
        artifact.bytes.length > 1400000 ||
        typeof artifact.hash !== "string"
      )
        throw new Error("Artifact type or byte bound unavailable")
      const raw = Uint8Array.from(
        atob(artifact.bytes),
        (c) => c.codePointAt(0) ?? 0,
      )
      const hash = Array.from(
        new Uint8Array(await crypto.subtle.digest("SHA-256", raw)),
        (b) => b.toString(16).padStart(2, "0"),
      ).join("")
      if (hash !== artifact.hash) throw new Error("Artifact hash mismatch")
      const url = URL.createObjectURL(new Blob([raw], { type: mime })),
        link = document.createElement("a")
      link.href = url
      link.download =
        artifact.kind === "screenshot" ? "lab-screenshot.png" : "lab-cdp.pdf"
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
      setArtifactError("")
    } catch (error) {
      setArtifactError(
        error instanceof Error ? error.message : "Artifact unavailable",
      )
    }
  }
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
      {artifacts.length > 0 && (
        <div aria-label="Typed test artifacts">
          <p>
            Controlled test artifacts. Screenshots and CDP PDFs do not establish
            native print/save.
          </p>
          {artifacts.map((value, index) => {
            const item = value as Record<string, unknown>
            return (
              <p
                key={String(item.hash) + ":" + String(item.name)}
                className="identity"
              >
                {String(item.kind)} · {String(item.name)} ·{" "}
                {String(item.origin)} · {String(item.runtime)}
                <br />
                SHA-256 {String(item.hash)}{" "}
                <button onClick={() => void saveArtifact(value)}>
                  Download verified artifact {index + 1}
                </button>
              </p>
            )
          })}
          {artifactError && <p role="alert">{artifactError}</p>}
        </div>
      )}
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
