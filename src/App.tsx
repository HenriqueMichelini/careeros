import { useState } from "react"
import { StoreProvider } from "./lib/store"
import { Page } from "./lib/types"
import Layout from "./components/Layout"
import HomePage from "./pages/HomePage"
import RepositoryPage from "./pages/RepositoryPage"
import ResultsPage from "./pages/ResultsPage"
import CvPage from "./pages/CvPage"
import LandingPage from "./pages/LandingPage"
import AccountGate from "./components/AccountGate"

function AppInner() {
  const [page, setPage] = useState<Page>("landing")

  return (
    <Layout page={page} setPage={setPage}>
      {page === "landing" && <LandingPage setPage={setPage} />}
      {page === "repository" && <RepositoryPage />}
      {page === "cv" && <CvPage />}
      {page === "results" && <ResultsPage setPage={setPage} />}
      <HomePage
        setPage={setPage}
        active={page === "home"}
        enrichmentActive={page === "results"}
      />
    </Layout>
  )
}

export default function App() {
  if (import.meta.env.VITE_CAREEROS_ACCOUNT_MODE === "1")
    return (
      <AccountGate>
        <AppInner />
      </AccountGate>
    )
  return (
    <StoreProvider>
      <AppInner />
    </StoreProvider>
  )
}
