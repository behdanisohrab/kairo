import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { useState, useEffect, createContext, useContext, Suspense, lazy, type ReactNode } from 'react'
import { api } from './api'
import type { UserData } from './api'
import { ToastProvider } from './components/ui/Toast'
import { ThemeProvider } from './lib/theme'
import { I18nProvider, useI18n } from './lib/i18n'
import Layout from './components/Layout'
import { FiAlertCircle } from 'react-icons/fi'

// Lazy pages — code splitting for production
const Login = lazy(() => import('./pages/Login'))
const AdminOverview = lazy(() => import('./pages/admin/Overview'))
const Users = lazy(() => import('./pages/admin/Users'))
const UserDevices = lazy(() => import('./pages/admin/UserDevices'))
const AllDevices = lazy(() => import('./pages/admin/AllDevices'))
const Domains = lazy(() => import('./pages/admin/Domains'))
const UserDashboard = lazy(() => import('./pages/user/Dashboard'))
const Guide = lazy(() => import('./pages/Guide'))

// ── Auth context ────────────────────────────────────────────────────
interface AuthCtx {
  user: UserData | null
  setUser: (u: UserData | null) => void
  logout: () => Promise<void>
  refresh: () => Promise<void>
  initialized: boolean
}

export const AuthContext = createContext<AuthCtx>({
  user: null,
  setUser: () => {},
  logout: async () => {},
  refresh: async () => {},
  initialized: false,
})
export const useAuth = () => useContext(AuthContext)

// ── Guards ──────────────────────────────────────────────────────────
function Guard({ children, admin = false }: { children: ReactNode; admin?: boolean }) {
  const { user, initialized } = useAuth()
  const loc = useLocation()
  if (!initialized) return <PageLoader />
  if (!user) return <Navigate to="/login" state={{ from: loc }} replace />
  if (admin && user.role !== 'admin') return <Navigate to="/dashboard" replace />
  return <>{children}</>
}

function PublicOnly({ children }: { children: ReactNode }) {
  const { user, initialized } = useAuth()
  if (!initialized) return <PageLoader />
  if (user) return <Navigate to={user.role === 'admin' ? '/admin' : '/dashboard'} replace />
  return <>{children}</>
}

function PageLoader() {
  const { t } = useI18n()
  return (
    <div className="min-h-[60vh] flex flex-col items-center justify-center gap-3">
      <div
        className="h-8 w-8 animate-spin rounded-full border-2"
        style={{ borderColor: 'var(--color-border)', borderTopColor: 'var(--color-brand)' }}
      />
      <p className="mono text-xs tracking-wide text-[var(--color-ink-4)]">kairo {t('common.loading')}</p>
    </div>
  )
}

function RouteFallback() {
  return (
    <div className="min-h-[60vh] flex items-center justify-center">
      <div className="skeleton h-32 w-full max-w-xl rounded-[18px]" />
    </div>
  )
}

function NotFound() {
  const { user } = useAuth()
  const home = user?.role === 'admin' ? '/admin' : user ? '/dashboard' : '/login'
  return (
    <div className="min-h-[60vh] flex flex-col items-center justify-center text-center px-6">
      <div className="inline-flex h-12 w-12 items-center justify-center rounded-2xl border bg-[var(--color-surface)] text-[var(--color-ink-3)]">
        <FiAlertCircle size={20} />
      </div>
      <h1 className="mt-4 text-xl font-semibold tracking-tight">Page not found</h1>
      <p className="mt-1 text-sm text-[var(--color-ink-3)]">The requested page does not exist</p>
      <a href={home} className="btn btn-primary mt-5">
        Go home
      </a>
    </div>
  )
}

function ErrorBoundary({ children }: { children: ReactNode }) {
  return <Suspense fallback={<RouteFallback />}>{children}</Suspense>
}

// ── Inner app (needs i18n) ──────────────────────────────────────────
function AppRoutes() {
  const [user, setUser] = useState<UserData | null>(null)
  const [initialized, setInitialized] = useState(false)

  const refresh = async () => {
    const r = await api.me()
    if (r.ok && r.user) setUser(r.user)
    else setUser(null)
  }

  useEffect(() => {
    refresh().finally(() => setInitialized(true))
  }, [])

  const logout = async () => {
    await api.logout()
    setUser(null)
  }

  if (!initialized) {
    return (
      <div className="min-h-screen grid place-items-center bg-[var(--color-bg)]">
        <div className="flex flex-col items-center gap-3">
          <div className="h-10 w-10 grid place-items-center rounded-2xl bg-[var(--color-ink)] text-[var(--color-bg)] font-mono text-sm font-semibold">
            k
          </div>
          <div
            className="h-6 w-6 animate-spin rounded-full border-2"
            style={{ borderColor: 'var(--color-border)', borderTopColor: 'var(--color-ink)' }}
          />
        </div>
      </div>
    )
  }

  return (
    <AuthContext.Provider value={{ user, setUser, logout, refresh, initialized }}>
      <ToastProvider>
        <BrowserRouter>
          <ErrorBoundary>
            <Routes>
              <Route path="/login" element={<PublicOnly><Login /></PublicOnly>} />
              <Route path="/admin" element={<Guard admin><Layout><AdminOverview /></Layout></Guard>} />
              <Route path="/admin/users" element={<Guard admin><Layout><Users /></Layout></Guard>} />
              <Route path="/admin/users/:id/devices" element={<Guard admin><Layout><UserDevices /></Layout></Guard>} />
              <Route path="/admin/devices" element={<Guard admin><Layout><AllDevices /></Layout></Guard>} />
              <Route path="/admin/domains" element={<Guard admin><Layout><Domains /></Layout></Guard>} />
              <Route path="/dashboard" element={<Guard><Layout><UserDashboard /></Layout></Guard>} />
              <Route path="/guide" element={<Guard><Layout><Guide /></Layout></Guard>} />
              <Route
                path="/"
                element={
                  !user ? <Navigate to="/login" replace /> : user.role === 'admin' ? <Navigate to="/admin" replace /> : <Navigate to="/dashboard" replace />
                }
              />
              <Route path="*" element={<Guard><Layout><NotFound /></Layout></Guard>} />
            </Routes>
          </ErrorBoundary>
        </BrowserRouter>
      </ToastProvider>
    </AuthContext.Provider>
  )
}

// ── Root with providers ─────────────────────────────────────────────
export default function App() {
  return (
    <ThemeProvider>
      <I18nProvider>
        <AppRoutes />
      </I18nProvider>
    </ThemeProvider>
  )
}
