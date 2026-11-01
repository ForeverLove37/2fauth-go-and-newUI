import { create } from 'zustand'
import { applyTheme, parseTheme, serialiseTheme } from './apply'
import { defaultTheme, presetById, presets } from './presets'
import type { ThemeConfig, ThemeProfile } from './types'

const THEME_KEY = '2fauth.theme.v2'
const PROFILES_KEY = '2fauth.theme.profiles.v2'

function loadTheme(): ThemeConfig {
  try {
    const raw = localStorage.getItem(THEME_KEY)
    if (raw) {
      const parsed = parseTheme(raw)
      if (parsed) return parsed
    }
  } catch {
    /* ignore a corrupt entry */
  }
  return defaultTheme
}

function loadProfiles(): ThemeProfile[] {
  try {
    const raw = localStorage.getItem(PROFILES_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as ThemeProfile[]
      if (Array.isArray(parsed)) return parsed.filter((profile) => profile && profile.theme)
    }
  } catch {
    /* ignore a corrupt entry */
  }
  return []
}

function persist(theme: ThemeConfig, profiles: ThemeProfile[]) {
  try {
    localStorage.setItem(THEME_KEY, JSON.stringify(theme))
    localStorage.setItem(PROFILES_KEY, JSON.stringify(profiles))
  } catch {
    /* storage may be full or disabled */
  }
}

interface ThemeState {
  theme: ThemeConfig
  profiles: ThemeProfile[]
  /** preview applies to the document without being persisted */
  previewing: boolean
  setTheme: (theme: ThemeConfig) => void
  previewTheme: (theme: ThemeConfig) => void
  cancelPreview: () => void
  commitPreview: () => void
  usePreset: (id: string) => void
  patch: (patch: Partial<ThemeConfig>) => void
  patchPalette: (patch: Partial<ThemeConfig['palette']>) => void
  patchSurface: (patch: Partial<ThemeConfig['surface']>) => void
  patchTypography: (patch: Partial<ThemeConfig['typography']>) => void
  patchMotion: (patch: Partial<ThemeConfig['motion']>) => void
  patchBackground: (patch: Partial<ThemeConfig['background']>) => void
  saveProfile: (name: string) => ThemeProfile
  applyProfile: (id: string) => void
  renameProfile: (id: string, name: string) => void
  deleteProfile: (id: string) => void
  importTheme: (json: string) => ThemeConfig | null
  exportTheme: () => string
  reset: () => void
}

const initial = loadTheme()
applyTheme(initial)

export const useThemeStore = create<ThemeState>((set, get) => {
  const commit = (theme: ThemeConfig) => {
    applyTheme(theme)
    persist(theme, get().profiles)
    set({ theme, previewing: false })
  }

  return {
    theme: initial,
    profiles: loadProfiles(),
    previewing: false,

    setTheme: commit,

    previewTheme(theme) {
      applyTheme(theme)
      set({ previewing: true, theme })
    },

    cancelPreview() {
      const active = loadTheme()
      applyTheme(active)
      set({ theme: active, previewing: false })
    },

    commitPreview() {
      persist(get().theme, get().profiles)
      set({ previewing: false })
    },

    usePreset(id) {
      const preset = presetById(id)
      if (!preset) return
      commit({ ...preset })
    },

    patch(patch) {
      commit({ ...get().theme, ...patch })
    },

    patchPalette(patch) {
      commit({ ...get().theme, palette: { ...get().theme.palette, ...patch } })
    },

    patchSurface(patch) {
      commit({ ...get().theme, surface: { ...get().theme.surface, ...patch } })
    },

    patchTypography(patch) {
      commit({ ...get().theme, typography: { ...get().theme.typography, ...patch } })
    },

    patchBackground(patch) {
      commit({ ...get().theme, background: { ...get().theme.background, ...patch } })
    },

    patchMotion(patch) {
      commit({ ...get().theme, motion: { ...get().theme.motion, ...patch } })
    },

    saveProfile(name) {
      const profile: ThemeProfile = {
        id: `profile-${Date.now()}-${Math.floor(Math.random() * 1000)}`,
        name: name.trim() || `配置 ${get().profiles.length + 1}`,
        theme: { ...get().theme, name: name.trim() || get().theme.name },
        createdAt: Date.now(),
      }
      const profiles = [...get().profiles, profile]
      persist(get().theme, profiles)
      set({ profiles })
      return profile
    },

    applyProfile(id) {
      const profile = get().profiles.find((item) => item.id === id)
      if (profile) commit({ ...profile.theme })
    },

    renameProfile(id, name) {
      const profiles = get().profiles.map((profile) =>
        profile.id === id ? { ...profile, name: name.trim() || profile.name } : profile,
      )
      persist(get().theme, profiles)
      set({ profiles })
    },

    deleteProfile(id) {
      const profiles = get().profiles.filter((profile) => profile.id !== id)
      persist(get().theme, profiles)
      set({ profiles })
    },

    importTheme(json) {
      const theme = parseTheme(json)
      if (!theme) return null
      commit(theme)
      return theme
    },

    exportTheme() {
      return serialiseTheme(get().theme)
    },

    reset() {
      commit({ ...defaultTheme })
    },
  }
})

export { presets }
