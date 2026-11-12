import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/sky.css'
import './styles/app.css'
import './styles/settings.css'
import { AppProvider, useApp } from './lib/app-context'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import Login from './pages/Login'
import Tonight from './pages/Tonight'
import SkyMap from './pages/SkyMap'
import Events from './pages/Events'
import LogPage from './pages/Log'
import Lists from './pages/Lists'
import PlansPage from './pages/Plans'
import PlacesPage from './pages/Places'
import Settings from './pages/Settings'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />
  return <>{children}</>
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login mode="login" />} />
      <Route path="/setup" element={<Login mode="setup" />} />
      <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><Settings /></ErrorBoundary></Gate>} />
      <Route element={<Gate><ErrorBoundary where="sky"><Layout /></ErrorBoundary></Gate>}>
        <Route path="/" element={<Tonight />} />
        <Route path="/map" element={<SkyMap />} />
        <Route path="/events" element={<Events />} />
        <Route path="/log" element={<LogPage />} />
        <Route path="/log/:id" element={<LogPage />} />
        <Route path="/lists" element={<Lists />} />
        <Route path="/plans" element={<PlansPage />} />
        <Route path="/plans/:id" element={<PlansPage />} />
        <Route path="/places" element={<PlacesPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

// Works without signal at a dark site (public/sw.js).
if ('serviceWorker' in navigator && window.isSecureContext && location.port !== '5173') {
  window.addEventListener('load', () => { navigator.serviceWorker.register('/sw.js').catch(() => {}) })
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><AppProvider><App /></AppProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
