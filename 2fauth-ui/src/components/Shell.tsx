import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useT } from '../i18n'
import { isAdminTab } from '../lib/settingsTabs'
import { useStore } from '../store/useStore'
import Icon from './Icon'
import Logo from './Logo'

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
  const t = useT()
  const user = useStore((state) => state.user)
  const config = useStore((state) => state.config)
  const logout = useStore((state) => state.logout)
  const toasts = useStore((state) => state.toasts)
  const dismiss = useStore((state) => state.dismiss)
  const navigate = useNavigate()
  const location = useLocation()
  const onSettings = location.pathname === '/settings'
  const onAdmin = onSettings && isAdminTab(new URLSearchParams(location.search).get('tab'))

  async function signOut() {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className="app-shell">
      <div className="layer">
        <header className="topbar">
          <div className="brand">
            <Logo />
            <div>
              {config?.appName ?? '2FAuth'}
              <small>{t('3D control panel')}</small>
            </div>
          </div>

          <nav className="nav">
            <NavLink to="/" end>
              <Icon name="orbit" size={15} /> <span>{t('Panel')}</span>
            </NavLink>
            <NavLink to="/settings" className={() => (onSettings && !onAdmin ? 'active' : '')}>
              <Icon name="sliders" size={15} /> <span>{t('Settings')}</span>
            </NavLink>
            {user?.is_admin && (
              <NavLink to="/settings?tab=admin" className={() => (onAdmin ? 'active' : '')}>
                <Icon name="shield" size={15} /> <span>{t('Admin')}</span>
              </NavLink>
            )}
          </nav>

          <div className="spacer" />

          <div className="flex">
            <button className="avatar" title={user ? `${user.name} · ${user.email}` : ''} onClick={() => navigate('/settings?tab=account')}>
              {initials(user?.name ?? '?')}
            </button>
            <button className="btn icon ghost" onClick={signOut} title={t('Sign out')}>
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
