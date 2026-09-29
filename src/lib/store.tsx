import { createContext, useContext, useReducer, useEffect, ReactNode } from 'react'
import { ProfessionalRepository, GeneratedMaterials } from './types'

const defaultRepo: ProfessionalRepository = {
  careerGoals: '',
  skills: '',
  competencies: '',
  experience: [],
  tools: '',
  projects: [],
  employmentStatus: 'Employed',
  currentSalary: '',
  desiredSalary: '',
  additionalInfo: '',
}

interface AppState {
  repository: ProfessionalRepository
  generatedMaterials: GeneratedMaterials | null
  apiKey: string
  isReviewingRepo: boolean
  isGenerating: boolean
  lastReviewSummary: string
  jobPosting: string
}

type Action =
  | { type: 'SET_REPO'; payload: ProfessionalRepository }
  | { type: 'SET_MATERIALS'; payload: GeneratedMaterials }
  | { type: 'SET_API_KEY'; payload: string }
  | { type: 'SET_REVIEWING'; payload: boolean }
  | { type: 'SET_GENERATING'; payload: boolean }
  | { type: 'SET_REVIEW_SUMMARY'; payload: string }
  | { type: 'SET_JOB_POSTING'; payload: string }

function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
    case 'SET_REPO': return { ...state, repository: action.payload }
    case 'SET_MATERIALS': return { ...state, generatedMaterials: action.payload }
    case 'SET_API_KEY': return { ...state, apiKey: action.payload }
    case 'SET_REVIEWING': return { ...state, isReviewingRepo: action.payload }
    case 'SET_GENERATING': return { ...state, isGenerating: action.payload }
    case 'SET_REVIEW_SUMMARY': return { ...state, lastReviewSummary: action.payload }
    case 'SET_JOB_POSTING': return { ...state, jobPosting: action.payload }
    default: return state
  }
}

function loadRepo(): ProfessionalRepository {
  try {
    const saved = localStorage.getItem('careeros_repo')
    return saved ? JSON.parse(saved) : defaultRepo
  } catch {
    return defaultRepo
  }
}

const StoreContext = createContext<{
  state: AppState
  dispatch: React.Dispatch<Action>
} | null>(null)

export function StoreProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, {
    repository: loadRepo(),
    generatedMaterials: null,
    apiKey: localStorage.getItem('careeros_apikey') || '',
    isReviewingRepo: false,
    isGenerating: false,
    lastReviewSummary: '',
    jobPosting: '',
  })

  useEffect(() => {
    localStorage.setItem('careeros_repo', JSON.stringify(state.repository))
  }, [state.repository])

  useEffect(() => {
    if (state.apiKey) localStorage.setItem('careeros_apikey', state.apiKey)
  }, [state.apiKey])

  return (
    <StoreContext.Provider value={{ state, dispatch }}>
      {children}
    </StoreContext.Provider>
  )
}

export function useStore() {
  const ctx = useContext(StoreContext)
  if (!ctx) throw new Error('useStore must be inside StoreProvider')
  return ctx
}
