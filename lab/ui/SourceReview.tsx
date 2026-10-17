import React from "react"
import { bytes, request } from "./api"
import { Json } from "./components"

type Worksheet = {
  id: string
  state: string
  revision: number
  contextHash: string
  derivedLabelVersion?: string
  annotations: { id: string }[]
  binding: {
    labelVersion: string
    sourceBytes: string
  }
}
type Approval = {
  reviewer: string
  contextHash: string
  worksheetHash: string
  confirmation: string
}
export function SourceReview({
  runId,
  caseId,
  reviewer,
}: {
  runId: string
  caseId: string
  reviewer?: string
}) {
  const [worksheet, setWorksheet] = React.useState<Worksheet | null>(null)
  const [error, setError] = React.useState("")
  const [busy, setBusy] = React.useState(false)
  const [approval, setApproval] = React.useState<Approval | null>(null)
  const [confirmation, setConfirmation] = React.useState("")
  const [rationale, setRationale] = React.useState("")
  const [decisions, setDecisions] = React.useState("")
  const [report, setReport] = React.useState<unknown>(null)
  const casePath = `runs/${encodeURIComponent(runId)}/cases/${encodeURIComponent(caseId)}/worksheet`
  const sheetPath = worksheet
    ? `runs/${encodeURIComponent(runId)}/worksheets/${worksheet.id}`
    : ""
  async function act(work: () => Promise<void>) {
    if (busy) return
    setBusy(true)
    setError("")
    try {
      await work()
    } catch (e) {
      setError(e instanceof Error ? e.message : "Review unavailable.")
    } finally {
      setBusy(false)
    }
  }
  function replace(value: Worksheet) {
    setWorksheet(value)
    setApproval(null)
    setConfirmation("")
    setReport(null)
  }
  async function exportSheet() {
    const value = await request<Worksheet>(sheetPath)
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(value, null, 2)], { type: "application/json" }),
    )
    const link = document.createElement("a")
    link.href = url
    link.download = "source-review.json"
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return (
    <section aria-label="Source review worksheet">
      <h3>Source review worksheet</h3>
      <p>
        Review exact synthetic source and output spans offline. Imports retain
        reviewer identity and append pending versions. Agent and model proposals
        require human adjudication.
      </p>
      <button
        disabled={busy}
        onClick={() =>
          void act(async () => replace(await request<Worksheet>(casePath, {})))
        }
      >
        Create pending source review
      </button>
      <label>
        Import source review (up to 1 MiB)
        <input
          type="file"
          accept=".json,application/json"
          disabled={busy}
          onChange={(e) => {
            const file = e.target.files?.[0]
            e.target.value = ""
            if (!file) return
            void act(async () => {
              if (file.size > 1024 * 1024)
                throw new Error("Worksheet exceeds 1 MiB.")
              replace(
                await request<Worksheet>(`${casePath}/import`, {
                  worksheetBytes: await bytes(file),
                }),
              )
            })
          }}
        />
      </label>
      {!reviewer && (
        <p>
          Human approval is disabled in this launch. Henrique or a designated
          domain reviewer must be configured explicitly.
        </p>
      )}
      {worksheet && (
        <>
          <p className="identity">
            {worksheet.state} · version {worksheet.revision} · {worksheet.id}
            <br />
            Original labels: {worksheet.binding.labelVersion}
            <br />
            Derived labels: {worksheet.derivedLabelVersion ?? "pending"}
          </p>
          <button disabled={busy} onClick={() => void act(exportSheet)}>
            Export source review
          </button>
          <button
            disabled={busy}
            onClick={() =>
              void act(async () =>
                setReport(await request(`${sheetPath}/report`)),
              )
            }
          >
            Show measurement-only review report
          </button>
          {report !== null && (
            <Json label="Measurement-only review report" value={report} open />
          )}
          {reviewer &&
            worksheet.state === "pending" &&
            worksheet.annotations.length > 0 && (
              <>
                <p>
                  Configured human domain reviewer: {reviewer}. This records a
                  local human attestation. It is not external identity
                  verification.
                </p>
                <button
                  disabled={busy}
                  onClick={() =>
                    void act(async () => {
                      setApproval(
                        await request<Approval>(`${sheetPath}/approval`),
                      )
                      setConfirmation("")
                    })
                  }
                >
                  Prepare human adjudication
                </button>
                {approval && (
                  <>
                    <p className="identity">
                      Review this exact worksheet, then type:{" "}
                      {approval.confirmation}
                    </p>
                    <label>
                      Human review confirmation
                      <input
                        value={confirmation}
                        onChange={(e) => setConfirmation(e.target.value)}
                        autoComplete="off"
                      />
                    </label>
                    <label>
                      Adjudication rationale
                      <textarea
                        value={rationale}
                        onChange={(e) => setRationale(e.target.value)}
                        maxLength={4096}
                      />
                    </label>
                    <label>
                      Annotation decisions (JSON array)
                      <textarea
                        value={decisions}
                        onChange={(e) => setDecisions(e.target.value)}
                        maxLength={50000}
                        placeholder='[{"annotationId":"…","verdict":"supported","rationale":"…"}]'
                      />
                    </label>
                    <button
                      disabled={
                        busy ||
                        confirmation !== approval.confirmation ||
                        !rationale ||
                        !decisions
                      }
                      onClick={() =>
                        void act(async () =>
                          replace(
                            await request<Worksheet>(
                              `${sheetPath}/adjudicate`,
                              {
                                actor: { id: approval.reviewer, type: "human" },
                                contextHash: approval.contextHash,
                                worksheetHash: approval.worksheetHash,
                                confirmation,
                                rationale,
                                decisions: JSON.parse(decisions),
                              },
                            ),
                          ),
                        )
                      }
                    >
                      Append human adjudication
                    </button>
                  </>
                )}
              </>
            )}
        </>
      )}
      {error && <p role="alert">{error}</p>}
    </section>
  )
}
