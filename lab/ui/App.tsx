import { SourceReview } from "./SourceReview"
import React, { useEffect, useState, useRef } from "react"
import { request, download, bytes } from "./api"
import { Json, Select, MetricTable, CaseDetail } from "./components"
import type {
  Suite,
  Status,
  Plan,
  Caps,
  View,
  CaseEvidence,
  Page,
  EventPage,
  Comparison,
} from "./types"

const unique = (values: string[]) => [...new Set(values)].sort()
const initialCaps: Caps = {
  maxCases: 100,
  maxHttpAttempts: 200,
  maxProviderCalls: 500,
  maxArtifactBytes: 8 * 1024 * 1024,
  deadlineMs: 60000,
}
const identity = () => crypto.randomUUID().replace(/-/g, "")
const errorText = (error: unknown) =>
  error instanceof Error ? error.message : "Local evidence unavailable."
const active = (view: View | null) =>
  view?.engineState === "planned" || view?.engineState === "running"
const restoredRun = () => /^#run=([a-f0-9]{32})$/.exec(location.hash)?.[1] ?? ""

export function App() {
  const [catalog, setCatalog] = useState<Suite[]>([]),
    [status, setStatus] = useState<Status | null>(null),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("")
  const [suiteId, setSuite] = useState("semantic-quality"),
    [configuration, setConfiguration] = useState("current"),
    [mode, setMode] = useState("controlled")
  const [filters, setFilters] = useState({
      workflow: "",
      language: "",
      category: "",
      split: "",
      search: "",
    }),
    [checked, setChecked] = useState<string[]>([]),
    [caps, setCaps] = useState(initialCaps)
  const [plan, setPlan] = useState<Plan | null>(null),
    [token, setToken] = useState(""),
    [consumed, setConsumed] = useState(false),
    [busy, setBusy] = useState(false)
  const [runId, setRunId] = useState(restoredRun),
    [view, setView] = useState<View | null>(null),
    [events, setEvents] = useState<EventPage | null>(null),
    [caseEvidence, setCaseEvidence] = useState<CaseEvidence | null>(null)
  const [runRead, setRunRead] = useState(0),
    [runError, setRunError] = useState("")
  const runSequence = useRef(0)
  const [caseError, setCaseError] = useState<{
    identity: string
    message: string
  } | null>(null)
  const caseSequence = useRef(0)
  const planSequence = useRef(0)
  const invalidatePlan = () => {
    planSequence.current++
    setPlan(null)
  }
  const [history, setHistory] = useState<Page>({
      runs: [],
      next: 0,
      problems: [],
    }),
    [historyFilters, setHistoryFilters] = useState<Record<string, string>>({}),
    [offset, setOffset] = useState(0)
  const historyQuery = useRef({ filters: historyFilters, offset }),
    historySequence = useRef(0)
  historyQuery.current = { filters: historyFilters, offset }
  const [left, setLeft] = useState(""),
    [right, setRight] = useState(""),
    [dimension, setDimension] = useState("configuration"),
    [comparison, setComparison] = useState<Comparison | null>(null)
  const comparisonSequence = useRef(0)
  const [importFile, setImportFile] = useState<File | null>(null),
    [dependencies, setDependencies] = useState<File[]>([]),
    [bundle, setBundle] = useState(false),
    [confirmDelete, setConfirmDelete] = useState("")
  const [viewedAt, setViewedAt] = useState(Date.now()),
    [now, setNow] = useState(Date.now())
  const invalidateComparison = () => {
    comparisonSequence.current++
    setComparison(null)
  }
  const changeLeft = (value: string) => {
    invalidateComparison()
    setLeft(value)
  }
  const changeRight = (value: string) => {
    invalidateComparison()
    setRight(value)
  }
  const changeDimension = (value: string) => {
    invalidateComparison()
    setDimension(value)
  }
  const compare = async () => {
    const sequence = ++comparisonSequence.current
    setComparison(null)
    setError("")
    try {
      const result = await request<Comparison>("comparisons", {
        leftId: left,
        rightId: right,
        changedDimension: dimension,
      })
      if (sequence === comparisonSequence.current) setComparison(result)
    } catch (e) {
      if (sequence === comparisonSequence.current) setError(errorText(e))
    }
  }
  const suite = catalog.find((s) => s.id === suiteId)
  const available = status?.suites.find((s) => s.id === suiteId)
  const visible = (suite?.cases ?? []).filter(
    (c) =>
      (!filters.workflow || c.workflow === filters.workflow) &&
      (!filters.language || c.language === filters.language) &&
      (!filters.split || c.split === filters.split) &&
      (!filters.category || (c.categories ?? []).includes(filters.category)) &&
      c.id.includes(filters.search),
  )
  const chosen = visible.filter((c) => checked.includes(c.id))
  const attempt = async (work: () => Promise<void>) => {
    setError("")
    try {
      await work()
    } catch (e) {
      setError(errorText(e))
    }
  }
  const refreshStatus = async () => setStatus(await request<Status>("status"))
  const refreshHistory = async () => {
    const sequence = ++historySequence.current
    const current = historyQuery.current
    const query = new URLSearchParams({
      offset: String(current.offset),
      limit: "20",
      ...Object.fromEntries(
        Object.entries(current.filters).filter(([, v]) => v),
      ),
    })
    const page = await request<Page>("runs?" + query)
    if (sequence === historySequence.current) setHistory(page)
  }
  const openRun = (id: string) => {
    // Every selection is a fresh read, even when React retains the same ID.
    // Invalidate in-flight reads immediately, before the next effect cleanup.
    runSequence.current++
    setRunRead((prior) => prior + 1)
    setRunError("")
    caseSequence.current++
    setRunId(id)
    location.hash = "run=" + id
    setCaseEvidence(null)
    setCaseError(null)
    setView(null)
    setEvents(null)
    setConfirmDelete("")
    setViewedAt(Date.now())
  }
  const loadCase = async (identity: string) => {
    const sequence = ++caseSequence.current
    setCaseEvidence(null)
    setCaseError(null)
    try {
      const evidence = await request<CaseEvidence>(
        `runs/${runId}/cases/${encodeURIComponent(identity)}`,
      )
      if (sequence === caseSequence.current) setCaseEvidence(evidence)
    } catch (e) {
      if (sequence === caseSequence.current)
        setCaseError({ identity, message: errorText(e) })
    }
  }
  useEffect(() => {
    void attempt(async () => {
      const result = await request<{ suites: Suite[] }>("suites")
      setCatalog(result.suites)
      await refreshStatus()
    })
  }, [])
  useEffect(() => {
    void attempt(() => refreshHistory())
  }, [offset, historyFilters])
  useEffect(() => {
    if (!runId) return
    const sequence = runSequence.current
    const current = () => !stopped && sequence === runSequence.current
    let stopped = false,
      timer: ReturnType<typeof setTimeout> | undefined
    let epoch = "",
      cursor = 0
    const poll = async () => {
      try {
        const result = await request<View>(`runs/${runId}/view`)
        if (!current()) return
        setRunError("")
        setView(result)
        setNow(Date.now())
        // Imports have no live/checkpoint journal. They never start execution.
        if (result.engineState) {
          try {
            const page = await request<EventPage>(
              `runs/${runId}/events?after=${cursor}&limit=100${
                epoch ? "&epoch=" + encodeURIComponent(epoch) : ""
              }`,
            )
            if (!current()) return
            epoch = page.epoch
            cursor = page.next
            setEvents((prior) => ({
              ...page,
              events: [
                ...(prior?.epoch === page.epoch ? prior.events : []),
                ...page.events,
              ].slice(-100),
            }))
          } catch (e) {
            if (!current()) return
            epoch = ""
            cursor = 0
            setRunError(
              "Stored events unavailable. Retry reloads stored evidence; execution is not restarted. " +
                errorText(e),
            )
          }
        }
        if (active(result)) timer = setTimeout(poll, 1000)
        else {
          void attempt(refreshHistory)
          void attempt(refreshStatus)
        }
      } catch (e) {
        if (current()) setRunError("Stored run unavailable: " + errorText(e))
      }
    }
    void poll()
    return () => {
      stopped = true
      if (timer) clearTimeout(timer)
    }
  }, [runId, runRead])
  const changeSuite = (id: string) => {
    const s = catalog.find((s) => s.id === id)
    setSuite(id)
    setMode(s?.modes[0] ?? "")
    setConfiguration(s?.configurations[0] ?? "")
    setChecked([])
    setFilters({
      workflow: "",
      language: "",
      category: "",
      split: "",
      search: "",
    })
    invalidatePlan()
  }
  const filter = (key: keyof typeof filters, value: string) => {
    setFilters({ ...filters, [key]: value })
    invalidatePlan()
  }
  const preview = async () => {
    const sequence = ++planSequence.current
    setBusy(true)
    setError("")
    try {
      const next = await request<Plan>("plans", {
        selection: {
          suite: suiteId,
          mode,
          configuration,
          caseIds: chosen.map((c) => c.id),
        },
        caps,
      })
      if (sequence === planSequence.current) {
        setPlan(next)
        setToken(identity())
        setConsumed(false)
      }
    } catch (e) {
      if (sequence === planSequence.current) setError(errorText(e))
    } finally {
      setBusy(false)
    }
  }
  const start = () =>
    attempt(async () => {
      if (!plan || consumed) return
      setBusy(true)
      setConsumed(true)
      try {
        const result = await request<{ id: string }>("runs", {
          plan,
          idempotencyToken: token,
        })
        openRun(result.id)
        await refreshStatus()
      } catch (e) {
        setNotice(
          "Start response unavailable or rejected. Inspect history before planning another run; no automatic retry occurs.",
        )
        throw e
      } finally {
        setBusy(false)
      }
    })
  const importEvidence = () =>
    attempt(async () => {
      if (!importFile) return
      setBusy(true)
      try {
        if (
          dependencies.reduce((n, f) => n + f.size, importFile.size) >
          8 * 1024 * 1024
        )
          throw new Error("Combined synthetic import limit is 8 MiB.")
        const original = await bytes(importFile)
        const body = bundle
          ? { exportBundle: original }
          : {
              originalBytes: original,
              dependencies: await Promise.all(dependencies.map(bytes)),
            }
        const result = await request<{ id: string }>("imports", body)
        openRun(result.id)
        await refreshHistory()
        await refreshStatus()
      } finally {
        setBusy(false)
      }
    })
  return (
    <main>
      <header>
        <p className="muted">CAREEROS · LOCAL EVALUATION</p>
        <h1>AI Evaluation Lab</h1>
        <p>
          Inspect synthetic evidence, run selected offline or controlled cases,
          and compare compatible results.
        </p>
        <p className="muted">
          Private local history · One active run · No live provider control · No
          automatic retries
        </p>
      </header>
      {error && (
        <div role="alert" className="notice error">
          {error}
        </div>
      )}
      {notice && (
        <div role="status" className="notice">
          {notice}
        </div>
      )}
      <section aria-labelledby="health-title">
        <h2 id="health-title">Launch and storage</h2>
        {status ? (
          <>
            <p>
              {status.storageReady
                ? "Private history ready"
                : "Storage unavailable — starting another run is blocked"}
              . {status.usage.bytes.toLocaleString()} bytes /{" "}
              {status.usage.files} inventory entries.
            </p>
            {status.usage.softWarning && (
              <p className="notice">
                Disk usage reached the{" "}
                {status.usage.softWarningBytes.toLocaleString()} byte soft
                warning. Delete only evidence you deliberately select; retention
                is never automatic.
              </p>
            )}
            <Json
              label="Exact build and local runtime evidence"
              value={status.build}
            />
          </>
        ) : (
          <p>Checking local capabilities…</p>
        )}
        <button onClick={() => void attempt(refreshStatus)}>
          Refresh storage
        </button>
        <p className="muted">
          Existing Go, Node and browser tests use fixed selectors and private
          process isolation. Runtime preflight is shown below. Provider, human
          review, physical mobile and actual-PDF observations are not inferred
          from controlled results.
        </p>
        {status?.processCapabilities && (
          <Json
            label="Isolated test runtime preflight and coverage gaps"
            value={status.processCapabilities}
          />
        )}
      </section>
      <section aria-labelledby="composer-title">
        <h2 id="composer-title">Compose a run</h2>
        <div className="filters">
          <label>
            <span>Suite</span>
            <select
              value={suiteId}
              onChange={(e) => changeSuite(e.target.value)}
            >
              {catalog.map((s) => (
                <option key={s.id}>{s.id}</option>
              ))}
            </select>
          </label>
          <label>
            <span>Mode</span>
            <select
              value={mode}
              onChange={(e) => {
                setMode(e.target.value)
                invalidatePlan()
              }}
            >
              {suite?.modes.map((v) => <option key={v}>{v}</option>)}
            </select>
          </label>
          <label>
            <span>Configuration</span>
            <select
              value={configuration}
              onChange={(e) => {
                setConfiguration(e.target.value)
                invalidatePlan()
              }}
            >
              {suite?.configurations.map((v) => <option key={v}>{v}</option>)}
            </select>
          </label>
        </div>
        {available && !available.available && (
          <p className="notice">{available.reason}</p>
        )}
        <p className="muted">
          {suite?.assertionSemantics} · Required tools:{" "}
          {suite?.requiredTools.join(", ")}
        </p>
        <div className="filters">
          <Select
            label="Workflow"
            value={filters.workflow}
            values={unique((suite?.cases ?? []).map((c) => c.workflow))}
            onChange={(v) => filter("workflow", v)}
          />
          <Select
            label="Language"
            value={filters.language}
            values={unique((suite?.cases ?? []).map((c) => c.language))}
            onChange={(v) => filter("language", v)}
          />
          <Select
            label="Category (fixture tags)"
            value={filters.category}
            values={unique(
              (suite?.cases ?? []).flatMap((c) => c.categories ?? []),
            )}
            onChange={(v) => filter("category", v)}
          />
          <Select
            label="Split"
            value={filters.split}
            values={unique((suite?.cases ?? []).map((c) => c.split))}
            onChange={(v) => filter("split", v)}
          />
          <label>
            <span>Case ID contains</span>
            <input
              value={filters.search}
              onChange={(e) => filter("search", e.target.value)}
            />
          </label>
        </div>
        <div className="actions">
          <button
            onClick={() => {
              setChecked(visible.map((c) => c.id))
              invalidatePlan()
            }}
          >
            Select filtered cases
          </button>
          <button
            onClick={() => {
              setChecked([])
              invalidatePlan()
            }}
          >
            Clear selection
          </button>
          <span>
            {chosen.length} selected / {visible.length} visible
          </span>
        </div>
        <p className="muted">
          Category filters use supplied fixture tags. Untagged fixtures have
          unavailable category metadata.
        </p>
        <div className="table">
          <table>
            <thead>
              <tr>
                <th>Case</th>
                <th>Workflow / language</th>
                <th>Split / exposure</th>
                <th>Required legs / max simulated calls</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((c) => (
                <tr key={c.id}>
                  <td>
                    <label className="case-choice">
                      <input
                        type="checkbox"
                        aria-label={"Select " + c.id}
                        checked={checked.includes(c.id)}
                        onChange={(e) => {
                          setChecked(
                            e.target.checked
                              ? [...checked, c.id]
                              : checked.filter((id) => id !== c.id),
                          )
                          invalidatePlan()
                        }}
                      />
                      {c.id}
                    </label>
                  </td>
                  <td>
                    {c.workflow}
                    <br />
                    {c.language}
                  </td>
                  <td>
                    {c.split}
                    <br />
                    {c.exposureGroup}
                  </td>
                  <td>
                    {c.requiredSteps} / {c.maximumCalls}
                    {suiteId === "test-adapters" && (
                      <p className="muted">
                        {(() => {
                          const capability =
                            status?.processCapabilities?.selectors?.find(
                              (item) => item.id === c.id,
                            )
                          if (!capability)
                            return "Process preflight unavailable"
                          const availability = capability.available
                            ? "runtime available"
                            : capability.unavailableReasons.join("; ")
                          return `${capability.requiredTools.join(", ")} · ${availability}`
                        })()}
                      </p>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <details>
          <summary>Explicit execution caps</summary>
          <div className="filters">
            {(Object.keys(caps) as Array<keyof Caps>).map((key) => (
              <label key={key}>
                <span>{key}</span>
                <input
                  type="number"
                  min="1"
                  value={caps[key]}
                  onChange={(e) => {
                    setCaps({ ...caps, [key]: Number(e.target.value) })
                    invalidatePlan()
                  }}
                />
              </label>
            ))}
          </div>
        </details>
        <div className="actions">
          <button
            disabled={
              busy ||
              !chosen.length ||
              !available?.available ||
              !status?.storageReady ||
              active(view)
            }
            onClick={() => void preview()}
          >
            Preview frozen plan
          </button>
        </div>
        {plan && (
          <article aria-label="Frozen plan">
            <h3>Frozen plan</h3>
            <p>
              {plan.caseOrder.length} logical cases ·{" "}
              {plan.requiredProcessAttempts ?? 0} fixed test drivers ·{" "}
              {plan.requiredHttpAttempts} required HTTP legs · up to{" "}
              {plan.maximumPlannedProviderCalls} simulated dispatches ·
              concurrency 1
            </p>
            <p className="identity">
              Plan {plan.id}
              <br />
              SHA-256 {plan.hash}
            </p>
            <Json
              label="Frozen inputs, versions, order, caps and unavailable provenance"
              value={plan}
            />
            <button
              disabled={
                busy || consumed || active(view) || !status?.storageReady
              }
              onClick={() => void start()}
            >
              Start controlled / offline run
            </button>
          </article>
        )}
      </section>
      <section aria-labelledby="import-title">
        <h2 id="import-title">Import synthetic evidence</h2>
        <p className="muted">
          Choose explicit local JSON bytes (8 MiB combined). Unknown artifacts
          are quarantined; recognized schemas do not certify sensitivity.
          Historical originals remain immutable.
        </p>
        <label>
          <span>Report or exported JSON bundle</span>
          <input
            type="file"
            accept=".json,application/json"
            onChange={(e) => setImportFile(e.target.files?.[0] ?? null)}
          />
        </label>
        <label className="case-choice my-3">
          <input
            type="checkbox"
            checked={bundle}
            onChange={(e) => setBundle(e.target.checked)}
          />
          This file is a Lab export bundle
        </label>
        <label>
          <span>Approved fixture dependencies (optional)</span>
          <input
            type="file"
            multiple
            accept=".json,application/json"
            disabled={bundle}
            onChange={(e) => setDependencies(Array.from(e.target.files ?? []))}
          />
        </label>
        <button
          disabled={!importFile || busy}
          onClick={() => void importEvidence()}
        >
          Import selected bytes
        </button>
      </section>
      <section aria-labelledby="history-title">
        <h2 id="history-title">History</h2>
        <p className="muted">
          Filters apply before pagination. Older history without indexed
          metadata has unavailable fields and will not match those filters.
          Versions and model filters use exact observed values.
        </p>
        <div className="filters">
          {[
            "suite",
            "workflow",
            "language",
            "category",
            "mode",
            "outcome",
            "model",
            "returnedModel",
            "version",
          ].map((key) => (
            <label key={key}>
              <span>
                {key === "model"
                  ? "Configured model"
                  : key === "returnedModel"
                    ? "Returned model"
                    : key}
              </span>
              <input
                aria-label={"History " + key}
                value={historyFilters[key] ?? ""}
                onChange={(e) => {
                  setHistoryFilters({
                    ...historyFilters,
                    [key]: e.target.value,
                  })
                  setOffset(0)
                }}
              />
            </label>
          ))}
        </div>
        <div className="actions">
          <button onClick={() => void attempt(() => refreshHistory())}>
            Refresh history
          </button>
          <button disabled={!offset} onClick={() => setOffset(0)}>
            First page
          </button>
          <button
            disabled={!history.next}
            onClick={() => setOffset(history.next)}
          >
            Next page
          </button>
          <span>Page starts at {offset}</span>
        </div>
        {history.problems.length > 0 && (
          <Json
            label="History recovery problems"
            value={history.problems}
            open
          />
        )}
        <div className="table">
          <table>
            <thead>
              <tr>
                <th>Stored evidence</th>
                <th>Origin / configuration</th>
                <th>Engine / assertion</th>
                <th>Coverage / model metadata</th>
                <th>Compare</th>
              </tr>
            </thead>
            <tbody>
              {history.runs.map((r) => (
                <tr key={r.id}>
                  <td>
                    <button onClick={() => openRun(r.id)}>{r.suite}</button>
                    <div className="identity">{r.id}</div>
                  </td>
                  <td>
                    {r.mode}
                    <br />
                    {r.configuration || "unavailable"}
                    {r.quarantined && <strong> · Quarantined</strong>}
                  </td>
                  <td>
                    {r.engineState} / {r.assertionVerdict}
                  </td>
                  <td>
                    {r.cases} cases ·{" "}
                    {r.languages?.join(", ") || "language unavailable"}
                    <br />
                    {r.configuredModels?.join(", ") ||
                      "configured model unavailable"}{" "}
                    →{" "}
                    {r.returnedModels?.join(", ") ||
                      "returned model unavailable"}
                    <details>
                      <summary>Indexed versions and categories</summary>
                      <pre>
                        {JSON.stringify(
                          {
                            versions: r.versions,
                            categories: r.categories,
                            workflows: r.workflows,
                          },
                          null,
                          2,
                        )}
                      </pre>
                    </details>
                  </td>
                  <td>
                    <button onClick={() => changeLeft(r.id)}>Set left</button>{" "}
                    <button onClick={() => changeRight(r.id)}>Set right</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {!history.runs.length && <p>No matching stored evidence.</p>}
      </section>
      {runId && (
        <section aria-labelledby="run-title">
          <h2 id="run-title">Selected run</h2>
          <p className="muted">
            Historical outcomes use versioned normalizers; original bytes stay
            separate from derived interpretations. Operational completion does
            not establish semantic support or client acceptance.
          </p>
          <p className="identity">{runId}</p>
          {runError && (
            <div role="alert" className="notice error">
              <p>{runError}</p>
              <button onClick={() => openRun(runId)}>Retry</button>
              <p>Reload stored evidence only; execution is not restarted.</p>
            </div>
          )}
          {view ? (
            <>
              <p>
                Engine {view.engineState ?? "historical import"} · assertion{" "}
                {view.assertionVerdict ?? "unavailable"} ·{" "}
                {view.durable
                  ? "durable local snapshot"
                  : "pending / not durable"}{" "}
                · origin{" "}
                {view.selection?.mode ?? view.originMode ?? "unavailable"}
              </p>
              {view.quarantined && (
                <p className="notice">
                  Quarantined: trusted metrics, comparison and export may be
                  unavailable. Import is not authorization to run.
                </p>
              )}
              {active(view) && (
                <p role="status">
                  Polling about once per second. Observed since this view
                  opened: {Math.round((now - viewedAt) / 1000)} s.
                </p>
              )}
              <p>
                Native sequence elapsed:{" "}
                {view.elapsedMs === undefined
                  ? "Unavailable"
                  : (view.elapsedMs / 1000).toFixed(3) + " s"}
                ; excludes final terminal persistence. Start:{" "}
                {view.startedAt ?? "unavailable for historical imports"}.
              </p>
              {view.progress && (
                <p>
                  Logical cases {view.progress.finishedCases}/
                  {view.progress.plannedCases} finished;{" "}
                  {view.progress.startedCases} started. HTTP legs{" "}
                  {view.progress.finishedHttpAttempts}/
                  {view.progress.plannedHttpAttempts} finished;{" "}
                  {view.progress.attemptedHttpAttempts} attempted;{" "}
                  {view.progress.unreachedHttpAttempts} unreached. Provider
                  dispatches attempted {view.progress.attemptedDispatches}.
                  {(view.progress.plannedProcesses ?? 0) > 0 && (
                    <>
                      {" "}
                      Test drivers: {view.progress.finishedProcesses ?? 0}/
                      {view.progress.plannedProcesses} finished;{" "}
                      {view.progress.attemptedProcesses ?? 0} attempted;{" "}
                      {view.progress.unreachedProcesses ?? 0} unreached. Nested
                      test/process totals are unavailable.
                    </>
                  )}
                </p>
              )}
              <div className="actions">
                <button
                  disabled={!active(view)}
                  onClick={() =>
                    void attempt(async () => {
                      await request(`runs/${runId}/cancel`, {})
                      setNotice(
                        "Cancellation requested; waiting for terminal retention.",
                      )
                    })
                  }
                >
                  Cancel active run
                </button>
                {["json", "markdown", "original", "diagnostics"].map(
                  (format) => (
                    <button
                      key={format}
                      onClick={() =>
                        void attempt(() => download(runId, format))
                      }
                    >
                      Export {format}
                    </button>
                  ),
                )}
              </div>
              <Json
                label="Versions, provenance, caps, raw/derived origin and unavailable evidence"
                value={{
                  version: view.version,
                  originalVersion: view.originalVersion,
                  originalHash: view.originalHash,
                  provenance: view.provenance,
                  context: view.context,
                  derivations: view.derivations ?? view.derivation,
                  unavailable: view.unavailableReasons,
                }}
              />
              <Json
                label="Observed stage events / durable restart checkpoints"
                value={events}
                open
              />
              <h3>Cases · lazy evidence</h3>
              <div className="table">
                <table>
                  <thead>
                    <tr>
                      <th>Case / language</th>
                      <th>Assertion / operational outcome</th>
                      <th>Missing or blocked evidence</th>
                    </tr>
                  </thead>
                  <tbody>
                    {view.cases.map((c) => (
                      <tr key={c.identity}>
                        <td>
                          <button onClick={() => void loadCase(c.identity)}>
                            {c.id}
                          </button>
                          <br />
                          {c.language}
                        </td>
                        <td>
                          {c.assertionVerdict} / {c.operationalOutcome}
                        </td>
                        <td>
                          {c.unavailableReasons?.join("; ") ||
                            "No additional unavailable reason retained"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {caseError && (
                <div
                  role="alert"
                  aria-label="Case loading error"
                  className="notice error"
                >
                  <p>{caseError.message}</p>
                  <button onClick={() => void loadCase(caseError.identity)}>
                    Retry case
                  </button>
                  <p>
                    Reload stored case evidence only; execution is not
                    restarted.
                  </p>
                </div>
              )}
              {caseEvidence && (
                <div key={runId + ":" + caseEvidence.identity}>
                  <CaseDetail evidence={caseEvidence} />
                  <SourceReview
                    runId={runId}
                    caseId={caseEvidence.identity}
                    reviewer={status?.humanReviewer}
                  />
                </div>
              )}
              <h3>Metric opportunities and missingness</h3>
              <MetricTable
                metrics={view.metrics ?? []}
                definitions={status?.metricDefinitions ?? []}
              />
              <details>
                <summary>Deliberately delete this evidence</summary>
                <p>
                  Deletion removes this local run and its artifacts. Export
                  anything you need first. Active runs must reach terminal
                  retention.
                </p>
                <label>
                  <span>Type exact run ID to confirm deletion</span>
                  <input
                    value={confirmDelete}
                    onChange={(e) => setConfirmDelete(e.target.value)}
                  />
                </label>
                <button
                  disabled={active(view) || confirmDelete !== runId}
                  onClick={() =>
                    void attempt(async () => {
                      await request(`runs/${runId}/delete`, {
                        confirmId: confirmDelete,
                      })
                      runSequence.current++
                      caseSequence.current++
                      setRunError("")
                      setRunId("")
                      setView(null)
                      location.hash = ""
                      setNotice("Selected local evidence deleted.")
                      await refreshHistory()
                      await refreshStatus()
                    })
                  }
                >
                  Delete confirmed evidence
                </button>
              </details>
            </>
          ) : !runError ? (
            <p role="status">
              Reading the existing run; execution is not restarted.
            </p>
          ) : null}
        </section>
      )}
      <section aria-labelledby="compare-title">
        <h2 id="compare-title">Compare stored runs</h2>
        <div className="filters">
          <label>
            <span>Left run ID</span>
            <input
              value={left}
              onChange={(e) => {
                changeLeft(e.target.value)
                setComparison(null)
              }}
            />
          </label>
          <label>
            <span>Right run ID</span>
            <input
              value={right}
              onChange={(e) => {
                changeRight(e.target.value)
                setComparison(null)
              }}
            />
          </label>
          <label>
            <span>Changed dimension</span>
            <select
              value={dimension}
              onChange={(e) => {
                changeDimension(e.target.value)
                setComparison(null)
              }}
            >
              {[
                "configuration",
                "source",
                "prompt",
                "schema",
                "scorer",
                "strategy",
                "budget",
              ].map((d) => (
                <option key={d}>{d}</option>
              ))}
            </select>
          </label>
        </div>
        <button
          disabled={!left || !right || left === right}
          onClick={() => void compare()}
        >
          Compare safety and omission evidence
        </button>
        {comparison && (
          <>
            <p className="notice">
              {comparison.compatibility.compatible
                ? "Compatible finite evidence"
                : "Incompatible: " +
                  comparison.compatibility.reasons.join("; ")}
              . {comparison.partial ? "Partial case overlap. " : ""}
              {comparison.pooling}
            </p>
            <p className="muted">
              Safety, omission and regression deltas are shown before cost or
              speed. Curated forbidden matches are not confirmed semantic
              judgments. Cost, causal attribution and human acceptance remain
              unavailable without their evidence.
            </p>
            <Json
              label="Per-case and per-language safety, omission and relationship deltas"
              value={comparison.deltas}
              open
            />
          </>
        )}
      </section>
      <footer className="muted">
        Controlled, replay, historical provider, browser and actual-artifact
        evidence are distinct. Completion, regex agreement and durable storage
        do not establish factual support or user acceptance.
      </footer>
    </main>
  )
}
