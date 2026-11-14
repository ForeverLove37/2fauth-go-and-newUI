import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import Icon from '../components/Icon'
import { api, BASE } from '../api/client'
import { useStore } from '../store/useStore'
import { sso, type SsoConfig } from '../api/sso'
import { locales, useI18nStore, useT } from '../i18n'
import type { ApiError } from '../api/types'
import Logo from '../components/Logo'

const ssoErrors: Record<string, string> = {
  cancelled: 'SSO sign-in was cancelled',
  state: 'The SSO session expired, please try again',
  provider: 'Could not reach the identity provider — check the issuer',
  exchange: 'Could not exchange the authorization code — check the client secret and redirect URI',
  userinfo: 'The identity provider did not return user information',
  email: 'The identity provider did not return an e-mail address',
  email_unverified: 'The identity provider has not verified this e-mail address',
  token: 'The identity token of the provider could not be verified',
  register_disabled: 'This instance does not accept new accounts',
  account: 'No account matches this identity, contact an administrator',
  server: 'Internal server error',
}

/** Language switcher shown before signing in. */
function LanguagePicker() {
  const t = useT()
  const locale = useI18nStore((state) => state.locale)
  const setLocale = useI18nStore((state) => state.setLocale)
  return (
    <div className="auth-lang">
      <Icon name="orbit" size={13} />
      <select value={locale} onChange={(event) => setLocale(event.target.value)} aria-label={t('Language')}>
        {locales.map((item) => (
          <option key={item.id} value={item.id}>
            {item.label}
          </option>
        ))}
      </select>
    </div>
  )
}

function useSubmit() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string[]>>({})
  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    setFields({})
    try {
      await action()
    } catch (caught) {
      const typed = caught as ApiError
      setError(typed.message)
      setFields(typed.errors ?? {})
    } finally {
      setBusy(false)
    }
  }
  return { busy, error, fields, run }
}

function FieldError({ messages }: { messages?: string[] }) {
  if (!messages?.length) return null
  return <span className="error-text">{messages[0]}</span>
}

export function Login() {
  const { login, config } = useStore()
  const navigate = useNavigate()
  const { busy, error, fields, run } = useSubmit()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const t = useT()
  const [params] = useSearchParams()
  const [ssoConfig, setSsoConfig] = useState<SsoConfig | null>(null)
  const [ssoError, setSsoError] = useState('')

  useEffect(() => {
    void sso
      .config()
      .then((loaded) => setSsoConfig(loaded))
      .catch(() => setSsoConfig(null))
  }, [])

  useEffect(() => {
    const code = params.get('sso_error')
    if (code) setSsoError(t(ssoErrors[code] ?? 'SSO sign-in failed ({code})', { code }))
  }, [params])

  async function startSso() {
    setSsoError('')
    try {
      const url = await sso.start()
      window.location.assign(url)
    } catch (caught) {
      setSsoError((caught as Error).message)
    }
  }

  // automatic redirect when the instance is configured for it
  useEffect(() => {
    if (ssoConfig?.enabled && ssoConfig.auto_redirect && !params.get('sso_error')) {
      void startSso()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ssoConfig?.enabled, ssoConfig?.auto_redirect])

  const ssoOnly = Boolean(config?.appSettings?.useSsoOnly) && Boolean(ssoConfig?.enabled)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    await run(async () => {
      await login(email, password)
      navigate('/', { replace: true })
    })
  }

  const registrationOpen = !config?.appSettings?.disableRegistration

  return (
    <div className="auth">
      <LanguagePicker />
      <form className="auth-card" onSubmit={submit}>
        <div className="auth-hero">
          <Logo />
          <h1>{t('Welcome back')}</h1>
          <p className="sub">{t('Sign in to your {app} vault', { app: config?.appName ?? '2FAuth' })}</p>
        </div>

        <div className="field">
          <label htmlFor="email">{t('Email')}</label>
          <input
            id="email"
            className="input"
            type="email"
            autoComplete="username"
            autoFocus
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            required
          />
          <FieldError messages={fields.email} />
        </div>

        <div className="field">
          <label htmlFor="password">{t('Password')}</label>
          <input
            id="password"
            className="input"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            required
          />
          <FieldError messages={fields.password} />
        </div>

        {error && <p className="error-text">{error}</p>}
        {ssoError && <p className="error-text">{ssoError}</p>}

        {/* Under SSO-only the server still accepts the password of an administrator:
            the way back in when the provider is down. */}
        <button className={ssoOnly ? 'btn ghost' : 'btn primary'} type="submit" disabled={busy}>
          {busy ? t('Signing in…') : ssoOnly ? t('Administrator sign-in') : t('Sign in')}
        </button>

        {config?.mailEnabled && !ssoOnly && (
          <p className="auth-switch">
            <Link to="/password/lost">{t('Forgot your password?')}</Link>
          </p>
        )}

        {ssoConfig?.enabled && (
          <>
            <div className="auth-divider">{t('or')}</div>
            <button type="button" className="btn sso-btn" onClick={() => void startSso()}>
              <Icon name="shield" size={15} />
              {ssoConfig.label || t('Sign in with SSO')}
            </button>
          </>
        )}

        {registrationOpen && (
          <p className="auth-switch">
            {t('No account yet?')} <Link to="/register">{t('Create one')}</Link>
          </p>
        )}

        <p className="auth-switch auth-links">
          <Link to="/">{t('About {app}', { app: config?.appName ?? '2FAuth' })}</Link>
          <span aria-hidden="true">·</span>
          <a href={`${BASE}/#android`} onClick={(event) => { event.preventDefault(); navigate('/'); window.setTimeout(() => document.getElementById('android')?.scrollIntoView(), 400) }}>
            {t('Android app')}
          </a>
          <span aria-hidden="true">·</span>
          <Link to="/agents">{t('Agents')}</Link>
          <span aria-hidden="true">·</span>
          <Link to="/terms">{t('Terms')}</Link>
          <span aria-hidden="true">·</span>
          <Link to="/privacy">{t('Privacy')}</Link>
        </p>
      </form>
    </div>
  )
}

