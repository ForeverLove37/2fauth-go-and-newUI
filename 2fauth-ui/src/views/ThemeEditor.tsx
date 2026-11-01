import { useRef, useState } from 'react'
import Icon from '../components/Icon'
import { download, stamp } from '../lib/format'
import { fontChoices, monoChoices, previewStyle } from '../theme/apply'
import { presets } from '../theme/presets'
import { useThemeStore } from '../theme/store'
import type { BackgroundStyle, ThemeConfig } from '../theme/types'
import { useT } from '../i18n'
import { useStore } from '../store/useStore'

function ColorField({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="color-row">
      <label>{label}</label>
      <input type="color" value={value} onChange={(event) => onChange(event.target.value)} />
      <input
        className="input mono"
        style={{ height: 30, padding: '4px 8px', maxWidth: 110 }}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  )
}

function Slider({
  label,
  value,
  min,
  max,
  step = 1,
  suffix = '',
  onChange,
}: {
  label: string
  value: number
  min: number
  max: number
  step?: number
  suffix?: string
  onChange: (value: number) => void
}) {
  return (
    <div className="color-row">
      <label>{label}</label>
      <input type="range" min={min} max={max} step={step} value={value} onChange={(event) => onChange(Number(event.target.value))} />
      <span className="val">
        {Number.isInteger(value) ? value : value.toFixed(2)}
        {suffix}
      </span>
    </div>
  )
}

function Toggle({ label, value, onChange }: { label: string; value: boolean; onChange: (value: boolean) => void }) {
  return (
    <div className="switch">
      <div className="switch-label">{label}</div>
      <button className={`toggle${value ? ' on' : ''}`} onClick={() => onChange(!value)} aria-pressed={value} />
    </div>
  )
}

function Swatch({ theme, active, onClick }: { theme: ThemeConfig; active: boolean; onClick: () => void }) {
  return (
    <button className={`theme-swatch${active ? ' active' : ''}`} style={previewStyle(theme)} onClick={onClick}>
      <strong style={{ color: theme.palette.text }}>{theme.name}</strong>
      <span style={{ color: theme.palette.muted }}>{theme.base === 'dark' ? '深色' : '浅色'} · {theme.surface.radius}px</span>
      <span className="dots">
        <i style={{ background: theme.palette.accent }} />
        <i style={{ background: theme.palette.accent2 }} />
        <i style={{ background: theme.palette.accent3 }} />
        <i style={{ background: theme.palette.panel, border: `1px solid ${theme.palette.borderStrong}` }} />
      </span>
    </button>
  )
}

const sceneryChoices: { id: BackgroundStyle; label: string }[] = [
  { id: 'grid', label: 'Grid' },
  { id: 'stars', label: 'Starfield' },
  { id: 'nebula', label: 'Nebula' },
  { id: 'waves', label: 'Waves' },
  { id: 'aurora', label: 'Aurora' },
  { id: 'bokeh', label: 'Bokeh' },
  { id: 'minimal', label: 'Minimal' },
]

