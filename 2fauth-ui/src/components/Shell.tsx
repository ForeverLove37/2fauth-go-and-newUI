import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useStore } from '../store/useStore'
import Icon from './Icon'

function initials(name: string): string {
  return name
    .split(/\s+/)
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()
}

/** Application frame: top bar, routed content and toasts. */
export default function Shell() {
  const user = useStore((state) => state.user)
  const prefs = useStore((state) => state.prefs)
  const config = useStore((state) => state.config)
  const logout = useStore((state) => state.logout)
  const setPreference = useStore((state) => state.setPreference)
  const toasts = useStore((state) => state.toasts)
  const dismiss = useStore((state) => state.dismiss)
  const navigate = useNavigate()

  const theme = String(prefs.theme ?? 'system')

  async function signOut() {
    await logout()
    navigate('/login', { replace: true })
  }

  async function cycleTheme() {
    const order = ['system', 'dark', 'light']
    const next = order[(order.indexOf(theme) + 1) % order.length]
    await setPreference('theme', next)
  }

  return (
    <div className="app-shell">
      <div className="layer">
        <header className="topbar">
          <div className="brand">
            <div className="brand-mark">2F</div>
            <div>
              {config?.appName ?? '2FAuth'}
              <small>3D control panel</small>
            </div>
          </div>

          <nav className="nav">
            <NavLink to="/" end>
              <Icon name="orbit" size={15} /> <span>Panel</span>
            </NavLink>
            <NavLink to="/settings">
              <Icon name="sliders" size={15} /> <span>Settings</span>
            </NavLink>
            {user?.is_admin && (
              <NavLink to="/settings?tab=admin">
                <Icon name="shield" size={15} /> <span>Admin</span>
              </NavLink>
            )}
          </nav>

          <div className="spacer" />

          <div className="flex">
            <button className="btn icon ghost" onClick={cycleTheme} title={`Theme: ${theme}`}>
              <Icon name="eye" size={15} />
            </button>
            <button className="avatar" title={user ? `${user.name} · ${user.email}` : ''} onClick={() => navigate('/settings')}>
              {initials(user?.name ?? '?')}
            </button>
            <button className="btn icon ghost" onClick={signOut} title="Sign out">
              <Icon name="logout" size={15} />
            </button>
          </div>
        </header>

        <Outlet />
      </div>

      <div className="toasts">
        {toasts.map((toast) => (
          <div key={toast.id} className={`toast ${toast.kind}`} onClick={() => dismiss(toast.id)}>
            <Icon name={toast.kind === 'error' ? 'close' : 'check'} size={15} />
            <span>{toast.text}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
