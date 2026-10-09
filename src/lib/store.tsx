import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useReducer,
  useRef,
  useState,
  ReactNode,
} from "react"
import { ProfessionalRepository, GeneratedMaterials } from "./types"
import {
  editProfile,
  type ProfileEdit,
  emptyProfileView,
  migrateProfile,
  profileView,
  replaceProfileView,
  type ProfileDocument,
  type ProfileOrigin,
} from "./profileDocument"
import {
  applySectionProposal,
  type SectionReviewRequest,
  type SectionProposal,
} from "./sectionReview"
import {
  openProfile,
  saveProfile,
  PROFILE_KEY,
  LEGACY_KEY,
  ProfileStorageError,
  type ProfileStorageCode,
} from "./profileStorage"
import {
  getInitialLocale,
  isLocale,
  Locale,
  translate,
  TranslationKey,
  TranslationValue,
} from "./i18n"

interface AppState {
  uiLocale: Locale
  cvLanguage: Locale
  repository: ProfessionalRepository
  profileError: ProfileStorageCode | null
  generatedMaterials: GeneratedMaterials | null
  typesafeKey: string
  apiKey: string
  isReviewingRepo: boolean
  isGenerating: boolean
  lastReviewSummary: string
  jobPosting: string
}

type Action = {
  type: "SET_TYPESAFE_KEY"
  payload: string
} | {
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
    case "SET_TYPESAFE_KEY":
      return { ...state, typesafeKey: action.payload }
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

function readSetting(key: string): string {
  try {
    return localStorage.getItem(key) || ""
  } catch {
    return ""
  }
}

function writeSetting(key: string, value: string) {
  try {
    if (value) localStorage.setItem(key, value)
    else localStorage.removeItem(key)
  } catch {
    /* Profile persistence reports its own errors. */
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
  dispatch: (action: Action) => Promise<boolean>
  profileDocument: ProfileDocument | null
  editCanonicalProfile: (edit: ProfileEdit) => Promise<boolean>
  applyProfileProposal: (
    request: SectionReviewRequest,
    proposal: SectionProposal,
    edits: Record<string, string>,
    removals: string[],
  ) => Promise<boolean>
  saveRepository: (
    repo: ProfessionalRepository,
    origin?: ProfileOrigin,
  ) => Promise<boolean>
} | null>(null)

export function StoreProvider({ children }: { children: ReactNode }) {
  const [profileDocument, setProfileDocument] =
    useState<ProfileDocument | null>(null)
  const optimisticDocument = useRef<ProfileDocument | null>(null)
  const documentRef = useRef<ProfileDocument | null>(null)
  const queue = useRef<Promise<boolean>>(Promise.resolve(true))
  const failed = useRef(false)
  const draft = useRef(emptyProfileView())
  const [ready, setReady] = useState(false)
  const [profileError, setProfileError] = useState<ProfileStorageCode | null>(
    null,
  )
  const [state, rawDispatch] = useReducer(reducer, {
    uiLocale: getInitialLocale(),
    cvLanguage: loadCvLanguage(),
    repository: emptyProfileView(),
    profileError: null,
    generatedMaterials: null,
    typesafeKey: readSetting("careeros_typesafe_key"),
    apiKey: readSetting("careeros_apikey"),
    isReviewingRepo: false,
    isGenerating: false,
    lastReviewSummary: "",
    jobPosting: "",
  })

  useEffect(() => {
    let active = true
    Promise.resolve()
      .then(() => openProfile(localStorage))
      .then((doc) => {
        if (!active) return
        optimisticDocument.current = doc
        setProfileDocument(doc)
        documentRef.current = doc
        draft.current = profileView(doc)
        rawDispatch({ type: "SET_REPO", payload: draft.current })
        setReady(true)
      })
      .catch((error) => {
        if (!active) return
        // A valid legacy Profile remains visible when migration cannot persist.
        try {
          const raw = localStorage.getItem(LEGACY_KEY)
          if (localStorage.getItem(PROFILE_KEY) === null && raw !== null) {
            const recovered = migrateProfile(
              JSON.parse(raw),
              crypto.randomUUID(),
            )
            optimisticDocument.current = recovered
            setProfileDocument(recovered)
            draft.current = profileView(recovered)
            rawDispatch({ type: "SET_REPO", payload: draft.current })
          }
        } catch {
          /* Malformed originals are retained for download. */
        }
        failed.current = true
        setProfileError(
          error instanceof ProfileStorageError ? error.code : "storage",
        )
        setReady(true)
      })
    const changed = (event: StorageEvent) => {
      if (event.key !== PROFILE_KEY && event.key !== null) return
      if (
        !documentRef.current ||
        event.newValue === JSON.stringify(documentRef.current)
      )
        return
      // Keep the local draft and review input, but block stale writes until reload.
      failed.current = true
      setProfileError("stale")
    }
    window.addEventListener("storage", changed)
    return () => {
      active = false
      window.removeEventListener("storage", changed)
    }
  }, [])

  const updateCanonical = useCallback(
    (build: (doc: ProfileDocument) => ProfileDocument) => {
      const expected = optimisticDocument.current
      if (!expected) return Promise.resolve(false)
      let candidate: ProfileDocument
      try {
        candidate = build(expected)
      } catch {
        setProfileError("validation")
        return Promise.resolve(false)
      }
      optimisticDocument.current = candidate
      setProfileDocument(candidate)
      draft.current = profileView(candidate)
      rawDispatch({ type: "SET_REPO", payload: draft.current })
      const saving = queue.current.then(async () => {
        if (failed.current || !documentRef.current) return false
        try {
          await saveProfile(localStorage, expected, candidate)
          documentRef.current = candidate
          return true
        } catch (error) {
          failed.current = true
          setProfileError(
            error instanceof ProfileStorageError ? error.code : "validation",
          )
          return false
        }
      })
      queue.current = saving
      return saving
    },
    [],
  )
  const editCanonicalProfile = useCallback(
    (edit: ProfileEdit) => updateCanonical((doc) => editProfile(doc, edit)),
    [updateCanonical],
  )
  const applyProfileProposal = useCallback(
    async (
      request: SectionReviewRequest,
      proposal: SectionProposal,
      edits: Record<string, string>,
      removals: string[],
    ) => {
      const saving = queue.current.then(async () => {
        const expected = documentRef.current
        if (failed.current || !expected) return false
        try {
          const candidate = applySectionProposal(
            expected,
            request,
            proposal,
            edits,
            removals,
          )
          await saveProfile(localStorage, expected, candidate)
          documentRef.current = candidate
          optimisticDocument.current = candidate
          setProfileDocument(candidate)
          draft.current = profileView(candidate)
          rawDispatch({ type: "SET_REPO", payload: draft.current })
          return true
        } catch (error) {
          setProfileError(
            error instanceof ProfileStorageError ? error.code : error instanceof Error && error.message === "stale" ? "stale" : "validation",
          )
          return false
        }
      })
      queue.current = saving
      return saving
    },
    [],
  )
  const saveRepository = useCallback(
    (
      repo: ProfessionalRepository,
      origin: ProfileOrigin = { kind: "manual_edit", original: "user" },
    ) => updateCanonical((doc) => replaceProfileView(doc, repo, origin)),
    [updateCanonical],
  )
  const dispatch = useCallback(
    async (action: Action) => {
      if (action.type === "SET_REPO") return saveRepository(action.payload)
      rawDispatch(action)
      return true
    },
    [saveRepository],
  )

  function downloadRecovery() {
    const data: Record<string, unknown> = {
      unsavedProfile: draft.current,
      unsavedCanonicalProfile: optimisticDocument.current,
    }
    for (const key of [LEGACY_KEY, PROFILE_KEY]) {
      try {
        data[key] = localStorage.getItem(key)
      } catch {
        data[key] = null
      }
    }
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(data, null, 2)], { type: "application/json" }),
    )
    const anchor = document.createElement("a")
    anchor.href = url
    anchor.download = "careeros-profile-recovery.json"
    anchor.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  useEffect(() => {
    writeSetting("careeros_typesafe_key", state.typesafeKey)
  }, [state.typesafeKey])

  useEffect(() => {
    writeSetting("careeros_apikey", state.apiKey)
  }, [state.apiKey])

  useEffect(() => {
    writeSetting("careeros_locale", state.uiLocale)
    document.documentElement.lang = state.uiLocale
  }, [state.uiLocale])

  useEffect(() => {
    writeSetting("careeros_cv_language", state.cvLanguage)
  }, [state.cvLanguage])

  return (
    <StoreContext.Provider
      value={{
        state: { ...state, profileError },
        dispatch,
        saveRepository,
        profileDocument,
        editCanonicalProfile,
        applyProfileProposal,
      }}
    >
      {profileError && (
        <div
          role="alert"
          className="border-b border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-950"
        >
          <p>
            {translate(state.uiLocale, `profile.persistence.${profileError}`)}
          </p>
          <div className="mt-2 flex gap-4">
            <button
              type="button"
              className="underline"
              onClick={downloadRecovery}
            >
              {translate(state.uiLocale, "profile.persistence.download")}
            </button>
            <button
              type="button"
              className="underline"
              onClick={() => window.location.reload()}
            >
              {translate(state.uiLocale, "profile.persistence.reload")}
            </button>
          </div>
        </div>
      )}
      {ready ? (
        children
      ) : (
        <p role="status" className="p-4">
          {translate(state.uiLocale, "profile.persistence.loading")}
        </p>
      )}
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