/** Appearance centre: presets, per-token editing, live preview and named profiles. */
export default function ThemeEditor() {
  const t = useT()
  const theme = useThemeStore((state) => state.theme)
  const profiles = useThemeStore((state) => state.profiles)
  const usePreset = useThemeStore((state) => state.usePreset)
  const patchPalette = useThemeStore((state) => state.patchPalette)
  const patchSurface = useThemeStore((state) => state.patchSurface)
  const patchTypography = useThemeStore((state) => state.patchTypography)
  const patchMotion = useThemeStore((state) => state.patchMotion)
  const patchBackground = useThemeStore((state) => state.patchBackground)
  const saveProfile = useThemeStore((state) => state.saveProfile)
  const applyProfile = useThemeStore((state) => state.applyProfile)
  const renameProfile = useThemeStore((state) => state.renameProfile)
  const deleteProfile = useThemeStore((state) => state.deleteProfile)
  const importTheme = useThemeStore((state) => state.importTheme)
  const exportTheme = useThemeStore((state) => state.exportTheme)
  const reset = useThemeStore((state) => state.reset)
  const toast = useStore((state) => state.toast)

  const [profileName, setProfileName] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  return (
    <div className="stack">
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('Preset themes')}</h2>
            <p>{t('{count} built-in palettes, applied instantly; save your own edits as a profile.', { count: presets.length })}</p>
          </div>
          <div className="right flex">
            <button className="btn small ghost" onClick={reset} title={t('Reset to defaults')}>
              <Icon name="refresh" size={14} /> {t('Defaults')}
            </button>
          </div>
        </div>
        <div className="panel-body">
          <div className="theme-grid">
            {presets.map((preset) => (
              <Swatch key={preset.id} theme={preset} active={theme.id === preset.id} onClick={() => usePreset(preset.id)} />
            ))}
          </div>
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('Background scenery')}</h2>
            <p>{t('The animated scene behind your codes. Every preset ships its own; pick any style here.')}</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="scenery-grid">
            {sceneryChoices.map((item) => (
              <button
                key={item.id}
                className={`scenery-tile${(theme.background?.style ?? 'grid') === item.id ? ' active' : ''}`}
                onClick={() => patchBackground({ style: item.id })}
                aria-pressed={(theme.background?.style ?? 'grid') === item.id}
              >
                <span className="scenery-thumb" data-style={item.id} />
                <span>{t(item.label)}</span>
              </button>
            ))}
          </div>
          <Slider
            label={t('Scenery brightness')}
            value={theme.background?.intensity ?? 1}
            min={0.2}
            max={1.6}
            step={0.05}
            onChange={(value) => patchBackground({ intensity: value })}
          />
          <Slider
            label={t('Scenery speed')}
            value={theme.background?.speed ?? 1}
            min={0}
            max={3}
            step={0.05}
            suffix="×"
            onChange={(value) => patchBackground({ speed: value })}
          />
        </div>
      </div>

      <div className="row" style={{ alignItems: 'flex-start' }}>
        <div className="panel grow" style={{ flex: '2 1 420px' }}>
          <div className="panel-head">
            <div>
              <h2>{t('Background & surfaces')}</h2>
              <p>{t('Page colours, the three glows, glass panels, radius and blur.')}</p>
            </div>
          </div>
          <div className="panel-body">
            <ColorField label={t("Page background")} value={theme.palette.bg} onChange={(value) => patchPalette({ bg: value })} />
            <ColorField label={t("Page background (gradient end)")} value={theme.palette.bg2} onChange={(value) => patchPalette({ bg2: value })} />
            <ColorField label={t("Glow A")} value={theme.palette.glowA} onChange={(value) => patchPalette({ glowA: value })} />
            <ColorField label={t("Glow B")} value={theme.palette.glowB} onChange={(value) => patchPalette({ glowB: value })} />
            <ColorField label={t("Glow C")} value={theme.palette.glowC} onChange={(value) => patchPalette({ glowC: value })} />
            <ColorField label={t("Panel colour")} value={theme.palette.panel} onChange={(value) => patchPalette({ panel: value })} />
            <ColorField label={t("Panel border")} value={theme.palette.border} onChange={(value) => patchPalette({ border: value })} />
            <ColorField label={t("Accent border")} value={theme.palette.borderStrong} onChange={(value) => patchPalette({ borderStrong: value })} />
            <Slider label={t("Panel opacity")} value={theme.palette.panelAlpha} min={0.35} max={1} step={0.01} onChange={(value) => patchPalette({ panelAlpha: value })} />
            <Slider label={t("Corner radius")} value={theme.surface.radius} min={4} max={30} suffix="px" onChange={(value) => patchSurface({ radius: value })} />
            <Slider label={t("Backdrop blur")} value={theme.surface.blur} min={0} max={40} suffix="px" onChange={(value) => patchSurface({ blur: value })} />
            <Toggle label={t("Gradient card surface")} value={theme.surface.cardGradient} onChange={(value) => patchSurface({ cardGradient: value })} />
          </div>
        </div>

        <div className="panel grow" style={{ flex: '2 1 420px' }}>
          <div className="panel-head">
            <div>
              <h2>{t('Text & accents')}</h2>
              <p>{t('Body text, secondary text, the three brand colours and the status colours.')}</p>
            </div>
          </div>
          <div className="panel-body">
            <ColorField label={t("Body text")} value={theme.palette.text} onChange={(value) => patchPalette({ text: value })} />
            <ColorField label={t("Secondary text")} value={theme.palette.muted} onChange={(value) => patchPalette({ muted: value })} />
            <ColorField label={t("Accent")} value={theme.palette.accent} onChange={(value) => patchPalette({ accent: value })} />
            <ColorField label={t("Accent 2")} value={theme.palette.accent2} onChange={(value) => patchPalette({ accent2: value })} />
            <ColorField label={t("Accent 3")} value={theme.palette.accent3} onChange={(value) => patchPalette({ accent3: value })} />
            <ColorField label={t("Danger")} value={theme.palette.danger} onChange={(value) => patchPalette({ danger: value })} />
            <ColorField label={t("Warning")} value={theme.palette.warn} onChange={(value) => patchPalette({ warn: value })} />
            <div className="field" style={{ marginTop: 10 }}>
              <label>{t('Body font')}</label>
              <select className="select" value={theme.typography.sans} onChange={(event) => patchTypography({ sans: event.target.value })}>
                {fontChoices.map((font) => (
                  <option key={font.id} value={font.value}>
                    {font.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label>{t('Code font')}</label>
              <select className="select" value={theme.typography.mono} onChange={(event) => patchTypography({ mono: event.target.value })}>
                {monoChoices.map((font) => (
                  <option key={font.id} value={font.value}>
                    {font.label}
                  </option>
                ))}
              </select>
            </div>
            <Slider label={t("Code size")} value={theme.typography.scale} min={0.8} max={1.35} step={0.01} suffix="×" onChange={(value) => patchTypography({ scale: value })} />
          </div>
        </div>
      </div>

      <div className="row" style={{ alignItems: 'flex-start' }}>
        <div className="panel grow">
          <div className="panel-head">
            <div>
              <h2>{t('Motion')}</h2>
              <p>{t('Transition duration and automatic rotation of the 3D stage.')}</p>
            </div>
          </div>
          <div className="panel-body">
            <Toggle label={t("Enable transitions")} value={theme.motion.enabled} onChange={(value) => patchMotion({ enabled: value })} />
            <Slider label={t("Transition duration")} value={theme.motion.duration} min={0} max={600} step={10} suffix="ms" onChange={(value) => patchMotion({ duration: value })} />
            <Toggle label={t("Auto-rotate the 3D stage")} value={theme.motion.autoRotate} onChange={(value) => patchMotion({ autoRotate: value })} />
          </div>
        </div>

        <div className="panel grow">
          <div className="panel-head">
            <div>
              <h2>{t('Live preview')}</h2>
              <p>{t('The preview uses the values being edited, exactly like the real interface.')}</p>
            </div>
          </div>
          <div className="panel-body">
            <div className="preview-tile" style={{ background: 'var(--app-bg)' }}>
              <div className="flex">
                <span className="chip active">{t('Pinned')}</span>
                <span className="chip">{t('Work')}</span>
                <span className="btn small primary">{t('Primary button')}</span>
                <span className="btn small">{t('Secondary')}</span>
              </div>
              <div className="preview-card">
                <div className="flex">
                  <div className="card-avatar">GH</div>
                  <div>
                    <strong style={{ display: 'block', fontSize: 13 }}>GitHub</strong>
                    <span className="hint">alice@example.com</span>
                  </div>
                </div>
                <span className="code" style={{ color: 'var(--accent)' }}>
                  123 456
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>{t('Profiles')}</h2>
            <p>{t('Save the current look as a named profile, switch back any time, or export it as JSON.')}</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="flex wrap" style={{ marginBottom: 14 }}>
            <input
              className="input"
              style={{ maxWidth: 240 }}
              placeholder={t('Profile name')}
              value={profileName}
              onChange={(event) => setProfileName(event.target.value)}
            />
            <button
              className="btn primary small"
              onClick={() => {
                const profile = saveProfile(profileName || t('Profile {n}', { n: profiles.length + 1 }))
                setProfileName('')
                toast('success', t('Profile "{name}" saved', { name: profile.name }))
              }}
            >
              <Icon name="check" size={14} /> {t('Save current look')}
            </button>
            <button
              className="btn small"
              onClick={() => download(`2fauth-theme-${stamp()}.json`, exportTheme())}
            >
              <Icon name="download" size={14} /> {t('Export JSON')}
            </button>
            <button className="btn small" onClick={() => fileRef.current?.click()}>
              <Icon name="upload" size={14} /> {t('Import JSON')}
            </button>
            <input
              ref={fileRef}
              type="file"
              accept="application/json,.json"
              hidden
              onChange={async (event) => {
                const file = event.target.files?.[0]
                if (!file) return
                const imported = importTheme(await file.text())
                toast(
                  imported ? 'success' : 'error',
                  imported ? t('Profile "{name}" imported', { name: imported.name }) : t('That file is not a valid theme'),
                )
              }}
            />
          </div>

          {profiles.length === 0 ? (
            <p className="hint">{t('No saved profiles yet.')}</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>{t('Name')}</th>
                  <th>{t('Base')}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {profiles.map((profile) => (
                  <tr key={profile.id}>
                    <td>{profile.name}</td>
                    <td className="small muted">
                      {profile.theme.base === 'dark' ? t('Dark') : t('Light')} · {profile.theme.surface.radius}px
                    </td>
                    <td>
                      <div className="flex" style={{ justifyContent: 'flex-end' }}>
                        <button className="btn small ghost" onClick={() => applyProfile(profile.id)}>
                          {t('Apply')}
                        </button>
                        <button
                          className="btn small ghost"
                          onClick={() => {
                            const next = window.prompt(t('Rename profile'), profile.name)
                            if (next) renameProfile(profile.id, next)
                          }}
                        >
                          {t('Rename')}
                        </button>
                        <button className="btn small danger" onClick={() => deleteProfile(profile.id)}>
                          <Icon name="trash" size={13} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          <p className="hint" style={{ marginTop: 14 }}>
            {t('Profiles live in this browser (localStorage). Use "Export JSON" to move them to another device.')}
          </p>
        </div>
      </div>
    </div>
  )
}
