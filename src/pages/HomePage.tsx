import { useStore } from '../lib/store'
import { generateMaterials } from '../lib/ai'
import { Page } from '../lib/types'

interface Props {
  setPage: (p: Page) => void
}

function StatusDot({ ok }: { ok: boolean }) {
  return (
    <span
      className="inline-block w-1.5 h-1.5 rounded-full flex-shrink-0"
      style={{ backgroundColor: ok ? 'var(--color-fg)' : 'var(--color-border)' }}
    />
  )
}

function StatusRow({ label, ok, detail }: { label: string; ok: boolean; detail: string }) {
  return (
    <div className="flex items-start gap-3 py-3 border-b" style={{ borderColor: 'var(--color-border)' }}>
      <StatusDot ok={ok} />
      <div className="flex-1 min-w-0">
        <span
          className="text-xs uppercase tracking-[0.18em] block"
          style={{ fontFamily: 'var(--font-mono)', color: ok ? 'var(--color-fg)' : 'var(--color-muted-fg)' }}
        >
          {label}
        </span>
        <span
          className="text-xs block mt-0.5 truncate"
          style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
        >
          {detail}
        </span>
      </div>
    </div>
  )
}

export default function HomePage({ setPage }: Props) {
  const { state, dispatch } = useStore()
  const jobPosting = state.jobPosting

  const hasRepo = !!(state.repository.careerGoals || state.repository.experience.length || state.repository.skills)
  const hasPosting = jobPosting.trim().length > 30
  const canGenerate = hasRepo && hasPosting && !!state.apiKey && !state.isGenerating

  async function handleGenerate() {
    if (!canGenerate) return
    dispatch({ type: 'SET_GENERATING', payload: true })
    try {
      const materials = await generateMaterials(state.repository, jobPosting, state.apiKey)
      dispatch({ type: 'SET_MATERIALS', payload: materials })
      setPage('results')
    } catch (e: any) {
      alert(e.message || 'Generation failed. Check your API key and try again.')
    } finally {
      dispatch({ type: 'SET_GENERATING', payload: false })
    }
  }

  return (
    <div className="max-w-6xl mx-auto px-6 py-14">
      {/* Header */}
      <div className="mb-12">
        <p
          className="text-xs uppercase tracking-[0.3em] mb-5"
          style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
        >
          Step 01 — Opportunity Input
        </p>
        <h1
          className="text-7xl font-bold uppercase leading-[0.9] tracking-tight mb-5"
          style={{ fontFamily: 'var(--font-display)' }}
        >
          Find Your<br />
          <span style={{ color: 'var(--color-accent)' }}>Next Role</span>
        </h1>
        <p className="text-sm max-w-md leading-relaxed" style={{ color: 'var(--color-muted-fg)' }}>
          Paste a job posting URL, full job description, or any text about the opportunity.
          The AI will analyze it and generate tailored application materials from your profile.
        </p>
      </div>

      <div className="grid grid-cols-[1fr_300px] gap-12 items-start">
        {/* Textarea */}
        <div>
          <label
            className="text-xs uppercase tracking-[0.2em] block mb-2"
            style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
          >
            Job Posting or Description
          </label>
          <textarea
            value={jobPosting}
            onChange={e => dispatch({ type: 'SET_JOB_POSTING', payload: e.target.value })}
            placeholder="Paste the full job description, URL, or any text about the role you're applying to. Include requirements, responsibilities, company info — the more detail, the better the tailoring."
            className="w-full h-[440px] text-sm p-4 resize-none focus:outline-none transition-colors leading-relaxed"
            style={{
              fontFamily: 'var(--font-mono)',
              border: '1px solid var(--color-border)',
              backgroundColor: 'var(--color-card)',
              color: 'var(--color-fg)',
            }}
            onFocus={e => (e.target.style.borderColor = 'var(--color-fg)')}
            onBlur={e => (e.target.style.borderColor = 'var(--color-border)')}
          />
          <div
            className="flex justify-between items-center mt-2 text-xs"
            style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
          >
            <span>{jobPosting.trim().length} characters</span>
            {jobPosting && (
              <button
                onClick={() => dispatch({ type: 'SET_JOB_POSTING', payload: '' })}
                className="hover:opacity-60 transition-opacity"
              >
                Clear
              </button>
            )}
          </div>
        </div>

        {/* Sidebar */}
        <div className="pt-6">
          <p
            className="text-xs uppercase tracking-[0.2em] mb-1"
            style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
          >
            Checklist
          </p>
          <div className="mb-8">
            <StatusRow
              label="API Key"
              ok={!!state.apiKey}
              detail={state.apiKey ? 'Configured' : 'Click "! SET API KEY" above'}
            />
            <StatusRow
              label="Profile"
              ok={hasRepo}
              detail={hasRepo ? 'Repository ready' : 'Go to Profile and add your info'}
            />
            <StatusRow
              label="Posting"
              ok={hasPosting}
              detail={hasPosting ? `${jobPosting.trim().length} chars` : 'Paste job details on the left'}
            />
          </div>

          <button
            onClick={handleGenerate}
            disabled={!canGenerate}
            className="w-full py-4 text-sm uppercase tracking-[0.2em] font-bold transition-colors"
            style={{
              fontFamily: 'var(--font-display)',
              backgroundColor: canGenerate ? 'var(--color-fg)' : 'var(--color-muted)',
              color: canGenerate ? 'var(--color-bg)' : 'var(--color-muted-fg)',
              cursor: canGenerate ? 'pointer' : 'not-allowed',
            }}
          >
            {state.isGenerating ? 'Generating...' : 'Generate Materials'}
          </button>

          {state.isGenerating && (
            <p
              className="text-xs text-center mt-3 leading-relaxed"
              style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
            >
              Analyzing opportunity and tailoring your application — this may take 20–40 seconds.
            </p>
          )}

          <div className="mt-8 pt-6 border-t" style={{ borderColor: 'var(--color-border)' }}>
            <p
              className="text-xs uppercase tracking-[0.2em] mb-4"
              style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
            >
              Output
            </p>
            {['Tailored Résumé', 'Cover Letter', 'Application Q&A'].map((item, i) => (
              <div key={item} className="flex items-center gap-3 mb-3">
                <span
                  className="text-xs w-4"
                  style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-accent)' }}
                >
                  {String(i + 1).padStart(2, '0')}
                </span>
                <span className="text-sm" style={{ color: 'var(--color-fg)' }}>
                  {item}
                </span>
              </div>
            ))}
          </div>

          {state.generatedMaterials && (
            <div className="mt-4 pt-4 border-t" style={{ borderColor: 'var(--color-border)' }}>
              <button
                onClick={() => setPage('results')}
                className="text-xs uppercase tracking-[0.15em] w-full py-2 border transition-colors hover:opacity-75"
                style={{
                  fontFamily: 'var(--font-mono)',
                  borderColor: 'var(--color-fg)',
                  color: 'var(--color-fg)',
                }}
              >
                View Previous Results →
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
