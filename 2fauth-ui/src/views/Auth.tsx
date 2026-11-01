import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import Icon from '../components/Icon'
import { useStore } from '../store/useStore'
import { sso, type SsoConfig } from '../api/sso'
import type { ApiError } from '../api/types'

const ssoErrors: Record<string, string> = {
  cancelled: '已取消 SSO 登录',
  state: 'SSO 会话已失效，请重试',
  provider: '无法连接身份提供方，请检查 Issuer 配置',
  exchange: '无法用授权码换取令牌，请检查 client secret 与回调地址',
  userinfo: '身份提供方未返回用户信息',
  email: '身份提供方未返回邮箱地址',
  register_disabled: '本实例已关闭注册，无法自动创建账号',
  account: '无法匹配到账号，请联系管理员',
  server: '服务器内部错误',
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
    if (code) setSsoError(ssoErrors[code] ?? `SSO 登录失败（${code}）`)
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
      <form className="auth-card" onSubmit={submit}>
        <div className="auth-hero">
          <div className="brand-mark">2F</div>
          <h1>欢迎回来</h1>
          <p className="sub">登录你的 {config?.appName ?? '2FAuth'} 保险库</p>
        </div>

        <div className="field">
          <label htmlFor="email">Email</label>
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
          <label htmlFor="password">Password</label>
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

        {!ssoOnly && (
          <button className="btn primary" type="submit" disabled={busy}>
            {busy ? '正在登录…' : '登录'}
          </button>
        )}

        {ssoConfig?.enabled && (
          <>
            <div className="auth-divider">或</div>
            <button type="button" className="btn sso-btn" onClick={() => void startSso()}>
              <Icon name="shield" size={15} />
              {ssoConfig.label || '使用 SSO 登录'}
            </button>
          </>
        )}

        {registrationOpen && (
          <p className="auth-switch">
            No account yet? <Link to="/register">Create one</Link>
          </p>
        )}
      </form>
    </div>
  )
}

export function Register() {
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
          <h1>Registration closed</h1>
          <p className="sub">This instance does not accept new accounts.</p>
          <Link className="btn primary" to="/login">
            Back to sign in
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div className="auth">
      <form className="auth-card" onSubmit={submit}>
        <div className="auth-hero">
          <div className="brand-mark">2F</div>
          <h1>创建账号</h1>
          <p className="sub">第一个注册的账号将成为管理员</p>
        </div>

        <div className="field">
          <label htmlFor="name">Name</label>
          <input id="name" className="input" value={name} onChange={(event) => setName(event.target.value)} required autoFocus />
          <FieldError messages={fields.name} />
        </div>

        <div className="field">
          <label htmlFor="email">Email</label>
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
            <label htmlFor="password">Password</label>
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
            <label htmlFor="confirmation">Confirm</label>
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
          {busy ? 'Creating…' : 'Create account'}
        </button>

        <p className="auth-switch">
          Already registered? <Link to="/login">Sign in</Link>
        </p>
      </form>
    </div>
  )
}
