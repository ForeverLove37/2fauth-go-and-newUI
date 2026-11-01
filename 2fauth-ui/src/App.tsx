import { Suspense, lazy, useEffect } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import Shell from './components/Shell'
import { Login, Register } from './views/Auth'
import { useStore } from './store/useStore'

import Backdrop from './three/Backdrop'
import { useThemeStore } from './theme/store'

const Dashboard = lazy(() => import('./views/Dashboard'))
const Settings = lazy(() => import('./views/Settings'))

function Booting() {
  return (
    <div className="boot">
      <div className="spinner" />
      <span>Waking up the vault…</span>
    </div>
  )
}

function Protected({ children }: { children: React.ReactNode }) {
  const user = useStore((state) => state.user)
  const location = useLocation()
  if (!user) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  return <>{children}</>
}

export default function App() {
  const boot = useStore((state) => state.boot)
  const booting = useStore((state) => state.booting)
  const ready = useStore((state) => state.ready)
  const user = useStore((state) => state.user)
  const themeBase = useThemeStore((state) => state.theme.base)
  const accent = useThemeStore((state) => state.theme.palette.accent)
  const navigate = useNavigate()
  const toast = useStore((state) => state.toast)

  useEffect(() => {
    const onExpired = () => {
      if (!useStore.getState().user) return
      useStore.setState({ user: null, accounts: [], groups: [], prefs: {} })
      toast('warn', 'Your session expired, please sign in again')
      navigate('/login', { replace: true })
    }
    window.addEventListener('2fauth:session-expired', onExpired)
    return () => window.removeEventListener('2fauth:session-expired', onExpired)
  }, [navigate, toast])


  useEffect(() => {
    void boot()
  }, [boot])

  if (booting || !ready) return <Booting />

  return (
    <>
      <Backdrop light={themeBase === 'light'} accent={accent} />
      <Routes>
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <Login />} />
      <Route path="/register" element={user ? <Navigate to="/" replace /> : <Register />} />
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
