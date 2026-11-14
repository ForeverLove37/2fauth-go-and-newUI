import { Suspense, lazy, useEffect } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import Shell from './components/Shell'
import { useI18nStore, useT } from './i18n'
import { Login, PasswordLost, PasswordReset, Register } from './views/Auth'
import { clearLoaded } from './lib/hooks'
import { useStore } from './store/useStore'

import Backdrop from './three/Backdrop'
import { useThemeStore } from './theme/store'

const Dashboard = lazy(() => import('./views/Dashboard'))
const Settings = lazy(() => import('./views/Settings'))
const Agents = lazy(() => import('./views/Agents'))
const Landing = lazy(() => import('./views/Landing'))
const Legal = lazy(() => import('./views/Legal'))

function Booting() {
  const t = useT()
  return (
    <div className="boot">
      <div className="spinner" />
      <span>{t('Waking up the vault…')}</span>
    </div>
  )
}

function Protected({ children }: { children: React.ReactNode }) {
  const user = useStore((state) => state.user)
  const location = useLocation()
  if (!user) {
    // Visitors who are not signed in land on the presentation of the project.
    if (location.pathname === '/') {
      return (
        <Suspense fallback={<Booting />}>
          <Landing />
        </Suspense>
      )
    }
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return <>{children}</>
}

export default function App() {
  const t = useT()
  const boot = useStore((state) => state.boot)
  const booting = useStore((state) => state.booting)
  const ready = useStore((state) => state.ready)
  const user = useStore((state) => state.user)
  const themeBase = useThemeStore((state) => state.theme.base)
  const accent = useThemeStore((state) => state.theme.palette.accent)
  const background = useThemeStore((state) => state.theme.background)
  const motion = useThemeStore((state) => state.theme.motion)
  const navigate = useNavigate()
  const toast = useStore((state) => state.toast)

  useEffect(() => {
    const onExpired = () => {
      if (!useStore.getState().user) return
      useStore.setState({ user: null, accounts: [], groups: [], prefs: {} })
      toast('warn', t('Your session expired, please sign in again'))
      navigate('/login', { replace: true })
    }
    window.addEventListener('2fauth:session-expired', onExpired)
    return () => window.removeEventListener('2fauth:session-expired', onExpired)
  }, [navigate, toast])


  useEffect(() => {
    void boot()
  }, [boot])

  // what the settings screens loaded belongs to this user
  const userId = user?.id
  useEffect(() => clearLoaded(), [userId])

  // the language preference lives on the server; adopt it when nothing local was
  // chosen yet, and push a locally chosen language to the server after signing in
  const prefLang = useStore((state) => state.prefs.lang)
  const setPreference = useStore((state) => state.setPreference)
  const setLocale = useI18nStore((state) => state.setLocale)

  useEffect(() => {
    const local = useI18nStore.getState().locale
    const wanted = typeof prefLang === 'string' ? prefLang : ''
    if (!user || !wanted) return
    if (local === 'auto' || local === 'browser') {
      if (wanted !== 'browser') setLocale(wanted)
      return
    }
    if (wanted !== local) void setPreference('lang', local).catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user?.id, prefLang])

  if (booting || !ready) return <Booting />

  return (
    <>
      <Backdrop
        light={themeBase === 'light'}
        accent={accent}
        style={background?.style ?? 'grid'}
        intensity={background?.intensity ?? 1}
        speed={background?.speed ?? 1}
        fps={motion?.fps ?? 0}
        quality={motion?.quality ?? 'auto'}
      />
      <Routes>
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <Login />} />
      <Route path="/register" element={user ? <Navigate to="/" replace /> : <Register />} />
      <Route path="/password/lost" element={<PasswordLost />} />
      <Route path="/password/reset" element={<PasswordReset />} />
      {/* Public: an agent's user reads it before granting anything. */}
      <Route
        path="/agents"
        element={
          <Suspense fallback={<Booting />}>
            <Agents />
          </Suspense>
        }
      />
      <Route
        path="/terms"
        element={
          <Suspense fallback={<Booting />}>
            <Legal kind="terms" />
          </Suspense>
        }
      />
      <Route
        path="/privacy"
        element={
          <Suspense fallback={<Booting />}>
            <Legal kind="privacy" />
          </Suspense>
        }
      />
      <Route
        element={
          <Protected>
            <Shell />
          </Protected>
        }
      >
        <Route
          path="/"
          element={
            <Suspense fallback={<Booting />}>
              <Dashboard />
            </Suspense>
          }
        />
        <Route
          path="/settings"
          element={
            <Suspense fallback={<Booting />}>
              <Settings />
            </Suspense>
          }
        />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  )
}
