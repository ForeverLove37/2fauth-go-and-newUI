import { useState } from 'react'
import { api, type IconProvider, type IconProvidersResponse } from '../api/client'
import { useT } from '../i18n'
import { useLoaded } from '../lib/hooks'
import { useStore } from '../store/useStore'
import Icon from './Icon'

/**
 * Admin panel: the icon libraries tried when an account icon is fetched, in
 * order. The first library that has the icon wins; the next ones are used as
 * fallbacks, up to "libraries to try".
 */
/** enabled libraries in chain order first, then the disabled ones */
function sortChain(providers: IconProvider[]): IconProvider[] {
  return [...providers].sort((a, b) => {
    if (a.enabled !== b.enabled) return a.enabled ? -1 : 1
    if (a.enabled) return a.order - b.order
    return 0
  })
}

export default function IconProvidersPanel() {
  const t = useT()
  const { data, error, setData } = useLoaded('icon-providers', () => api.iconProviders(), { refresh: false })
  if (!data) return <p className="hint">{error || t('Loading…')}</p>
  return <IconProvidersForm loaded={data} onSaved={(saved) => setData(() => saved)} />
}

function IconProvidersForm({ loaded, onSaved }: { loaded: IconProvidersResponse; onSaved: (saved: IconProvidersResponse) => void }) {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const [providers, setProviders] = useState(() => sortChain(loaded.providers))
  const [maxTries, setMaxTries] = useState(loaded.max_tries)
  const [busy, setBusy] = useState(false)

  function move(index: number, delta: number) {
    const next = [...providers]
    const target = index + delta
    if (target < 0 || target >= next.length) return
    const [item] = next.splice(index, 1)
    next.splice(target, 0, item)
    setProviders(next)
  }

  function toggle(id: string) {
    setProviders((current) => current.map((item) => (item.id === id ? { ...item, enabled: !item.enabled } : item)))
  }

  async function save() {
    setBusy(true)
    try {
      const enabled = providers.filter((item) => item.enabled).map((item) => item.id)
      if (enabled.length === 0) {
        toast('error', t('Keep at least one icon library enabled'))
        return
      }
      const saved = await api.setIconProviders({
        providers: enabled,
        max_tries: Math.min(maxTries, enabled.length),
      })
      setProviders(sortChain(saved.providers))
      setMaxTries(saved.max_tries)
      onSaved(saved)
      toast('success', t('Icon libraries updated'))
    } catch (error) {
      toast('error', (error as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function fillNow() {
    try {
      const { pending } = await api.fillIcons()
      toast('success', pending ? t('Looking for {count} icons in the background', { count: pending }) : t('Every account has an icon'))
    } catch (error) {
      toast('error', (error as Error).message)
    }
  }

  const enabledCount = providers.filter((item) => item.enabled).length

  return (
    <>
        <table className="table">
          <thead>
            <tr>
              <th>{t('Order')}</th>
              <th>{t('Library')}</th>
              <th>{t('Enabled')}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {providers.map((provider, index) => (
              <tr key={provider.id} className={provider.enabled ? '' : 'muted'}>
                <td className="mono small">{index + 1}</td>
                <td>
                  <strong title={provider.template}>{provider.label}</strong>
                  <div className="hint mono small">{provider.source}</div>
                </td>
                <td>
                  <button
                    className={`toggle${provider.enabled ? ' on' : ''}`}
                    onClick={() => toggle(provider.id)}
                    aria-pressed={provider.enabled}
                    aria-label={t('Enabled')}
                  />
                </td>
                <td>
                  <div className="flex" style={{ justifyContent: 'flex-end' }}>
                    <button className="btn small ghost" onClick={() => move(index, -1)} disabled={index === 0} title={t('Move up')}>
                      ↑
                    </button>
                    <button
                      className="btn small ghost"
                      onClick={() => move(index, 1)}
                      disabled={index === providers.length - 1}
                      title={t('Move down')}
                    >
                      ↓
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>

        <div className="field" style={{ maxWidth: 320, marginTop: 12 }}>
          <label>{t('Libraries to try: {count}', { count: Math.min(maxTries, enabledCount || 1) })}</label>
          <input
            type="range"
            min={1}
            max={Math.max(1, enabledCount)}
            value={Math.min(maxTries, Math.max(1, enabledCount))}
            onChange={(event) => setMaxTries(Number(event.target.value))}
          />
          <p className="hint">
            {t('Only these many libraries are queried; the default icon is used when all of them miss.')}
          </p>
        </div>

        <button className="btn primary small" onClick={save} disabled={busy} style={{ marginTop: 10 }}>
          <Icon name="check" size={14} /> {t('Save libraries')}
        </button>
        <button className="btn ghost small" onClick={() => void fillNow()} style={{ marginTop: 10, marginLeft: 8 }}>
          <Icon name="refresh" size={14} /> {t('Fetch missing icons now')}
        </button>
    </>
  )
}
