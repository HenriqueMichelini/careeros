import { useEffect, useRef, useState } from "react"
import { submissionSession, releaseSubmissionSession } from "./lib/ingestionSubmission"
import { forgetManagedIngestion } from "./lib/managedIngestion"
import { forgetManagedProfileReview } from "./lib/managedProfileReview"
import { forgetManagedStandaloneCV } from "./lib/managedStandaloneCV"
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
import PilotSupport from "./components/PilotSupport"
import DraftRecoveryWorkspace, { type DraftEditor } from "./components/DraftRecoveryWorkspace"

function AppInner({ draftEditors }: { draftEditors: Map<string, DraftEditor> }) {
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
      {import.meta.env.VITE_CAREEROS_DRAFT_RECOVERY_MODE === "1" ? page === "home" && <DraftRecoveryWorkspace editors={draftEditors} /> : <HomePage
        setPage={setPage}
        active={page === "home"}
        enrichmentActive={page === "results"}
      />}
    </Layout>
  )
}

export default function App() {
  // Retain only user-entered editor wording in this document across same-account reauthentication.
  const draftEditors = useRef(new Map<string, DraftEditor>()).current
  useEffect(()=>{
    const deleted=(event:Event)=>{
      const owner=(event as CustomEvent<{accountId?:string}>).detail?.accountId
      if(!owner)return
      draftEditors.delete(owner)
      const key=`profile-submission-journey:${owner}`
      submissionSession(key)?.discard();releaseSubmissionSession(key)
      forgetManagedIngestion(owner);forgetManagedProfileReview(owner);forgetManagedStandaloneCV(owner)
    }
    window.addEventListener("careeros-deletion-fence",deleted)
    return()=>window.removeEventListener("careeros-deletion-fence",deleted)
  },[draftEditors])
  const pilot = import.meta.env.VITE_CAREEROS_PILOT_MODE === "1"
  if (pilot || import.meta.env.VITE_CAREEROS_ACCOUNT_MODE === "1" || import.meta.env.VITE_CAREEROS_DRAFT_RECOVERY_MODE === "1")
    return (
      <>
        <AccountGate pilot={pilot}>
          <AppInner draftEditors={draftEditors} />
        </AccountGate>
        {pilot && <PilotSupport />}
      </>
    )
  return (
    <StoreProvider>
      <AppInner draftEditors={draftEditors} />
    </StoreProvider>
  )
}
