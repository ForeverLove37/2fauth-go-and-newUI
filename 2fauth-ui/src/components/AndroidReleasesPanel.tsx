import { useState } from 'react'
import { api, BASE, type AndroidRelease } from '../api/client'
import type { ApiError } from '../api/types'
import { useT } from '../i18n'
import { useLoaded } from '../lib/hooks'
import { useStore } from '../store/useStore'
import Icon from './Icon'
import Section from './Section'

function megabytes(size: number): string {
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

/**
 * Administrators publish builds of the phone app here. The newest one is offered on
 * the landing page, and the installed apps find it and notify their users.
 */
export default function AndroidReleasesPanel() {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const { data: releases, error: loadError, reload } = useLoaded<AndroidRelease[]>('android-releases', () => api.androidReleases())
  const [file, setFile] = useState<File | null>(null)
  const [versionCode, setVersionCode] = useState('')
  const [versionName, setVersionName] = useState('')
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)
  const [errors, setErrors] = useState<Record<string, string[]>>({})

  const latest = releases?.[0]
  const nextCode = latest ? String(latest.version_code + 1) : '1'

  async function publish(event: React.FormEvent) {
    event.preventDefault()
    if (!file) return
    setBusy(true)
    setErrors({})
    try {
      await api.publishAndroid(file, Number(versionCode || nextCode), versionName.trim(), notes.trim())
      toast('success', t('Version {name} published', { name: versionName.trim() }))
      setFile(null)
      setVersionCode('')
      setVersionName('')
      setNotes('')
      reload()
    } catch (caught) {
      const typed = caught as ApiError
      setErrors(typed.errors ?? {})
      toast('error', typed.message)
    } finally {
      setBusy(false)
    }
  }

  async function remove(code: number) {
    if (!window.confirm(t('Withdraw this version? Phones that already have it keep it.'))) return
    try {
      await api.deleteAndroid(code)
      reload()
    } catch (caught) {
      toast('error', (caught as Error).message)
    }
  }

  return (
    <div className="stack">
      <Section
        id="android-releases"
        title={t('Android app')}
        hint={t('The newest version is offered on the home page; installed apps check for it and notify their users.')}
        defaultOpen
        summary={latest ? latest.version_name : undefined}
      >
        {!releases && <p className="hint">{loadError || t('Loading…')}</p>}
        {releases && releases.length === 0 && <p className="hint">{t('No version published yet.')}</p>}
        {releases && releases.length > 0 && (
          <table className="table">
            <thead>
              <tr>
                <th>{t('Version')}</th>
                <th>{t('Published')}</th>
                <th>{t('Size')}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {releases.map((release, index) => (
                <tr key={release.version_code}>
                  <td>
                    <strong>{release.version_name}</strong> <span className="hint mono">#{release.version_code}</span>
                    {index === 0 && <span className="flag hot" style={{ marginLeft: 8 }}>{t('Latest')}</span>}
                    {release.notes && <div className="hint small" style={{ whiteSpace: 'pre-line' }}>{release.notes}</div>}
                  </td>
                  <td className="hint">{new Date(release.published_at * 1000).toLocaleString()}</td>
                  <td className="hint">{megabytes(release.size)}</td>
                  <td className="right">
                    <a className="btn icon ghost" href={`${BASE}${release.url}`} title={t('Download')}>
                      <Icon name="download" size={14} />
                    </a>
                    <button className="btn icon ghost" onClick={() => void remove(release.version_code)} title={t('Withdraw')}>
                      <Icon name="trash" size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      <Section id="android-publish" title={t('Publish a version')} hint={t('A signed APK. Phones install it over the old version only if it is signed with the same key.')} defaultOpen>
        <form onSubmit={publish} className="stack">
          <div className="field">
            <label>{t('Package (.apk)')}</label>
            <input className="input" type="file" accept=".apk,application/vnd.android.package-archive" onChange={(event) => setFile(event.target.files?.[0] ?? null)} />
            {errors.apk && <span className="error-text">{errors.apk[0]}</span>}
          </div>
          <div className="row">
            <div className="field">
              <label>{t('Version name')}</label>
              <input className="input" placeholder="1.2.0" value={versionName} onChange={(event) => setVersionName(event.target.value)} />
              {errors.version_name && <span className="error-text">{errors.version_name[0]}</span>}
            </div>
            <div className="field">
              <label>{t('Version code')}</label>
              <input className="input mono" inputMode="numeric" placeholder={nextCode} value={versionCode} onChange={(event) => setVersionCode(event.target.value.replace(/\D/g, ''))} />
              {errors.version_code && <span className="error-text">{errors.version_code[0]}</span>}
            </div>
          </div>
          <div className="field">
            <label>{t('What is new')}</label>
            <textarea value={notes} onChange={(event) => setNotes(event.target.value)} style={{ minHeight: 90 }} />
          </div>
          <div>
            <button className="btn primary small" disabled={busy || !file || !versionName.trim()}>
              <Icon name="upload" size={14} /> {busy ? t('Publishing…') : t('Publish')}
            </button>
          </div>
        </form>
      </Section>
    </div>
  )
}
