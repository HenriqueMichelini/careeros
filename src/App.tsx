import { useState } from 'react'
import { StoreProvider } from './lib/store'
import { Page } from './lib/types'
import Layout from './components/Layout'
import HomePage from './pages/HomePage'
import RepositoryPage from './pages/RepositoryPage'
import ResultsPage from './pages/ResultsPage'
import CvPage from './pages/CvPage'
import LandingPage from './pages/LandingPage'

function AppInner() {
  const [page, setPage] = useState<Page>('landing')

  return (
    <Layout page={page} setPage={setPage}>
      {page === 'landing' && <LandingPage setPage={setPage} />}
      {page === 'home' && <HomePage setPage={setPage} />}
      {page === 'repository' && <RepositoryPage />}
      {page === 'cv' && <CvPage />}
      {page === 'results' && <ResultsPage setPage={setPage} />}
    </Layout>
  )
}

export default function App() {
  return (
    <StoreProvider>
      <AppInner />
    </StoreProvider>
  )
}
