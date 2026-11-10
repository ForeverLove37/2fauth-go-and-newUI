import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import type { ApiError } from '../api/types'
import Icon from '../components/Icon'
import Menu from '../components/Menu'
import Section from '../components/Section'
import ThemeEditor from './ThemeEditor'
import IconProvidersPanel from '../components/IconProvidersPanel'
import SsoSettingsPanel from '../components/SsoSettingsPanel'
import MailSettingsPanel from '../components/MailSettingsPanel'
import SshKeysPanel from '../components/SshKeysPanel'
import { useI18nStore, useT } from '../i18n'
import { useLoaded } from '../lib/hooks'
import { adminTabs, isAdminTab, personalTabs, type SettingsTab } from '../lib/settingsTabs'
import { useStore } from '../store/useStore'

const tabLabels: Record<SettingsTab, string> = {
  theme: 'Appearance',
  options: 'Options',
  account: 'Account',
  security: 'Security',
  admin: 'General',
  sso: 'Single sign-on',
  mail: 'E-mail',
}

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
  const wanted = params.get('tab')
  const admin = isAdminTab(wanted) && Boolean(user?.is_admin)
  const tabs: readonly SettingsTab[] = admin ? adminTabs : personalTabs
  const tab = tabs.find((item) => item === wanted) ?? tabs[0]

  return (
    <div className="stage" style={{ overflow: 'auto' }}>
      <div style={{ padding: '18px 26px 40px', maxWidth: 1020, margin: '0 auto', width: '100%' }}>
        <div className="tabs" style={{ marginBottom: 20 }}>
          {tabs.map((item) => (
            <button
              key={item}
              className={tab === item ? 'active' : ''}
              onClick={() => setParams(item === 'theme' ? {} : { tab: item })}
            >
              {t(tabLabels[item])}
            </button>
          ))}
        </div>

        {tab === 'theme' && <ThemeEditor />}
        {tab === 'options' && <OptionsTab prefs={prefs} locked={locked} setPreference={setPreference} />}
        {tab === 'account' && <AccountTab />}
        {tab === 'security' && <SecurityTab />}
        {tab === 'sso' && <SsoSettingsPanel />}
        {tab === 'mail' && <MailSettingsPanel />}
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
          label={t('Hide codes')}
          hint={t('Codes appear for 5 seconds when the mouse moves onto them. Copying works either way.')}
          value={prefs.showOtpAsDot !== false}
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
          label={t('Fetch official icons')}
          hint={t('Accounts without an icon get one from the icon libraries. This tells them the name of the service.')}
          value={prefs.getOfficialIcons !== false}
          disabled={isLocked('getOfficialIcons')}
          onChange={(value) => void setPreference('getOfficialIcons', value)}
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
  const [params] = useSearchParams()
  const verifiedParam = params.get('email_verified')

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

  async function sendVerification() {
    try {
      await api.sendEmailVerification()
      toast('success', t('Check your mailbox for the confirmation link'))
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  async function savePassword() {
    try {
      await api.updatePassword(currentPassword, newPassword)
      toast('success', t('Password changed. Your other devices were signed out and your tokens revoked.'))
      setCurrentPassword('')
      setNewPassword('')
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  return (
    <div className="stack">
      <Section
        id="account-profile"
        title={t('Profile')}
        hint={t('Your current password is required to save changes.')}
        defaultOpen
        summary={user?.email}
      >
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
        {verifiedParam === 'ok' && <p className="hint" role="status" style={{ marginTop: 12 }}>{t('Your e-mail address is confirmed.')}</p>}
        {verifiedParam === 'invalid' && <p className="error-text" style={{ marginTop: 12 }}>{t('This confirmation link is invalid or expired.')}</p>}
        {user && !user.email_verified && (
          <div className="flex wrap" style={{ marginTop: 12 }}>
            <span className="hint">{t('This e-mail address is not confirmed yet.')}</span>
            <button className="btn small" onClick={() => void sendVerification()}>
              {t('Send a confirmation link')}
            </button>
          </div>
        )}
      </Section>

      <Section
        id="account-password"
        title={t('Password')}
        hint={t('Minimum 8 characters, confirmed automatically.')}
      >
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
      </Section>
    </div>
  )
}

function SecurityTab() {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const { data: tokens, error: tokensError, reload: reloadTokens } = useLoaded('tokens', () => api.tokens())
  const { data: credentials, reload: reloadCredentials } = useLoaded('credentials', () => api.credentials())
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')

  async function create() {
    if (!name.trim()) return
    try {
      const response = await api.createToken(name.trim())
      setFresh(response.accessToken)
      setName('')
      await reloadTokens()
      toast('success', t('Token created'))
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  async function removeToken(id: string) {
    await api.deleteToken(id)
    await reloadTokens()
  }

  async function removeCredential(id: string) {
    if (!window.confirm(t('Remove this passkey?'))) return
    await api.deleteCredential(id)
    await reloadCredentials()
  }

  return (
    <div className="stack">
      <SshKeysPanel />
      <Section
        id="security-tokens"
        title={t('Personal access tokens')}
        hint={t('Use them as `Authorization: Bearer <token>` for scripts and the CLI.')}
        summary={tokens ? String(tokens.length) : undefined}
      >
        <p className="hint small" style={{ margin: '0 0 12px' }}>
          <Link to="/agents">{t('Guide for AI agents and scripts')} →</Link>
        </p>
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
        {!tokens ? (
          <p className="hint">{tokensError || t('Loading…')}</p>
        ) : tokens.length === 0 ? (
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
      </Section>

      {credentials && credentials.length > 0 && (
        <Section
          id="security-passkeys"
          title={t('Passkeys')}
          hint={t('Registered WebAuthn credentials. Registration requires the PHP-compatible WebAuthn endpoints.')}
        >
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
        </Section>
      )}
    </div>
  )
}

function AdminTab() {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const { data: users, reload: reloadUsers } = useLoaded('admin-users', () => api.users())
  const { data: settings, error, setData: setSettings } = useLoaded('admin-settings', () => api.settings())
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isAdmin, setIsAdmin] = useState(false)

  async function createUser() {
    try {
      await api.createUser({ name, email, password, is_admin: isAdmin })
      toast('success', t('User created'))
      setName('')
      setEmail('')
      setPassword('')
      setIsAdmin(false)
      setAdding(false)
      await reloadUsers()
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

  // Nothing is drawn before the answer: these switches have no meaningful default to show.
  if (!settings) return <p className="hint">{error || t('Loading…')}</p>

  return (
    <div className="stack">
      <Section
        id="admin-application"
        title={t('Application settings')}
        hint={t('Instance wide switches.')}
        defaultOpen
        summary={settings.disableRegistration ? t('Registrations closed') : t('Registrations open')}
      >
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
      </Section>

      <Section
        id="admin-signin"
        title={t('Sign-in policy')}
        hint={t('Who can sign in and who can create an account.')}
        summary={settings.useSsoOnly ? t('SSO required') : t('Password or SSO')}
      >
        <Toggle
          label={t('Require single sign-on')}
          hint={t('Password sign-in stays available to administrators only, as a way back in')}
          value={Boolean(settings.useSsoOnly)}
          onChange={(value) => void toggleSetting('useSsoOnly', value)}
        />
        <Toggle
          label={t('Keep SSO sign-ups open')}
          hint={t('New SSO users can register even when registrations are closed')}
          value={Boolean(settings.keepSsoRegistrationEnabled)}
          onChange={(value) => void toggleSetting('keepSsoRegistrationEnabled', value)}
        />
        <Toggle
          label={t('Restrict registrations')}
          hint={t('Only the addresses below can create an account')}
          value={Boolean(settings.restrictRegistration)}
          onChange={(value) => void toggleSetting('restrictRegistration', value)}
        />
        {Boolean(settings.restrictRegistration) && (
          <>
            <div className="field" style={{ marginTop: 12 }}>
              <label htmlFor="restrict-list">{t('Addresses or @domains, one per line')}</label>
              <textarea
                id="restrict-list"
                value={String(settings.restrictList ?? '')}
                onChange={(event) => setSettings((current) => ({ ...current, restrictList: event.target.value }))}
                onBlur={(event) => void toggleSetting('restrictList', event.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="restrict-rule">{t('Rule: empty or "allow", "deny", or a regular expression that also allows')}</label>
              <input
                id="restrict-rule"
                className="input"
                value={String(settings.restrictRule ?? '')}
                onChange={(event) => setSettings((current) => ({ ...current, restrictRule: event.target.value }))}
                onBlur={(event) => void toggleSetting('restrictRule', event.target.value)}
              />
            </div>
          </>
        )}
      </Section>

      <Section id="admin-icons" title={t('Icon libraries')} hint={t('When an icon is fetched the libraries are tried in order; the first hit wins and the rest act as fallbacks.')}>
        <IconProvidersPanel />
      </Section>

      <Section
        id="admin-users"
        title={t('Users')}
        summary={users ? String(users.length) : undefined}
        actions={
            <button className="btn small" onClick={() => setAdding(!adding)} aria-expanded={adding}>
              <Icon name="plus" size={14} /> {t('Add user')}
            </button>
          }
      >
        {adding && (
          <>
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
          </>
        )}

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
            {(users ?? []).map((managed) => (
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
                    <Menu
                      items={[
                        {
                          label: managed.is_admin ? t('Demote') : t('Promote'),
                          onClick: () => void api.promoteUser(managed.id, !managed.is_admin).then(reloadUsers),
                        },
                        {
                          label: t('Reset password'),
                          onClick: async () => {
                            const result = await api.resetUserPassword(managed.id)
                            toast('success', t('New password: {password}', { password: result.password }))
                          },
                        },
                        {
                          label: t('Delete'),
                          icon: 'trash',
                          danger: true,
                          onClick: async () => {
                            if (!window.confirm(t('Delete {name}?', { name: managed.name }))) return
                            try {
                              await api.deleteUser(managed.id)
                              await reloadUsers()
                            } catch (caught) {
                              toast('error', (caught as ApiError).message)
                            }
                          },
                        },
                      ]}
                    />
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Section>
    </div>
  )
}
