import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import type { ApiError, ManagedUser, PersonalAccessToken, WebAuthnCredential } from '../api/types'
import Icon from '../components/Icon'
import ThemeEditor from './ThemeEditor'
import IconProvidersPanel from '../components/IconProvidersPanel'
import SsoSettingsPanel from '../components/SsoSettingsPanel'
import { useI18nStore, useT } from '../i18n'
import { useStore } from '../store/useStore'

type Tab = 'theme' | 'options' | 'account' | 'security' | 'sso' | 'admin'

/** Locales that actually ship a catalogue; anything else falls back to English. */
const languages = ['browser', 'en', 'zh-CN', 'zh-TW']

/** User preferences, account settings, security and administration. */
export default function Settings() {
  const t = useT()
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
                ? t('Appearance')
                : item === 'options'
                  ? t('Options')
                  : item === 'account'
                    ? t('Account')
                    : item === 'security'
                      ? t('Security')
                      : item === 'sso'
                        ? t('Single sign-on')
                        : t('Admin')}
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
  const t = useT()
  const setLocale = useI18nStore((state) => state.setLocale)
  const isLocked = (key: string) => locked.includes(key)
  return (
    <div className="panel">
      <div className="panel-head">
        <div>
          <h2>{t('Behavior and preferences')}</h2>
          <p>{t('Preferences are stored per user on the server; change the look on the Appearance tab.')}</p>
        </div>
      </div>
      <div className="panel-body">
        <div className="row">
          <div className="field">
            <label>{t('Language')}</label>
            <select
              className="select"
              value={String(prefs.lang ?? 'browser')}
              disabled={isLocked('lang')}
              onChange={(event) => {
                setLocale(event.target.value)
                void setPreference('lang', event.target.value)
              }}
            >
              {languages.map((language) => (
                <option key={language} value={language}>
                  {language}
                </option>
              ))}
            </select>
          </div>
          <div className="field">
            <label>{t('Sort order')}</label>
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
          label={t('Hide codes as dots')}
          hint={t('Codes are masked until you reveal or copy them')}
          value={Boolean(prefs.showOtpAsDot)}
          disabled={isLocked('showOtpAsDot')}
          onChange={(value) => void setPreference('showOtpAsDot', value)}
        />
        <Toggle
          label={t('Show the next code')}
          hint={t('Preview the upcoming TOTP value')}
          value={Boolean(prefs.showNextOtp)}
          disabled={isLocked('showNextOtp')}
          onChange={(value) => void setPreference('showNextOtp', value)}
        />
        <Toggle
          label={t('Copy code on display')}
          hint={t('Put the code in the clipboard as soon as it is shown')}
          value={Boolean(prefs.copyOtpOnDisplay)}
          disabled={isLocked('copyOtpOnDisplay')}
          onChange={(value) => void setPreference('copyOtpOnDisplay', value)}
        />
        <Toggle
          label={t('Show account icons')}
          value={Boolean(prefs.showAccountsIcons)}
          disabled={isLocked('showAccountsIcons')}
          onChange={(value) => void setPreference('showAccountsIcons', value)}
        />
        <Toggle
          label={t('Group chips')}
          hint={t('Display the group filter bar above the orbit')}
          value={Boolean(prefs.useGroupChips)}
          disabled={isLocked('useGroupChips')}
          onChange={(value) => void setPreference('useGroupChips', value)}
        />
        <Toggle
          label={t('Remember the active group')}
          value={Boolean(prefs.rememberActiveGroup)}
          disabled={isLocked('rememberActiveGroup')}
          onChange={(value) => void setPreference('rememberActiveGroup', value)}
        />
      </div>
    </div>
  )
}

function AccountTab() {
  const t = useT()
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
      toast('success', t('Profile updated'))
      setPassword('')
      await refreshSession()
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  async function savePassword() {
    try {
      await api.updatePassword(currentPassword, newPassword)
      toast('success', t('Password changed'))
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
            <h2>{t('Profile')}</h2>
            <p>{t('Your current password is required to save changes.')}</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="row">
            <div className="field">
              <label>{t('Name')}</label>
              <input className="input" value={name} onChange={(event) => setName(event.target.value)} />
            </div>
            <div className="field">
              <label>{t('Email')}</label>
              <input className="input" type="email" value={email} onChange={(event) => setEmail(event.target.value)} />
            </div>
          </div>
          <div className="field">
            <label>{t('Current password')}</label>
            <input className="input" type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
          </div>
          <button className="btn primary" onClick={saveProfile}>
            {t('Save profile')}
          </button>
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('Password')}</h2>
            <p>{t('Minimum 8 characters, confirmed automatically.')}</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="row">
            <div className="field">
              <label>{t('Current password')}</label>
              <input
                className="input"
                type="password"
                value={currentPassword}
                onChange={(event) => setCurrentPassword(event.target.value)}
              />
            </div>
            <div className="field">
              <label>{t('New password')}</label>
              <input
                className="input"
                type="password"
                value={newPassword}
                onChange={(event) => setNewPassword(event.target.value)}
              />
            </div>
          </div>
          <button className="btn" onClick={savePassword} disabled={!currentPassword || newPassword.length < 8}>
            {t('Change password')}
          </button>
        </div>
      </div>
    </div>
  )
}

