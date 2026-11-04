import { useState } from 'react'
import { api, type MailSettings } from '../api/client'
import type { ApiError } from '../api/types'
import { useT } from '../i18n'
import { useLoaded } from '../lib/hooks'
import { useStore } from '../store/useStore'
import Menu from './Menu'

/** SMTP transport of the instance: password resets, confirmations and alerts. */
export default function MailSettingsPanel() {
  const t = useT()
  const { data, error, setData } = useLoaded('mail-settings', () => api.mailSettings(), { refresh: false })
  if (!data) return <p className="hint">{error || t('Loading…')}</p>
  return <MailForm loaded={data} onSaved={(saved) => setData(() => saved)} />
}

function MailForm({ loaded, onSaved }: { loaded: MailSettings; onSaved: (saved: MailSettings) => void }) {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const user = useStore((state) => state.user)
  const [settings, setSettings] = useState(loaded)
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState('')

  const change = <K extends keyof MailSettings>(key: K, value: MailSettings[K]) =>
    setSettings((current) => ({ ...current, [key]: value }))

  async function save() {
    setBusy(true)
    setStatus('')
    try {
      const updated = await api.saveMailSettings({
        mailer: settings.mailer,
        host: settings.host,
        port: settings.port,
        username: settings.username,
        encryption: settings.encryption,
        verify_tls: settings.verify_tls,
        from_name: settings.from_name,
        from_address: settings.from_address,
        ...(password ? { password } : {}),
      })
      setSettings(updated)
      onSaved(updated)
      setPassword('')
      toast('success', t('Mail settings saved'))
    } catch (caught) {
      setStatus((caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  async function clearPassword() {
    setBusy(true)
    try {
      const updated = await api.saveMailSettings({ clear_password: true })
      setSettings(updated)
      onSaved(updated)
      toast('success', t('Mail settings saved'))
    } catch (caught) {
      setStatus((caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  async function sendTest() {
    setBusy(true)
    setStatus(t('Sending…'))
    try {
      await api.sendTestEmail()
      setStatus(t('Test e-mail sent to {email}', { email: user?.email ?? '' }))
      toast('success', t('Test e-mail sent'))
    } catch (caught) {
      setStatus((caught as ApiError).message)
      toast('error', t('The test e-mail could not be sent'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="stack">
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('E-mail delivery')}</h2>
            <p>{t('Used for password resets, address confirmations and security alerts. Save before sending a test.')}</p>
          </div>
          <span className={`chip right${settings.enabled ? ' active' : ''}`}>
            {settings.enabled ? t('Configured') : t('Not configured')}
          </span>
        </div>
        <div className="panel-body">
          <div className="row">
            <div className="field">
              <label htmlFor="mail-mailer">{t('Transport')}</label>
              <select
                id="mail-mailer"
                className="input"
                value={settings.mailer === 'smtp' ? 'smtp' : 'log'}
                onChange={(event) => change('mailer', event.target.value)}
              >
                <option value="smtp">SMTP</option>
                <option value="log">{t('Do not send')}</option>
              </select>
            </div>
            <div className="field">
              <label htmlFor="mail-encryption">{t('Encryption')}</label>
              <select
                id="mail-encryption"
                className="input"
                value={settings.encryption || 'tls'}
                onChange={(event) => change('encryption', event.target.value)}
              >
                <option value="tls">{t('STARTTLS (usually port 587)')}</option>
                <option value="ssl">{t('TLS from the start (usually port 465)')}</option>
                <option value="none">{t('None (local relay only)')}</option>
              </select>
            </div>
          </div>
          <div className="row">
            <div className="field">
              <label htmlFor="mail-host">{t('SMTP server')}</label>
              <input id="mail-host" className="input" value={settings.host} onChange={(event) => change('host', event.target.value)} />
            </div>
            <div className="field">
              <label htmlFor="mail-port">{t('Port')}</label>
              <input
                id="mail-port"
                className="input"
                type="number"
                min={1}
                max={65535}
                value={settings.port || ''}
                onChange={(event) => change('port', Number(event.target.value))}
              />
            </div>
          </div>
          <div className="row">
            <div className="field">
              <label htmlFor="mail-username">{t('Username')}</label>
              <input
                id="mail-username"
                className="input"
                autoComplete="off"
                value={settings.username}
                onChange={(event) => change('username', event.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="mail-password">{t('Password')}</label>
              <input
                id="mail-password"
                className="input"
                type="password"
                autoComplete="new-password"
                placeholder={settings.password_set ? t('Stored encrypted, leave empty to keep it') : ''}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </div>
          </div>
          <div className="row">
            <div className="field">
              <label htmlFor="mail-from">{t('Sender address')}</label>
              <input
                id="mail-from"
                className="input"
                type="email"
                value={settings.from_address}
                onChange={(event) => change('from_address', event.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="mail-from-name">{t('Sender name')}</label>
              <input
                id="mail-from-name"
                className="input"
                value={settings.from_name}
                onChange={(event) => change('from_name', event.target.value)}
              />
            </div>
          </div>
          <div className="switch" style={{ marginBottom: 14 }}>
            <div className="switch-label">{t('Verify the certificate of the server')}</div>
            <button
              type="button"
              aria-label={t('Verify the certificate of the server')}
              className={`toggle${settings.verify_tls ? ' on' : ''}`}
              onClick={() => change('verify_tls', !settings.verify_tls)}
            />
          </div>
          <div className="flex wrap">
            <button className="btn primary" onClick={() => void save()} disabled={busy}>
              {t('Save')}
            </button>
            <button className="btn" onClick={() => void sendTest()} disabled={busy || !settings.enabled}>
              {t('Send a test e-mail')}
            </button>
            {settings.password_set && (
              <Menu items={[{ label: t('Remove the stored password'), icon: 'trash', danger: true, onClick: () => void clearPassword() }]} />
            )}
          </div>
          {status && (
            <p className="hint" role="status" style={{ marginTop: 12, overflowWrap: 'anywhere' }}>
              {status}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
