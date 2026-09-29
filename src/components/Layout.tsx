import { useState } from 'react'
import { Page } from '../lib/types'
import { useStore } from '../lib/store'

interface Props {
  page: Page
  setPage: (p: Page) => void
  children: React.ReactNode
}

const NAV_ITEMS: { id: Page; label: string }[] = [
  { id: 'home', label: 'Apply' },
  { id: 'repository', label: 'Profile' },
  { id: 'cv', label: 'CV' },
  { id: 'results', label: 'Results' },
]

export default function Layout({ page, setPage, children }: Props) {
  const { state, dispatch } = useStore()
  const [showKey, setShowKey] = useState(false)
  const [keyDraft, setKeyDraft] = useState(state.apiKey)

  function saveKey() {
    dispatch({ type: 'SET_API_KEY', payload: keyDraft.trim() })
    setShowKey(false)
  }

  if (page === 'landing') {
    return <div className="min-h-screen" style={{ backgroundColor: 'var(--color-bg)', color: 'var(--color-fg)' }}>{children}</div>
  }

  return (
    <div className="min-h-screen flex flex-col" style={{ backgroundColor: 'var(--color-bg)', color: 'var(--color-fg)' }}>
      <header
        className="sticky top-0 z-50 border-b"
        style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-bg)' }}
      >
        <div className="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between gap-8">
          {/* Wordmark */}
          <div className="flex items-center gap-2 flex-shrink-0">
            <div className="w-5 h-5 flex-shrink-0" style={{ backgroundColor: 'var(--color-accent)' }} />
            <span
              className="text-base font-bold tracking-[0.12em] uppercase select-none"
              style={{ fontFamily: 'var(--font-display)' }}
            >
              CareerOS
            </span>
          </div>

          {/* Nav */}
          <nav className="flex items-center gap-1">
            {NAV_ITEMS.map(({ id, label }) => (
              <button
                key={id}
                onClick={() => setPage(id)}
                className="px-4 py-1.5 text-xs uppercase tracking-[0.18em] transition-colors"
                style={{
                  fontFamily: 'var(--font-mono)',
                  backgroundColor: page === id ? 'var(--color-fg)' : 'transparent',
                  color: page === id ? 'var(--color-bg)' : 'var(--color-muted-fg)',
                }}
              >
                {label}
              </button>
            ))}
          </nav>

          {/* API key toggle */}
          <button
            onClick={() => { setKeyDraft(state.apiKey); setShowKey(v => !v) }}
            className="text-xs px-3 py-1.5 border transition-colors flex-shrink-0"
            style={{
              fontFamily: 'var(--font-mono)',
              borderColor: state.apiKey ? 'var(--color-border)' : 'var(--color-accent)',
              color: state.apiKey ? 'var(--color-muted-fg)' : 'var(--color-accent)',
            }}
          >
            {state.apiKey ? 'API KEY ✓' : '! SET API KEY'}
          </button>
        </div>

        {showKey && (
          <div className="border-t" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-card)' }}>
            <div className="max-w-6xl mx-auto px-6 py-3 flex items-center gap-4">
              <span
                className="text-xs uppercase tracking-widest flex-shrink-0"
                style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
              >
                Anthropic API Key
              </span>
              <input
                type="password"
                value={keyDraft}
                onChange={e => setKeyDraft(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && saveKey()}
                placeholder="sk-ant-api03-..."
                className="flex-1 text-sm bg-transparent border-b py-1 focus:outline-none transition-colors"
                style={{
                  fontFamily: 'var(--font-mono)',
                  borderColor: 'var(--color-border)',
                }}
                autoFocus
              />
              <button
                onClick={saveKey}
                className="text-xs uppercase tracking-widest px-4 py-1.5 transition-opacity hover:opacity-75 flex-shrink-0"
                style={{
                  fontFamily: 'var(--font-mono)',
                  backgroundColor: 'var(--color-fg)',
                  color: 'var(--color-bg)',
                }}
              >
                Save
              </button>
            </div>
          </div>
        )}
      </header>

      <main className="flex-1">{children}</main>
    </div>
  )
}
