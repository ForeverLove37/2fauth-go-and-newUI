export type ThemeBase = 'dark' | 'light'

export interface Palette {
  /** page background gradient stops */
  bg: string
  bg2: string
  /** three radial glows painted over the background */
  glowA: string
  glowB: string
  glowC: string
  /** glass surfaces */
  panel: string
  panelAlpha: number
  panelStrong: string
  /** borders and text */
  border: string
  borderStrong: string
  text: string
  muted: string
  /** brand + status colours */
  accent: string
  accent2: string
  accent3: string
  danger: string
  warn: string
}

export interface SurfaceConfig {
  radius: number
  blur: number
  cardGradient: boolean
}

export interface TypographyConfig {
  sans: string
  mono: string
  /** global size multiplier, 0.85 – 1.2 */
  scale: number
}

export interface MotionConfig {
  enabled: boolean
  duration: number
  autoRotate: boolean
}

export interface ThemeConfig {
  id: string
  name: string
  base: ThemeBase
  palette: Palette
  surface: SurfaceConfig
  typography: TypographyConfig
  motion: MotionConfig
}

export interface ThemeProfile {
  id: string
  name: string
  theme: ThemeConfig
  createdAt: number
}
