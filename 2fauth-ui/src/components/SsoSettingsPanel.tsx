import { useEffect, useState } from 'react'
import { sso, type KeycloakExport, type SsoPreset, type SsoSettings } from '../api/sso'
import { download } from '../lib/format'
import { useStore } from '../store/useStore'
import Icon from './Icon'

const formats: { id: 'client' | 'partial' | 'cli'; label: string; hint: string }[] = [
  { id: 'client', label: 'Client JSON', hint: 'Keycloak 管理台 → Clients → Import client' },
  { id: 'partial', label: 'Realm 导入', hint: 'Realm settings → Partial import' },
  { id: 'cli', label: 'kcadm 脚本', hint: '用 kcadm.sh 直接创建' },
]

/** Admin panel: generic OIDC provider settings plus a one-click Keycloak client export. */
export default function SsoSettingsPanel() {
  const toast = useStore((state) => state.toast)
  const [settings, setSettings] = useState<SsoSettings | null>(null)
  const [presets, setPresets] = useState<SsoPreset[]>([])
  const [issuer, setIssuer] = useState('')
  const [clientId, setClientId] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [scopes, setScopes] = useState('openid profile email')
  const [label, setLabel] = useState('使用 SSO 登录')
  const [enabled, setEnabled] = useState(false)
  const [autoRegister, setAutoRegister] = useState(true)
  const [autoRedirect, setAutoRedirect] = useState(false)
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
    setLabel(loaded.label || '使用 SSO 登录')
    setEnabled(loaded.enabled)
    setAutoRegister(loaded.auto_register)
    setAutoRedirect(loaded.auto_redirect)
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
      }
      if (clientSecret) payload.client_secret = clientSecret
      const updated = await sso.saveSettings(payload)
      apply(updated)
      toast('success', 'SSO 配置已保存')
    } catch (error) {
      setStatus((error as Error).message)
      toast('error', (error as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function test() {
    setBusy(true)
    setStatus('正在探测 .well-known/openid-configuration…')
    try {
      const result = await sso.test()
      if (result.ok && result.discovery) {
        setStatus(`连接成功：${result.discovery.issuer}`)
        toast('success', 'OIDC 发现成功')
      } else {
        setStatus(result.error ?? '发现失败')
        toast('error', result.error ?? 'OIDC 发现失败')
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
      toast('success', result.secret_generated ? '已生成并保存新的 client secret' : `已导出 ${result.filename}`)
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
            <h2>单点登录（OIDC）</h2>
            <p>支持 Keycloak、Authentik、Auth0、Okta、Google、Microsoft Entra、GitLab、Authelia 等标准 OpenID Provider。</p>
          </div>
          <label className="right flex" style={{ gap: 8 }}>
            <span className="hint">{enabled ? '已启用' : '已停用'}</span>
            <button className={`toggle${enabled ? ' on' : ''}`} onClick={() => setEnabled(!enabled)} aria-pressed={enabled} />
          </label>
        </div>
        <div className="panel-body">
          {presets.length > 0 && (
            <div className="field">
              <label>快速填入服务商模板</label>
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
              <label>Client secret {settings?.client_secret_set ? '（已配置，留空则不修改）' : ''}</label>
              <input
                className="input mono"
                type="password"
                placeholder={settings?.client_secret_set ? '••••••••' : '未配置'}
                value={clientSecret}
                onChange={(event) => setClientSecret(event.target.value)}
              />
            </div>
          </div>

          <div className="row">
            <div className="field">
              <label>Scopes</label>
              <input className="input mono" value={scopes} onChange={(event) => setScopes(event.target.value)} />
            </div>
            <div className="field">
              <label>登录按钮文案</label>
              <input className="input" value={label} onChange={(event) => setLabel(event.target.value)} />
            </div>
          </div>

          <div className="field">
            <label>回调地址（在 IdP 中登记）</label>
            <div className="flex">
              <input className="input mono grow" readOnly value={settings?.redirect_uri ?? ''} />
              <button
                className="btn small"
                onClick={() => {
                  void navigator.clipboard?.writeText(settings?.redirect_uri ?? '')
                  toast('success', '已复制回调地址')
                }}
              >
                <Icon name="copy" size={14} /> 复制
              </button>
            </div>
          </div>

          <div className="switch">
            <div className="switch-label">
              允许自动创建账号
              <small>用户首次通过 SSO 登录时，按邮箱自动建立账号</small>
            </div>
            <button className={`toggle${autoRegister ? ' on' : ''}`} onClick={() => setAutoRegister(!autoRegister)} />
          </div>
          <div className="switch">
            <div className="switch-label">
              打开登录页时自动跳转 IdP
              <small>只保留 SSO 登录时很有用</small>
            </div>
            <button className={`toggle${autoRedirect ? ' on' : ''}`} onClick={() => setAutoRedirect(!autoRedirect)} />
          </div>

          <div className="flex wrap" style={{ marginTop: 14 }}>
            <button className="btn primary small" onClick={save} disabled={busy}>
              <Icon name="check" size={14} /> 保存配置
            </button>
            <button className="btn small" onClick={test} disabled={busy || !issuer}>
              <Icon name="refresh" size={14} /> 测试连接
            </button>
            {clientSecret && (
              <button
                className="btn small danger"
                onClick={async () => {
                  setBusy(true)
                  try {
                    apply(await sso.saveSettings({ clear_secret: true }))
                    toast('success', '已清除 client secret')
                  } catch (error) {
                    toast('error', (error as Error).message)
                  } finally {
                    setBusy(false)
                  }
                }}
                disabled={busy}
              >
                清除 secret
              </button>
            )}
          </div>
          {status && <p className="hint" style={{ marginTop: 10 }}>{status}</p>}
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>Keycloak 一键接入</h2>
            <p>生成可直接导入 Keycloak 的 client 配置；首次生成会自动创建并保存 client secret。</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="row">
            <div className="field">
              <label>Realm（可留空，从 Issuer 推断）</label>
              <input className="input mono" value={realm} onChange={(event) => setRealm(event.target.value)} placeholder="main" />
            </div>
            <div className="field">
              <label>Client ID</label>
              <input className="input mono" value={clientId} onChange={(event) => setClientId(event.target.value)} placeholder="2fauth" />
            </div>
          </div>

          <div className="flex wrap">
            {formats.map((format) => (
              <button key={format.id} className="btn small" onClick={() => void exportKeycloak(format.id)} disabled={busy} title={format.hint}>
                <Icon name="download" size={14} /> {format.label}
              </button>
            ))}
          </div>

          {keycloak && (
            <div className="panel" style={{ marginTop: 14, padding: 14 }}>
              <div className="flex">
                <strong className="mono small">{keycloak.filename}</strong>
                <span className="right hint">{keycloak.secret_generated ? '已生成新 secret 并保存' : '使用已保存的 secret'}</span>
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
