import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useStore } from '../store/useStore'
import type { ApiError } from '../api/types'

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
          <h1>Welcome back</h1>
          <p className="sub">Sign in to your {config?.appName ?? '2FAuth'} vault</p>
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

        <button className="btn primary" type="submit" disabled={busy}>
          {busy ? 'Signing in…' : 'Sign in'}
        </button>

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
          <h1>Create your account</h1>
          <p className="sub">The first account becomes the administrator</p>
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
