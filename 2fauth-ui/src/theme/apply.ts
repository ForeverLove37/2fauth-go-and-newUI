import type { ThemeConfig } from './types'

/** #rrggbb (or #rgb) → [r,g,b] */
function parseHex(value: string): [number, number, number] {
  let hex = value.trim().replace('#', '')
  if (hex.length === 3) hex = hex.split('').map((char) => char + char).join('')
  const int = Number.parseInt(hex.slice(0, 6), 16)
  if (Number.isNaN(int)) return [127, 127, 127]
  return [(int >> 16) & 255, (int >> 8) & 255, int & 255]
}

export function toRgba(value: string, alpha: number): string {
  const [r, g, b] = parseHex(value)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

/** Mix two colours, `amount` between 0 (a) and 1 (b). */
export function mix(a: string, b: string, amount: number): string {
  const [r1, g1, b1] = parseHex(a)
  const [r2, g2, b2] = parseHex(b)
  const blend = (x: number, y: number) => Math.round(x + (y - x) * amount)
  const toHex = (n: number) => n.toString(16).padStart(2, '0')
  return `#${toHex(blend(r1, r2))}${toHex(blend(g1, g2))}${toHex(blend(b1, b2))}`
}

export function lighten(color: string, amount: number): string {
  return mix(color, '#ffffff', amount)
}

export function darken(color: string, amount: number): string {
  return mix(color, '#000000', amount)
}

/** Relative luminance, used to pick readable text on top of a colour. */
export function luminance(color: string): number {
  const [r, g, b] = parseHex(color).map((channel) => {
    const c = channel / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

export function readableOn(background: string, light = '#ffffff', dark = '#04121a'): string {
  return luminance(background) > 0.55 ? dark : light
}

/** The full set of CSS custom properties a theme produces. */
export function themeToVars(theme: ThemeConfig): Record<string, string> {
  const { palette, surface, typography, motion, base } = theme
  const dark = base === 'dark'

  const panel = toRgba(palette.panel, palette.panelAlpha)
  const panelStrong = toRgba(palette.panelStrong, dark ? 0.92 : 0.96)
  const cardTop = toRgba(
    surface.cardGradient ? lighten(palette.panel, dark ? 0.12 : 0.0) : palette.panel,
    Math.min(1, palette.panelAlpha + 0.2),
  )
  const cardBottom = toRgba(
    surface.cardGradient ? darken(palette.panel, dark ? 0.45 : 0.04) : palette.panel,
    Math.min(1, palette.panelAlpha + 0.16),
  )

  const appBackground = [
    `radial-gradient(900px 620px at 12% -10%, ${toRgba(palette.glowA, dark ? 0.22 : 0.3)}, transparent 62%)`,
    `radial-gradient(760px 520px at 88% 8%, ${toRgba(palette.glowB, dark ? 0.18 : 0.26)}, transparent 60%)`,
    `radial-gradient(900px 700px at 50% 110%, ${toRgba(palette.glowC, dark ? 0.16 : 0.24)}, transparent 65%)`,
    `linear-gradient(160deg, ${palette.bg}, ${palette.bg2})`,
  ].join(', ')

  return {
    '--bg': palette.bg,
    '--bg-2': palette.bg2,
    '--app-bg': appBackground,
    '--panel': panel,
    '--panel-strong': panelStrong,
    '--card-grad': `linear-gradient(150deg, ${cardTop}, ${cardBottom})`,
    '--border': toRgba(palette.border, dark ? 0.18 : 0.2),
    '--border-strong': toRgba(palette.borderStrong, dark ? 0.42 : 0.45),
    '--text': palette.text,
    '--muted': palette.muted,
    '--accent': palette.accent,
    '--accent-2': palette.accent2,
    '--accent-3': palette.accent3,
    '--accent-contrast': readableOn(palette.accent),
    '--danger': palette.danger,
    '--warn': palette.warn,
    '--glow-a': palette.glowA,
    '--glow-b': palette.glowB,
    '--glow-c': palette.glowC,
    '--radius': `${surface.radius}px`,
    '--radius-sm': `${Math.max(6, Math.round(surface.radius * 0.62))}px`,
    '--blur': `${surface.blur}px`,
    '--shadow': dark ? '0 24px 60px rgba(3, 6, 14, 0.6)' : '0 20px 48px rgba(40, 70, 130, 0.16)',
    '--font': typography.sans,
    '--mono': typography.mono,
    '--otp-size': `${Math.round(27 * typography.scale)}px`,
    '--otp-size-lg': `${Math.round(30 * typography.scale)}px`,
    '--motion': `${motion.duration}ms`,
    '--bg-intensity': String(theme.background?.intensity ?? 1),
    '--bg-speed': `${(theme.background?.speed ?? 1).toFixed(2)}`,
    '--otp-glow': `${toRgba(palette.accent, 0.42)}`,
    '--otp-glow-soft': `${toRgba(palette.accent2, 0.32)}`,
  }
}

/** Applies a theme to a DOM root (defaults to `<html>`). */
export function applyTheme(theme: ThemeConfig, root: HTMLElement = document.documentElement) {
  root.dataset.theme = theme.base
  root.dataset.motion = theme.motion.enabled ? 'on' : 'off'
  root.dataset.bg = theme.background?.style ?? 'grid'
  for (const [name, value] of Object.entries(themeToVars(theme))) {
    root.style.setProperty(name, value)
  }
}

/** Inline style used by the preview tile of the theme editor. */
export function previewStyle(theme: ThemeConfig): React.CSSProperties {
  const vars = themeToVars(theme)
  return {
    background: vars['--app-bg'],
    borderRadius: vars['--radius'],
    border: `1px solid ${vars['--border-strong']}`,
  } as React.CSSProperties
}

/** Serialises the parts of a theme that are safe to share, for export/import. */
export function serialiseTheme(theme: ThemeConfig): string {
  return JSON.stringify(theme, null, 2)
}

/** Parses an exported theme, rejecting anything that is not a theme object. */
export function parseTheme(text: string): ThemeConfig | null {
  try {
    const value = JSON.parse(text) as Partial<ThemeConfig>
    if (!value || typeof value !== 'object' || !value.palette || !value.name) return null
    const base = value.base === 'light' ? 'light' : 'dark'
    return {
      id: typeof value.id === 'string' && value.id ? value.id : `imported-${Date.now()}`,
      name: String(value.name).slice(0, 40),
      base,
      palette: { ...presetFallbackPalette, ...value.palette },
      surface: { radius: 18, blur: 22, cardGradient: true, ...(value.surface ?? {}) },
      typography: { sans: defaultFonts.sans, mono: defaultFonts.mono, scale: 1, ...(value.typography ?? {}) },
      motion: { enabled: true, duration: 180, autoRotate: false, ...(value.motion ?? {}) },
      background: { style: 'grid', intensity: 1, speed: 1, ...(value.background ?? {}) },
    }
  } catch {
    return null
  }
}

const defaultFonts = {
  sans: "'Inter', 'Segoe UI', system-ui, -apple-system, 'Helvetica Neue', sans-serif",
  mono: "'JetBrains Mono', 'SFMono-Regular', ui-monospace, 'Cascadia Mono', Menlo, monospace",
}

const presetFallbackPalette: ThemeConfig['palette'] = {
  bg: '#05070f',
  bg2: '#080d18',
  glowA: '#6ea8ff',
  glowB: '#b07cff',
  glowC: '#55e6c1',
  panel: '#101727',
  panelAlpha: 0.72,
  panelStrong: '#0b111e',
  border: '#78a0ff',
  borderStrong: '#9ec0ff',
  text: '#e8eefc',
  muted: '#8f9dbb',
  accent: '#55e6c1',
  accent2: '#6ea8ff',
  accent3: '#b07cff',
  danger: '#ff6b81',
  warn: '#ffc46b',
}

export const fontChoices = [
  { id: 'inter', label: 'Inter / 系统无衬线', value: defaultFonts.sans },
  { id: 'grotesk', label: 'Space Grotesk', value: "'Space Grotesk', 'Segoe UI', sans-serif" },
  { id: 'serif', label: '编辑衬线', value: "'Source Serif 4', 'Georgia', serif" },
  { id: 'rounded', label: '圆润无衬线', value: "'Nunito', 'Segoe UI', sans-serif" },
  { id: 'condensed', label: '窄体标题', value: "'Barlow Condensed', 'Arial Narrow', sans-serif" },
]

export const monoChoices = [
  { id: 'jetbrains', label: 'JetBrains Mono', value: defaultFonts.mono },
  { id: 'ibm', label: 'IBM Plex Mono', value: "'IBM Plex Mono', ui-monospace, monospace" },
  { id: 'fira', label: 'Fira Code', value: "'Fira Code', ui-monospace, monospace" },
  { id: 'system-mono', label: '系统等宽', value: "ui-monospace, 'SFMono-Regular', Menlo, monospace" },
]