/** Asks for a reset link. The answer does not tell whether the address exists. */
export function PasswordLost() {
  const t = useT()
  const { busy, error, fields, run } = useSubmit()
  const [email, setEmail] = useState('')
  const [sent, setSent] = useState(false)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    await run(async () => {
      await api.passwordLost(email)
      setSent(true)
    })
  }

  return (
    <div className="auth">
      <LanguagePicker />
      <form className="auth-card" onSubmit={submit}>
        <div className="auth-hero">
          <Logo />
          <h1>{t('Reset your password')}</h1>
          <p className="sub">{t('We will send a link to the address of your account.')}</p>
        </div>
        {sent ? (
          <p role="status">{t('If an account uses this address, a link is on its way. It works for 60 minutes.')}</p>
        ) : (
          <>
            <div className="field">
              <label htmlFor="email">{t('Email')}</label>
              <input
                id="email"
                className="input"
                type="email"
                autoComplete="username"
                autoFocus
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                required
              />
              <FieldError messages={fields.email} />
            </div>
            {error && <p className="error-text">{error}</p>}
            <button className="btn primary" type="submit" disabled={busy}>
              {t('Send the link')}
            </button>
          </>
        )}
        <p className="auth-switch">
          <Link to="/login">{t('Back to sign in')}</Link>
        </p>
      </form>
    </div>
  )
}

/** Target of the link of the reset e-mail. */
export function PasswordReset() {
  const t = useT()
  const [params] = useSearchParams()
  const { busy, error, fields, run } = useSubmit()
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [done, setDone] = useState(false)
  const email = params.get('email') ?? ''
  const token = params.get('token') ?? ''

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    await run(async () => {
      await api.passwordReset(email, token, password)
      setDone(true)
    })
  }

  return (
    <div className="auth">
      <LanguagePicker />
      <form className="auth-card" onSubmit={submit}>
        <div className="auth-hero">
          <Logo />
          <h1>{t('Choose a new password')}</h1>
          <p className="sub">{email}</p>
        </div>
        {done ? (
          <p role="status">{t('Your password was changed. Your other devices were signed out.')}</p>
        ) : !email || !token ? (
          <p className="error-text">{t('This link is incomplete: open the one from the e-mail again.')}</p>
        ) : (
          <>
            <div className="field">
              <label htmlFor="password">{t('New password')}</label>
              <input
                id="password"
                className="input"
                type="password"
                autoComplete="new-password"
                autoFocus
                minLength={8}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                required
              />
              <FieldError messages={fields.password} />
            </div>
            <div className="field">
              <label htmlFor="confirmation">{t('Confirm')}</label>
              <input
                id="confirmation"
                className="input"
                type="password"
                autoComplete="new-password"
                value={confirmation}
                onChange={(event) => setConfirmation(event.target.value)}
                required
              />
            </div>
            <FieldError messages={fields.email} />
            {error && !fields.email && <p className="error-text">{error}</p>}
            <button className="btn primary" type="submit" disabled={busy || password !== confirmation}>
              {t('Change password')}
            </button>
          </>
        )}
        <p className="auth-switch">
          <Link to="/login">{t('Back to sign in')}</Link>
        </p>
      </form>
    </div>
  )
}

export function Register() {
  const t = useT()
  const { register, config } = useStore()
  const navigate = useNavigate()
  const { busy, error, fields, run } = useSubmit()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    await run(async () => {
      await register(name, email, password, confirmation)
      navigate('/', { replace: true })
    })
  }

  if (config?.appSettings?.disableRegistration) {
    return (
      <div className="auth">
        <div className="auth-card">
          <h1>{t('Registration closed')}</h1>
          <p className="sub">{t('This instance does not accept new accounts.')}</p>
          <Link className="btn primary" to="/login">
            {t('Back to sign in')}
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div className="auth">
      <LanguagePicker />
      <form className="auth-card" onSubmit={submit}>
        <div className="auth-hero">
          <Logo />
          <h1>{t('Create account')}</h1>
          <p className="sub">{t('The first account becomes the administrator')}</p>
        </div>

        <div className="field">
          <label htmlFor="name">{t('Name')}</label>
          <input id="name" className="input" value={name} onChange={(event) => setName(event.target.value)} required autoFocus />
          <FieldError messages={fields.name} />
        </div>

        <div className="field">
          <label htmlFor="email">{t('Email')}</label>
          <input
            id="email"
            className="input"
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            required
          />
          <FieldError messages={fields.email} />
        </div>

        <div className="row">
          <div className="field">
            <label htmlFor="password">{t('Password')}</label>
            <input
              id="password"
              className="input"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
            />
            <FieldError messages={fields.password} />
          </div>
          <div className="field">
            <label htmlFor="confirmation">{t('Confirm')}</label>
            <input
              id="confirmation"
              className="input"
              type="password"
              autoComplete="new-password"
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              required
            />
          </div>
        </div>

        {error && <p className="error-text">{error}</p>}

        <button className="btn primary" type="submit" disabled={busy}>
          {busy ? t('Creating…') : t('Create account')}
        </button>

        <p className="auth-switch">
          {t('Already registered?')} <Link to="/login">{t('Sign in')}</Link>
        </p>
      </form>
    </div>
  )
}