function SecurityTab() {
  const t = useT()
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
      toast('success', t('Token created'))
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  async function removeToken(id: string) {
    await api.deleteToken(id)
    setTokens(await api.tokens())
  }

  async function removeCredential(id: string) {
    if (!window.confirm(t('Remove this passkey?'))) return
    await api.deleteCredential(id)
    setCredentials(await api.credentials())
  }

  return (
    <div className="stack">
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('Personal access tokens')}</h2>
            <p>{t('Use them as `Authorization: Bearer <token>` for scripts and the CLI.')}</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="flex" style={{ marginBottom: 14 }}>
            <input
              className="input grow"
              placeholder={t('Token name')}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
            <button className="btn primary" onClick={create} disabled={!name.trim()}>
              <Icon name="plus" size={14} /> {t('Create')}
            </button>
          </div>
          {fresh && (
            <div className="panel" style={{ padding: 12, marginBottom: 14, borderColor: 'rgba(85,230,193,.5)' }}>
              <div className="hint" style={{ marginBottom: 6 }}>
                {t('Copy this token now, it will not be shown again:')}
              </div>
              <code className="mono" style={{ wordBreak: 'break-all' }}>{fresh}</code>
            </div>
          )}
          {tokens.length === 0 ? (
            <p className="hint">{t('No token yet.')}</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>{t('Name')}</th>
                  <th>{t('Created')}</th>
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
            <h2>{t('Passkeys')}</h2>
            <p>{t('Registered WebAuthn credentials. Registration requires the PHP-compatible WebAuthn endpoints.')}</p>
          </div>
        </div>
        <div className="panel-body">
          {credentials.length === 0 ? (
            <p className="hint">{t('No passkey registered.')}</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>{t('Name')}</th>
                  <th>{t('Created')}</th>
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
  const t = useT()
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
      toast('success', t('User created'))
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
      toast('success', t('{key} updated', { key }))
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  return (
    <div className="stack">
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('Application settings')}</h2>
            <p>{t('Instance wide switches.')}</p>
          </div>
        </div>
        <div className="panel-body">
          <Toggle
            label={t('New registrations')}
            hint={t('Allow visitors to create an account')}
            value={!Boolean(settings.disableRegistration)}
            onChange={(value) => void toggleSetting('disableRegistration', !value)}
          />
          <Toggle
            label={t('Account sharing')}
            hint={t('Let users share their accounts with others')}
            value={Boolean(settings.enableSharing)}
            onChange={(value) => void toggleSetting('enableSharing', value)}
          />
          <Toggle
            label={t('Share with everyone')}
            hint={t('Enable the all-users sharing scope')}
            value={Boolean(settings.enableAllUsersSharingScope)}
            onChange={(value) => void toggleSetting('enableAllUsersSharingScope', value)}
          />
          <Toggle
            label={t('Check for updates')}
            value={Boolean(settings.checkForUpdate)}
            onChange={(value) => void toggleSetting('checkForUpdate', value)}
          />
        </div>
      </div>

      <IconProvidersPanel />

      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('Users')}</h2>
            <p>{t('{count} accounts on this instance.', { count: users.length })}</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="row" style={{ marginBottom: 14 }}>
            <div className="field">
              <label>{t('Name')}</label>
              <input className="input" value={name} onChange={(event) => setName(event.target.value)} />
            </div>
            <div className="field">
              <label>{t('Email')}</label>
              <input className="input" type="email" value={email} onChange={(event) => setEmail(event.target.value)} />
            </div>
            <div className="field">
              <label>{t('Password')}</label>
              <input className="input" type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
            </div>
          </div>
          <div className="flex" style={{ marginBottom: 16 }}>
            <Toggle label={t('Administrator')} value={isAdmin} onChange={setIsAdmin} />
            <button className="btn primary" onClick={createUser} disabled={!name || !email || password.length < 8}>
              <Icon name="plus" size={14} /> {t('Create user')}
            </button>
          </div>

          <table className="table">
            <thead>
              <tr>
                <th>{t('User')}</th>
                <th>{t('Accounts')}</th>
                <th>{t('Role')}</th>
                <th>{t('Last seen')}</th>
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
                  <td>{managed.is_admin ? t('admin') : t('user')}</td>
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
                        {managed.is_admin ? t('Demote') : t('Promote')}
                      </button>
                      <button
                        className="btn small ghost"
                        onClick={async () => {
                          const result = await api.resetUserPassword(managed.id)
                          toast('success', t('New password: {password}', { password: result.password }))
                        }}
                      >
                        {t('Reset')}
                      </button>
                      <button
                        className="btn small danger"
                        onClick={async () => {
                          if (!window.confirm(t('Delete {name}?', { name: managed.name }))) return
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
