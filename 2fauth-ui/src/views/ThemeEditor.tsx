import { useRef, useState } from 'react'
import Icon from '../components/Icon'
import { download, stamp } from '../lib/format'
import { fontChoices, monoChoices, previewStyle } from '../theme/apply'
import { presets } from '../theme/presets'
import { useThemeStore } from '../theme/store'
import type { ThemeConfig } from '../theme/types'
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

/** Appearance centre: presets, per-token editing, live preview and named profiles. */
export default function ThemeEditor() {
  const theme = useThemeStore((state) => state.theme)
  const profiles = useThemeStore((state) => state.profiles)
  const usePreset = useThemeStore((state) => state.usePreset)
  const patchPalette = useThemeStore((state) => state.patchPalette)
  const patchSurface = useThemeStore((state) => state.patchSurface)
  const patchTypography = useThemeStore((state) => state.patchTypography)
  const patchMotion = useThemeStore((state) => state.patchMotion)
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
            <h2>预设主题</h2>
            <p>{presets.length} 套内置配色，点击立即生效；改完可另存为自己的配置档案。</p>
          </div>
          <div className="right flex">
            <button className="btn small ghost" onClick={reset} title="恢复默认">
              <Icon name="refresh" size={14} /> 默认
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

      <div className="row" style={{ alignItems: 'flex-start' }}>
        <div className="panel grow" style={{ flex: '2 1 420px' }}>
          <div className="panel-head">
            <div>
              <h2>背景与表面</h2>
              <p>页面底色、三处光晕、玻璃面板与圆角模糊。</p>
            </div>
          </div>
          <div className="panel-body">
            <ColorField label="底色" value={theme.palette.bg} onChange={(value) => patchPalette({ bg: value })} />
            <ColorField label="底色（渐变末端）" value={theme.palette.bg2} onChange={(value) => patchPalette({ bg2: value })} />
            <ColorField label="光晕 A" value={theme.palette.glowA} onChange={(value) => patchPalette({ glowA: value })} />
            <ColorField label="光晕 B" value={theme.palette.glowB} onChange={(value) => patchPalette({ glowB: value })} />
            <ColorField label="光晕 C" value={theme.palette.glowC} onChange={(value) => patchPalette({ glowC: value })} />
            <ColorField label="面板底色" value={theme.palette.panel} onChange={(value) => patchPalette({ panel: value })} />
            <ColorField label="面板描边" value={theme.palette.border} onChange={(value) => patchPalette({ border: value })} />
            <ColorField label="强调描边" value={theme.palette.borderStrong} onChange={(value) => patchPalette({ borderStrong: value })} />
            <Slider label="面板不透明度" value={theme.palette.panelAlpha} min={0.35} max={1} step={0.01} onChange={(value) => patchPalette({ panelAlpha: value })} />
            <Slider label="圆角" value={theme.surface.radius} min={4} max={30} suffix="px" onChange={(value) => patchSurface({ radius: value })} />
            <Slider label="毛玻璃模糊" value={theme.surface.blur} min={0} max={40} suffix="px" onChange={(value) => patchSurface({ blur: value })} />
            <Toggle label="卡片使用渐变表面" value={theme.surface.cardGradient} onChange={(value) => patchSurface({ cardGradient: value })} />
          </div>
        </div>

        <div className="panel grow" style={{ flex: '2 1 420px' }}>
          <div className="panel-head">
            <div>
              <h2>文字与强调色</h2>
              <p>正文、次要文字、品牌三色与状态色。</p>
            </div>
          </div>
          <div className="panel-body">
            <ColorField label="正文" value={theme.palette.text} onChange={(value) => patchPalette({ text: value })} />
            <ColorField label="次要文字" value={theme.palette.muted} onChange={(value) => patchPalette({ muted: value })} />
            <ColorField label="强调色" value={theme.palette.accent} onChange={(value) => patchPalette({ accent: value })} />
            <ColorField label="强调色 2" value={theme.palette.accent2} onChange={(value) => patchPalette({ accent2: value })} />
            <ColorField label="强调色 3" value={theme.palette.accent3} onChange={(value) => patchPalette({ accent3: value })} />
            <ColorField label="危险" value={theme.palette.danger} onChange={(value) => patchPalette({ danger: value })} />
            <ColorField label="警告" value={theme.palette.warn} onChange={(value) => patchPalette({ warn: value })} />
            <div className="field" style={{ marginTop: 10 }}>
              <label>正文字体</label>
              <select className="select" value={theme.typography.sans} onChange={(event) => patchTypography({ sans: event.target.value })}>
                {fontChoices.map((font) => (
                  <option key={font.id} value={font.value}>
                    {font.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label>验证码字体</label>
              <select className="select" value={theme.typography.mono} onChange={(event) => patchTypography({ mono: event.target.value })}>
                {monoChoices.map((font) => (
                  <option key={font.id} value={font.value}>
                    {font.label}
                  </option>
                ))}
              </select>
            </div>
            <Slider label="验证码字号" value={theme.typography.scale} min={0.8} max={1.35} step={0.01} suffix="×" onChange={(value) => patchTypography({ scale: value })} />
          </div>
        </div>
      </div>

      <div className="row" style={{ alignItems: 'flex-start' }}>
        <div className="panel grow">
          <div className="panel-head">
            <div>
              <h2>动效</h2>
              <p>过渡时长与 3D 舞台的自动旋转。</p>
            </div>
          </div>
          <div className="panel-body">
            <Toggle label="启用过渡动效" value={theme.motion.enabled} onChange={(value) => patchMotion({ enabled: value })} />
            <Slider label="过渡时长" value={theme.motion.duration} min={0} max={600} step={10} suffix="ms" onChange={(value) => patchMotion({ duration: value })} />
            <Toggle label="3D 舞台默认自动旋转" value={theme.motion.autoRotate} onChange={(value) => patchMotion({ autoRotate: value })} />
          </div>
        </div>

        <div className="panel grow">
          <div className="panel-head">
            <div>
              <h2>实时预览</h2>
              <p>预览使用当前编辑中的值，和真实界面一致。</p>
            </div>
          </div>
          <div className="panel-body">
            <div className="preview-tile" style={{ background: 'var(--app-bg)' }}>
              <div className="flex">
                <span className="chip active">置顶</span>
                <span className="chip">工作</span>
                <span className="btn small primary">主要按钮</span>
                <span className="btn small">次要</span>
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
            <h2>配置档案</h2>
            <p>保存当前外观为命名档案，随时切回；也可导出 JSON 分享或备份。</p>
          </div>
        </div>
        <div className="panel-body">
          <div className="flex wrap" style={{ marginBottom: 14 }}>
            <input
              className="input"
              style={{ maxWidth: 240 }}
              placeholder="档案名称"
              value={profileName}
              onChange={(event) => setProfileName(event.target.value)}
            />
            <button
              className="btn primary small"
              onClick={() => {
                const profile = saveProfile(profileName || `档案 ${profiles.length + 1}`)
                setProfileName('')
                toast('success', `已保存「${profile.name}」`)
              }}
            >
              <Icon name="check" size={14} /> 保存当前外观
            </button>
            <button
              className="btn small"
              onClick={() => download(`2fauth-theme-${stamp()}.json`, exportTheme())}
            >
              <Icon name="download" size={14} /> 导出 JSON
            </button>
            <button className="btn small" onClick={() => fileRef.current?.click()}>
              <Icon name="upload" size={14} /> 导入 JSON
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
                toast(imported ? 'success' : 'error', imported ? `已导入「${imported.name}」` : '文件不是有效的主题配置')
              }}
            />
          </div>

          {profiles.length === 0 ? (
            <p className="hint">还没有保存的档案。</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>基础</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {profiles.map((profile) => (
                  <tr key={profile.id}>
                    <td>{profile.name}</td>
                    <td className="small muted">{profile.theme.base === 'dark' ? '深色' : '浅色'} · {profile.theme.surface.radius}px</td>
                    <td>
                      <div className="flex" style={{ justifyContent: 'flex-end' }}>
                        <button className="btn small ghost" onClick={() => applyProfile(profile.id)}>
                          应用
                        </button>
                        <button
                          className="btn small ghost"
                          onClick={() => {
                            const next = window.prompt('重命名档案', profile.name)
                            if (next) renameProfile(profile.id, next)
                          }}
                        >
                          重命名
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
            配置保存在本机浏览器（localStorage）。换设备时可用「导出 JSON」带走，或在设置中重新选配色。
          </p>
        </div>
      </div>
    </div>
  )
}
