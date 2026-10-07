import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useReducer,
  useRef,
  ReactNode,
} from "react"
import { ProfessionalRepository, GeneratedMaterials } from "./types"
import { emptyContact, withContactFields } from "./profile"
import {
  getInitialLocale,
  isLocale,
  Locale,
  translate,
  TranslationKey,
  TranslationValue,
} from "./i18n"

const defaultRepo: ProfessionalRepository = {
  ...emptyContact,
  careerGoals: "",
  skills: "",
  competencies: "",
  experience: [],
  tools: "",
  projects: [],
  education: [],
  certifications: [],
  languages: [],
  employmentStatus: "",
  currentSalary: "",
  desiredSalary: "",
  additionalInfo: "",
}

interface AppState {
  uiLocale: Locale
  cvLanguage: Locale
  repository: ProfessionalRepository
  generatedMaterials: GeneratedMaterials | null
  apiKey: string
  isReviewingRepo: boolean
  isGenerating: boolean
  lastReviewSummary: string
  jobPosting: string
}

type Action = {
  type: "SET_CV_LANGUAGE"
  payload: Locale
} | {
  type: "SET_UI_LOCALE"
  payload: Locale
} | {
  type: "SET_REPO"
  payload: ProfessionalRepository
} | {
  type: "SET_MATERIALS"
  payload: GeneratedMaterials
} | {
  type: "SET_API_KEY"
  payload: string
} | {
  type: "SET_REVIEWING"
  payload: boolean
} | {
  type: "SET_GENERATING"
  payload: boolean
} | {
  type: "SET_REVIEW_SUMMARY"
  payload: string
} | {
  type: "SET_JOB_POSTING"
  payload: string
}

function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
    case "SET_UI_LOCALE":
      return { ...state, uiLocale: action.payload }
    case "SET_CV_LANGUAGE":
      return { ...state, cvLanguage: action.payload }
    case "SET_REPO":
      return { ...state, repository: action.payload }
    case "SET_MATERIALS":
      return { ...state, generatedMaterials: action.payload }
    case "SET_API_KEY":
      return { ...state, apiKey: action.payload }
    case "SET_REVIEWING":
      return { ...state, isReviewingRepo: action.payload }
    case "SET_GENERATING":
      return { ...state, isGenerating: action.payload }
    case "SET_REVIEW_SUMMARY":
      return { ...state, lastReviewSummary: action.payload }
    case "SET_JOB_POSTING":
      return { ...state, jobPosting: action.payload }
    default:
      return state
  }
}

function loadRepo(): ProfessionalRepository {
  try {
    const saved = localStorage.getItem("careeros_repo")
    return saved ? withContactFields(JSON.parse(saved)) : defaultRepo
  } catch {
    return defaultRepo
  }
}

function loadCvLanguage(): Locale {
  try {
    const saved = localStorage.getItem("careeros_cv_language")
    if (isLocale(saved)) return saved
    const document = JSON.parse(
      localStorage.getItem("careeros_curated_cv_v1") || "null",
    )
    const previous = document?.cvLanguage ?? document?.locale
    if (isLocale(previous)) return previous
  } catch {
    /* Use the initial site language for a new CV preference. */
  }
  return getInitialLocale()
}

const StoreContext = createContext<{
  state: AppState
  dispatch: React.Dispatch<Action>
} | null>(null)

export function StoreProvider({ children }: { children: ReactNode }) {
  const lastPersistedRepo = useRef<string | null>(
    localStorage.getItem("careeros_repo"),
  )
  const [state, dispatch] = useReducer(reducer, {
    uiLocale: getInitialLocale(),
    cvLanguage: loadCvLanguage(),
    repository: loadRepo(),
    generatedMaterials: null,
    apiKey: localStorage.getItem("careeros_apikey") || "",
    isReviewingRepo: false,
    isGenerating: false,
    lastReviewSummary: "",
    jobPosting: "",
  })

  useEffect(() => {
    const serialized = JSON.stringify(state.repository)
    if (
      lastPersistedRepo.current === null &&
      serialized === JSON.stringify(defaultRepo)
    )
      return
    if (lastPersistedRepo.current !== serialized) {
      localStorage.setItem("careeros_repo", serialized)
      lastPersistedRepo.current = serialized
    }
  }, [state.repository])

  useEffect(() => {
    if (state.apiKey) localStorage.setItem("careeros_apikey", state.apiKey)
  }, [state.apiKey])

  useEffect(() => {
    localStorage.setItem("careeros_locale", state.uiLocale)
    document.documentElement.lang = state.uiLocale
  }, [state.uiLocale])

  useEffect(() => {
    localStorage.setItem("careeros_cv_language", state.cvLanguage)
  }, [state.cvLanguage])

  return (
    <StoreContext.Provider value={{ state, dispatch }}>
      {children}
    </StoreContext.Provider>
  )
}

export function useStore() {
  const ctx = useContext(StoreContext)
  if (!ctx) throw new Error("useStore must be inside StoreProvider")
  return ctx
}

export function useI18n() {
  const { state, dispatch } = useStore()
  const t = useCallback(
    (key: TranslationKey, values?: Record<string, TranslationValue>) =>
      translate(state.uiLocale, key, values),
    [state.uiLocale],
  )
  const setUiLocale = useCallback(
    (locale: Locale) => dispatch({ type: "SET_UI_LOCALE", payload: locale }),
    [dispatch],
  )

  return { uiLocale: state.uiLocale, setUiLocale, t }
}
