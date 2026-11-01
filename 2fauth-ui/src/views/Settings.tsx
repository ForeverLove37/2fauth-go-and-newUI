import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import type { ApiError, ManagedUser, PersonalAccessToken, WebAuthnCredential } from '../api/types'
import Icon from '../components/Icon'
import ThemeEditor from './ThemeEditor'
import SsoSettingsPanel from '../components/SsoSettingsPanel'
import { useStore } from '../store/useStore'

type Tab = 'theme' | 'options' | 'account' | 'security' | 'sso' | 'admin'

const languages = ['browser', 'en', 'fr', 'de', 'es-ES', 'it', 'nl', 'pt-BR', 'ru', 'zh-CN', 'zh-TW', 'ja', 'ko', 'tr', 'bg', 'da', 'hi']

/** User preferences, account settings, security and administration. */
export default function Settings() {
  const user = useStore((state) => state.user)
  const prefs = useStore((state) => state.prefs)
  const locked = useStore((state) => state.locked)
  const setPreference = useStore((state) => state.setPreference)
  const [params, setParams] = useSearchParams()
  const [tab, setTab] = useState<Tab>((params.get('tab') as Tab) ?? 'theme')

  return (
    <div className="stage" style={{ overflow: 'auto' }}>
      <div style={{ padding: '18px 26px 40px', maxWidth: 1020, margin: '0 auto', width: '100%' }}>
        <div className="tabs" style={{ marginBottom: 20 }}>
          {(['theme', 'options', 'account', 'security', ...(user?.is_admin ? (['sso', 'admin'] as Tab[]) : [])] as Tab[]).map((item) => (
            <button
              key={item}
              className={tab === item ? 'active' : ''}
              onClick={() => {
                setTab(item)
                setParams(item === 'theme' ? {} : { tab: item })
              }}
            >
              {item === 'theme'
                ? '外观'
                : item === 'options'
                  ? '选项'
                  : item === 'account'
                    ? '账号'
                    : item === 'security'
                      ? '安全'
                      : item === 'sso'
                        ? '单点登录'
                        : '管理'}
            </button>
          ))}
        </div>

        {tab === 'theme' && <ThemeEditor />}
        {tab === 'options' && <OptionsTab prefs={prefs} locked={locked} setPreference={setPreference} />}
        {tab === 'account' && <AccountTab />}
        {tab === 'security' && <SecurityTab />}
        {tab === 'sso' && <SsoSettingsPanel />}
        {tab === 'admin' && <AdminTab />}
      </div>
    </div>
  )
}

function Toggle({
  label,
  hint,
  value,
  disabled,
  onChange,
}: {
  label: string
  hint?: string
  value: boolean
  disabled?: boolean
  onChange: (value: boolean) => void
}) {
  return (
    <div className="switch">
      <div className="switch-label">
        {label}
        {hint && <small>{hint}</small>}
      </div>
      <button
        className={`toggle${value ? ' on' : ''}`}
        disabled={disabled}
        onClick={() => onChange(!value)}
        aria-pressed={value}
      />
    </div>
  )
}

function OptionsTab({
  prefs,
  locked,
  setPreference,
}: {
  prefs: Record<string, unknown>
  locked: string[]
  setPreference: (key: string, value: unknown) => Promise<void>
}) {
  const isLocked = (key: string) => locked.includes(key)
  return (
    <div className="panel">
      <div className="panel-head">
        <div>
          <h2>行为与偏好</h2>
          <p>偏好按用户保存在服务端；外观请在「外观」页调整。</p>
        </div>
      </div>
      <div className="panel-body">
        <div className="row">
          <div className="field">
            <label>语言 / Language</label>
            <select
              className="select"
              value={String(prefs.lang ?? 'browser')}
              disabled={isLocked('lang')}
              onChange={(event) => void setPreference('lang', event.target.value)}
            >
              {languages.map((language) => (
                <option key={language} value={language}>
                  {language}
                </option>
              ))}
            </select>
          </div>
          <div className="field">
            <label>Sort order</label>
            <select
              className="select"
              value={String(prefs.sortOrder ?? 'asc')}
              disabled={isLocked('sortOrder')}
              onChange={(event) => void setPreference('sortOrder', event.target.value)}
            >
              <option value="asc">A → Z</option>
              <option value="desc">Z → A</option>
            </select>
          </div>
        </div>

        <Toggle
          label="Hide codes as dots"
          hint="Codes are masked until you reveal or copy them"
          value={Boolean(prefs.showOtpAsDot)}
          disabled={isLocked('showOtpAsDot')}
          onChange={(value) => void setPreference('showOtpAsDot', value)}
        />
        <Toggle
          label="Show the next code"
          hint="Preview the upcoming TOTP value"
          value={Boolean(prefs.showNextOtp)}
          disabled={isLocked('showNextOtp')}
          onChange={(value) => void setPreference('showNextOtp', value)}
        />
        <Toggle
          label="Copy code on display"
          hint="Put the code in the clipboard as soon as it is shown"
          value={Boolean(prefs.copyOtpOnDisplay)}
          disabled={isLocked('copyOtpOnDisplay')}
          onChange={(value) => void setPreference('copyOtpOnDisplay', value)}
        />
        <Toggle
          label="Show account icons"
          value={Boolean(prefs.showAccountsIcons)}
          disabled={isLocked('showAccountsIcons')}
          onChange={(value) => void setPreference('showAccountsIcons', value)}
        />
        <Toggle
          label="Group chips"
          hint="Display the group filter bar above the orbit"
          value={Boolean(prefs.useGroupChips)}
          disabled={isLocked('useGroupChips')}
          onChange={(value) => void setPreference('useGroupChips', value)}
        />
        <Toggle
          label="Remember the active group"
          value={Boolean(prefs.rememberActiveGroup)}
          disabled={isLocked('rememberActiveGroup')}
          onChange={(value) => void setPreference('rememberActiveGroup', value)}
        />
      </div>
    </div>
  )
}

