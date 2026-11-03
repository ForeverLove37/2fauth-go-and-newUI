import { useEffect, useState } from 'react'
import { sso, type KeycloakExport, type SsoPreset, type SsoSettings } from '../api/sso'
import { download } from '../lib/format'
import { useT } from '../i18n'
import { useStore } from '../store/useStore'
import Icon from './Icon'

/** Export formats; the labels and hints are English message ids resolved with t() at render time. */
const formats: { id: 'client' | 'partial' | 'cli'; label: string; hint: string }[] = [
  { id: 'client', label: 'Client JSON', hint: 'Keycloak admin console → Clients → Import client' },
  { id: 'partial', label: 'Realm import', hint: 'Realm settings → Partial import' },
  { id: 'cli', label: 'kcadm script', hint: 'Create directly with kcadm.sh' },
]

/** Admin panel: generic OIDC provider settings plus a one-click Keycloak client export. */
export default function SsoSettingsPanel() {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const [settings, setSettings] = useState<SsoSettings | null>(null)
  const [presets, setPresets] = useState<SsoPreset[]>([])
  const [issuer, setIssuer] = useState('')
  const [clientId, setClientId] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [scopes, setScopes] = useState('openid profile email')
  const [label, setLabel] = useState(t('Sign in with SSO'))
  const [enabled, setEnabled] = useState(false)
  const [autoRegister, setAutoRegister] = useState(true)
  const [autoRedirect, setAutoRedirect] = useState(false)
  const [trustEmail, setTrustEmail] = useState(false)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState('')
  const [realm, setRealm] = useState('')
  const [keycloak, setKeycloak] = useState<KeycloakExport | null>(null)

  useEffect(() => {
    void (async () => {
      try {
        const loaded = await sso.settings()
        apply(loaded)
      } catch (error) {
        setStatus((error as Error).message)
      }
      try {
        setPresets(await sso.presets())
      } catch {
        setPresets([])
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function apply(loaded: SsoSettings) {
    setSettings(loaded)
    setIssuer(loaded.issuer)
    setClientId(loaded.client_id)
    setScopes(loaded.scopes || 'openid profile email')
    setLabel(loaded.label || t('Sign in with SSO'))
    setEnabled(loaded.enabled)
    setAutoRegister(loaded.auto_register)
    setAutoRedirect(loaded.auto_redirect)
    setTrustEmail(Boolean(loaded.trust_unverified_email))
    setClientSecret('')
  }

  async function save() {
    setBusy(true)
    setStatus('')
    try {
      const payload: Record<string, unknown> = {
        enabled,
        issuer: issuer.trim(),
        client_id: clientId.trim(),
        scopes: scopes.trim(),
        label: label.trim(),
        auto_register: autoRegister,
        auto_redirect: autoRedirect,
        trust_unverified_email: trustEmail,
      }
      if (clientSecret) payload.client_secret = clientSecret
      const updated = await sso.saveSettings(payload)
      apply(updated)
      toast('success', t('SSO settings saved'))
    } catch (error) {
      setStatus((error as Error).message)
      toast('error', (error as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function test() {
    setBusy(true)
    setStatus(t('Probing .well-known/openid-configuration…'))
    try {
      const result = await sso.test()
      if (result.ok && result.discovery) {
        setStatus(t('Connected: {issuer}', { issuer: result.discovery.issuer }))
        toast('success', t('OIDC discovery succeeded'))
      } else {
        setStatus(result.error ?? t('Discovery failed'))
        toast('error', result.error ?? t('OIDC discovery failed'))
      }
    } catch (error) {
      setStatus((error as Error).message)
      toast('error', (error as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function exportKeycloak(format: 'client' | 'partial' | 'cli') {
    setBusy(true)
    try {
      const result = await sso.keycloak(format, { realm: realm || undefined, client_id: clientId || undefined })
      setKeycloak(result)
      download(result.filename, result.content, format === 'cli' ? 'text/plain' : 'application/json')
      toast(
        'success',
        result.secret_generated
          ? t('New client secret generated and saved')
          : t('Exported {filename}', { filename: result.filename }),
      )
    } catch (error) {
      toast('error', (error as Error).message)
    } finally {
      setBusy(false)
    }
  }

  function applyPreset(preset: SsoPreset) {
    if (preset.issuer) {
      setIssuer(preset.issuer)
      if (/keycloak/i.test(preset.id) || preset.issuer.includes('/realms/')) {
        const derived = preset.issuer.match(/\/realms\/([^/]+)/)?.[1]
        if (derived) setRealm(derived)
      }
    }
    if (preset.scopes) setScopes(preset.scopes)
    setStatus(preset.hint)
  }

  return (
    <div className="stack">
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('Single sign-on (OIDC)')}</h2>
            <p>
              {t(
                'Works with any standard OpenID Provider: Keycloak, Authentik, Auth0, Okta, Google, Microsoft Entra, GitLab, Authelia and more.',
              )}
            </p>
          </div>
          <label className="right flex" style={{ gap: 8 }}>
            <span className="hint">{enabled ? t('Enabled') : t('Disabled')}</span>
            <button className={`toggle${enabled ? ' on' : ''}`} onClick={() => setEnabled(!enabled)} aria-pressed={enabled} />
          </label>
        </div>
        <div className="panel-body">
          {presets.length > 0 && (
            <div className="field">
              <label>{t('Fill from a provider preset')}</label>
              <div className="flex wrap">
                {presets.map((preset) => (
                  <button key={preset.id} className="chip" onClick={() => applyPreset(preset)} title={preset.hint}>
                    {preset.name}
                  </button>
                ))}
              </div>
            </div>
          )}

          <div className="row">
            <div className="field">
              <label>Issuer URL</label>
              <input
                className="input mono"
                placeholder="https://keycloak.example.com/realms/main"
                value={issuer}
                onChange={(event) => setIssuer(event.target.value)}
                spellCheck={false}
              />
            </div>
          </div>

          <div className="row">
            <div className="field">
              <label>Client ID</label>
              <input className="input mono" value={clientId} onChange={(event) => setClientId(event.target.value)} />
            </div>
            <div className="field">
              <label>{settings?.client_secret_set ? t('Client secret (set, leave empty to keep it)') : t('Client secret')}</label>
              <input
                className="input mono"
                type="password"
                placeholder={settings?.client_secret_set ? '••••••••' : t('Not set')}
                value={clientSecret}
                onChange={(event) => setClientSecret(event.target.value)}
              />
            </div>
          </div>

          <div className="row">
            <div className="field">
              <label>{t('Scopes')}</label>
              <input className="input mono" value={scopes} onChange={(event) => setScopes(event.target.value)} />
            </div>
            <div className="field">
              <label>{t('Login button label')}</label>
              <input className="input" value={label} onChange={(event) => setLabel(event.target.value)} />
            </div>
          </div>

          <div className="field">
            <label>{t('Callback URL (register it at the IdP)')}</label>
            <div className="flex">
              <input className="input mono grow" readOnly value={settings?.redirect_uri ?? ''} />
              <button
                className="btn small"
                onClick={() => {
                  void navigator.clipboard?.writeText(settings?.redirect_uri ?? '')
                  toast('success', t('Callback URL copied'))
                }}
              >
                <Icon name="copy" size={14} /> {t('Copy')}
              </button>
            </div>
          </div>

          <div className="switch">
            <div className="switch-label">
              {t('Create accounts automatically')}
              <small>{t('Create the account from the e-mail address on the first SSO sign-in')}</small>
            </div>
            <button className={`toggle${autoRegister ? ' on' : ''}`} onClick={() => setAutoRegister(!autoRegister)} />
          </div>
          <div className="switch">
            <div className="switch-label">
              {t('Redirect to the IdP from the login page')}
              <small>{t('Useful when SSO is the only way in')}</small>
            </div>
            <button className={`toggle${autoRedirect ? ' on' : ''}`} onClick={() => setAutoRedirect(!autoRedirect)} />
          </div>
          <div className="switch">
            <div className="switch-label">
              {t('Trust e-mail addresses the provider has not verified')}
              <small>{t('Only for providers that never send email_verified. Anyone able to choose their address there could become the local user with that address.')}</small>
            </div>
            <button className={`toggle${trustEmail ? ' on' : ''}`} onClick={() => setTrustEmail(!trustEmail)} />
          </div>

          <div className="flex wrap" style={{ marginTop: 14 }}>
            <button className="btn primary small" onClick={save} disabled={busy}>
              <Icon name="check" size={14} /> {t('Save settings')}
            </button>
            <button className="btn small" onClick={test} disabled={busy || !issuer}>
              <Icon name="refresh" size={14} /> {t('Test connection')}
            </button>
            {clientSecret && (
              <button
                className="btn small danger"
                onClick={async () => {
                  setBusy(true)
                  try {
                    apply(await sso.saveSettings({ clear_secret: true }))
                    toast('success', t('Client secret cleared'))
                  } catch (error) {
                    toast('error', (error as Error).message)
                  } finally {
                    setBusy(false)
                  }
                }}
                disabled={busy}
              >
                {t('Clear secret')}
              </button>
            )}
          </div>
          {status && <p className="hint" style={{ marginTop: 10 }}>{status}</p>}
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('One-click Keycloak setup')}</h2>
            <p>
              {t(
                'Generate a client configuration you can import straight into Keycloak; the first run creates and saves a client secret.',
              )}
            </p>
          </div>
        </div>
        <div className="panel-body">
          <div className="row">
            <div className="field">
              <label>{t('Realm (optional, inferred from the Issuer)')}</label>
              <input className="input mono" value={realm} onChange={(event) => setRealm(event.target.value)} placeholder="main" />
            </div>
            <div className="field">
              <label>Client ID</label>
              <input className="input mono" value={clientId} onChange={(event) => setClientId(event.target.value)} placeholder="2fauth" />
            </div>
          </div>

          <div className="flex wrap">
            {formats.map((format) => (
              <button
                key={format.id}
                className="btn small"
                onClick={() => void exportKeycloak(format.id)}
                disabled={busy}
                title={t(format.hint)}
              >
                <Icon name="download" size={14} /> {t(format.label)}
              </button>
            ))}
          </div>

          {keycloak && (
            <div className="panel" style={{ marginTop: 14, padding: 14 }}>
              <div className="flex">
                <strong className="mono small">{keycloak.filename}</strong>
                <span className="right hint">
                  {keycloak.secret_generated ? t('New secret generated and saved') : t('Using the saved secret')}
                </span>
              </div>
              <p className="hint" style={{ margin: '8px 0' }}>{keycloak.notice}</p>
              <textarea readOnly value={keycloak.content} style={{ minHeight: 160 }} />
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
