import { useState } from "react"
import { StoreProvider, useStore } from "./lib/store"
import { hasQualificationEvidence } from "./lib/qualificationEvidence"
import { Page } from "./lib/types"
import Layout from "./components/Layout"
import HomePage from "./pages/HomePage"
import RepositoryPage from "./pages/RepositoryPage"
import ResultsPage from "./pages/ResultsPage"
import CvPage from "./pages/CvPage"
import LandingPage from "./pages/LandingPage"
import AccountGate from "./components/AccountGate"
import DraftRecoveryWorkspace from "./components/DraftRecoveryWorkspace"

function AppInner() {
  const { accountId, profileDocument, state } = useStore()
  const [page, setPage] = useState<Page>(() =>
    accountId
      ? hasQualificationEvidence(profileDocument) && !state.profileError
        ? "home"
        : "repository"
      : "landing",
  )

  return (
    <Layout page={page} setPage={setPage}>
      {page === "landing" && <LandingPage setPage={setPage} />}
      {page === "repository" && <RepositoryPage />}
      {page === "cv" && <CvPage />}
      {page === "results" && <ResultsPage setPage={setPage} />}
      {import.meta.env.VITE_CAREEROS_DRAFT_RECOVERY_MODE === "1" ? page === "home" && <DraftRecoveryWorkspace /> : <HomePage
        setPage={setPage}
        active={page === "home"}
        enrichmentActive={page === "results"}
      />}
    </Layout>
  )
}

export default function App() {
  const pilot = import.meta.env.VITE_CAREEROS_PILOT_MODE === "1"
  if (pilot || import.meta.env.VITE_CAREEROS_ACCOUNT_MODE === "1" || import.meta.env.VITE_CAREEROS_DRAFT_RECOVERY_MODE === "1")
    return (
      <AccountGate pilot={pilot}>
        <AppInner />
      </AccountGate>
    )
  return (
    <StoreProvider>
      <AppInner />
    </StoreProvider>
  )
}