function AccountTab() {
  const user = useStore((state) => state.user)
  const toast = useStore((state) => state.toast)
  const refreshSession = useStore((state) => state.refreshSession)
  const [name, setName] = useState(user?.name ?? '')
  const [email, setEmail] = useState(user?.email ?? '')
  const [password, setPassword] = useState('')
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')

  useEffect(() => {
    setName(user?.name ?? '')
    setEmail(user?.email ?? '')
  }, [user?.name, user?.email])

  async function saveProfile() {
    try {
      await api.updateProfile({ name, email, password })
      toast('success', 'Profile updated')
      setPassword('')
      await refreshSession()
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  async function savePassword() {
    try {
      await api.updatePassword(currentPassword, newPassword)
      toast('success', 'Password changed')
      setCurrentPassword('')
      setNewPassword('')
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  return (
    <div className="stack">
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>Profile</h2>
            <p>Your current password is required to save changes.</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="row">
            <div className="field">
              <label>Name</label>
              <input className="input" value={name} onChange={(event) => setName(event.target.value)} />
            </div>
            <div className="field">
              <label>Email</label>
              <input className="input" type="email" value={email} onChange={(event) => setEmail(event.target.value)} />
            </div>
          </div>
          <div className="field">
            <label>Current password</label>
            <input className="input" type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
          </div>
          <button className="btn primary" onClick={saveProfile}>
            Save profile
          </button>
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>Password</h2>
            <p>Minimum 8 characters, confirmed automatically.</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="row">
            <div className="field">
              <label>Current password</label>
              <input
                className="input"
                type="password"
                value={currentPassword}
                onChange={(event) => setCurrentPassword(event.target.value)}
              />
            </div>
            <div className="field">
              <label>New password</label>
              <input
                className="input"
                type="password"
                value={newPassword}
                onChange={(event) => setNewPassword(event.target.value)}
              />
            </div>
          </div>
          <button className="btn" onClick={savePassword} disabled={!currentPassword || newPassword.length < 8}>
            Change password
          </button>
        </div>
      </div>
    </div>
  )
}

function SecurityTab() {
  const toast = useStore((state) => state.toast)
  const [tokens, setTokens] = useState<PersonalAccessToken[]>([])
  const [credentials, setCredentials] = useState<WebAuthnCredential[]>([])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')

  useEffect(() => {
    void api.tokens().then(setTokens).catch(() => setTokens([]))
    void api.credentials().then(setCredentials).catch(() => setCredentials([]))
  }, [])

  async function create() {
    if (!name.trim()) return
    try {
      const response = await api.createToken(name.trim())
      setFresh(response.accessToken)
      setName('')
      setTokens(await api.tokens())
      toast('success', 'Token created')
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  async function removeToken(id: string) {
    await api.deleteToken(id)
    setTokens(await api.tokens())
  }

  async function removeCredential(id: string) {
    if (!window.confirm('Remove this passkey?')) return
    await api.deleteCredential(id)
    setCredentials(await api.credentials())
  }

  return (
    <div className="stack">
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>Personal access tokens</h2>
            <p>Use them as `Authorization: Bearer &lt;token&gt;` for scripts and the CLI.</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="flex" style={{ marginBottom: 14 }}>
            <input
              className="input grow"
              placeholder="Token name"
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
            <button className="btn primary" onClick={create} disabled={!name.trim()}>
              <Icon name="plus" size={14} /> Create
            </button>
          </div>
          {fresh && (
            <div className="panel" style={{ padding: 12, marginBottom: 14, borderColor: 'rgba(85,230,193,.5)' }}>
              <div className="hint" style={{ marginBottom: 6 }}>
                Copy this token now, it will not be shown again:
              </div>
              <code className="mono" style={{ wordBreak: 'break-all' }}>{fresh}</code>
            </div>
          )}
          {tokens.length === 0 ? (
            <p className="hint">No token yet.</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Created</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {tokens.map((token) => (
                  <tr key={token.id}>
                    <td>{token.name}</td>
                    <td className="mono small">{token.created_at ?? '—'}</td>
                    <td style={{ textAlign: 'right' }}>
                      <button className="btn small danger" onClick={() => void removeToken(token.id)}>
                        <Icon name="trash" size={13} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>Passkeys</h2>
            <p>Registered WebAuthn credentials. Registration requires the PHP-compatible WebAuthn endpoints.</p>
          </div>
        </div>
        <div className="panel-body">
          {credentials.length === 0 ? (
            <p className="hint">No passkey registered.</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Created</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {credentials.map((credential) => (
                  <tr key={credential.id}>
                    <td>{credential.name}</td>
                    <td className="mono small">{credential.created_at}</td>
                    <td style={{ textAlign: 'right' }}>
                      <button className="btn small danger" onClick={() => void removeCredential(credential.id)}>
                        <Icon name="trash" size={13} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

function AdminTab() {
  const toast = useStore((state) => state.toast)
  const [users, setUsers] = useState<ManagedUser[]>([])
  const [settings, setSettings] = useState<Record<string, unknown>>({})
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isAdmin, setIsAdmin] = useState(false)

  const reload = async () => {
    try {
      setUsers(await api.users())
    } catch {
      setUsers([])
    }
    try {
      setSettings(await api.settings())
    } catch {
      setSettings({})
    }
  }

  useEffect(() => {
    void reload()
  }, [])

  async function createUser() {
    try {
      await api.createUser({ name, email, password, is_admin: isAdmin })
      toast('success', 'User created')
      setName('')
      setEmail('')
      setPassword('')
      setIsAdmin(false)
      await reload()
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  async function toggleSetting(key: string, value: unknown) {
    try {
      await api.setSetting(key, value)
      setSettings((current) => ({ ...current, [key]: value }))
      toast('success', `${key} updated`)
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  return (
    <div className="stack">
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>Application settings</h2>
            <p>Instance wide switches.</p>
          </div>
        </div>
        <div className="panel-body">
          <Toggle
            label="New registrations"
            hint="Allow visitors to create an account"
            value={!Boolean(settings.disableRegistration)}
            onChange={(value) => void toggleSetting('disableRegistration', !value)}
          />
          <Toggle
            label="Account sharing"
            hint="Let users share their accounts with others"
            value={Boolean(settings.enableSharing)}
            onChange={(value) => void toggleSetting('enableSharing', value)}
          />
          <Toggle
            label="Share with everyone"
            hint="Enable the all-users sharing scope"
            value={Boolean(settings.enableAllUsersSharingScope)}
            onChange={(value) => void toggleSetting('enableAllUsersSharingScope', value)}
          />
          <Toggle
            label="Check for updates"
            value={Boolean(settings.checkForUpdate)}
            onChange={(value) => void toggleSetting('checkForUpdate', value)}
          />
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>Users</h2>
            <p>{users.length} accounts on this instance.</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="row" style={{ marginBottom: 14 }}>
            <div className="field">
              <label>Name</label>
              <input className="input" value={name} onChange={(event) => setName(event.target.value)} />
            </div>
            <div className="field">
              <label>Email</label>
              <input className="input" type="email" value={email} onChange={(event) => setEmail(event.target.value)} />
            </div>
            <div className="field">
              <label>Password</label>
              <input className="input" type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
            </div>
          </div>
          <div className="flex" style={{ marginBottom: 16 }}>
            <Toggle label="Administrator" value={isAdmin} onChange={setIsAdmin} />
            <button className="btn primary" onClick={createUser} disabled={!name || !email || password.length < 8}>
              <Icon name="plus" size={14} /> Create user
            </button>
          </div>

          <table className="table">
            <thead>
              <tr>
                <th>User</th>
                <th>Accounts</th>
                <th>Role</th>
                <th>Last seen</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {users.map((managed) => (
                <tr key={managed.id}>
                  <td>
                    <strong>{managed.name}</strong>
                    <div className="hint small">{managed.email}</div>
                  </td>
                  <td className="mono">{managed.twofaccounts_count}</td>
                  <td>{managed.is_admin ? 'admin' : 'user'}</td>
                  <td className="small muted">{managed.last_seen_at || '—'}</td>
                  <td>
                    <div className="flex" style={{ justifyContent: 'flex-end' }}>
                      <button
                        className="btn small ghost"
                        onClick={async () => {
                          await api.promoteUser(managed.id, !managed.is_admin)
                          await reload()
                        }}
                      >
                        {managed.is_admin ? 'Demote' : 'Promote'}
                      </button>
                      <button
                        className="btn small ghost"
                        onClick={async () => {
                          const result = await api.resetUserPassword(managed.id)
                          toast('success', `New password: ${result.password}`)
                        }}
                      >
                        Reset
                      </button>
                      <button
                        className="btn small danger"
                        onClick={async () => {
                          if (!window.confirm(`Delete ${managed.name}?`)) return
                          try {
                            await api.deleteUser(managed.id)
                            await reload()
                          } catch (caught) {
                            toast('error', (caught as ApiError).message)
                          }
                        }}
                      >
                        <Icon name="trash" size={13} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
