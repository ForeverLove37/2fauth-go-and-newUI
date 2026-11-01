import { create } from 'zustand'
import { common } from './dict.common'
import { forms } from './dict.forms'
import { settings } from './dict.settings'

/**
 * Tiny message catalogue.
 *
 * English strings are the message ids, so components stay readable and a
 * missing translation silently falls back to English rather than showing a key.
 * Add a language by adding another dictionary map below.
 */
export const locales = [
  { id: 'auto', label: 'Follow browser / 跟随系统' },
  { id: 'en', label: 'English' },
  { id: 'zh-CN', label: '简体中文' },
  { id: 'zh-TW', label: '繁體中文' },
] as const

export type LocaleId = 'en' | 'zh-CN' | 'zh-TW'

const dictionaries: Record<LocaleId, Record<string, string>> = {
  en: {},
  'zh-CN': { ...common, ...settings, ...forms },
  'zh-TW': {},
}

const STORAGE_KEY = '2fauth.locale.v1'

export function browserLocale(): LocaleId {
  const tag = (typeof navigator !== 'undefined' && navigator.language) || 'en'
  if (/^zh-(TW|HK|Hant)/i.test(tag)) return 'zh-TW'
  if (/^zh/i.test(tag)) return 'zh-CN'
  return 'en'
}

export function resolveLocale(requested: string): LocaleId {
  if (!requested || requested === 'auto' || requested === 'browser') return browserLocale()
  const normalised = requested.replace('_', '-')
  if (normalised in dictionaries) return normalised as LocaleId
  if (/^zh$/i.test(normalised)) return 'zh-CN'
  if (/^zh-(tw|hk|hant)/i.test(normalised)) return 'zh-TW'
  if (/^zh/i.test(normalised)) return 'zh-CN'
  return 'en'
}

export function translate(locale: LocaleId, source: string, vars?: Record<string, string | number>): string {
  const table = dictionaries[locale] ?? {}
  // zh-TW falls back to the simplified catalogue before English
  const template = table[source] ?? (locale === 'zh-TW' ? dictionaries['zh-CN'][source] : undefined) ?? source
  if (!vars) return template
  return template.replace(/\{(\w+)\}/g, (match, key: string) => (key in vars ? String(vars[key]) : match))
}

interface I18nState {
  locale: string
  setLocale: (locale: string) => void
}

export const useI18nStore = create<I18nState>((set) => ({
  locale: (typeof localStorage !== 'undefined' && localStorage.getItem(STORAGE_KEY)) || 'auto',
  setLocale: (locale) => {
    try {
      localStorage.setItem(STORAGE_KEY, locale)
    } catch {
      /* private mode */
    }
    set({ locale })
  },
}))

export type Translate = (source: string, vars?: Record<string, string | number>) => string

/** Returns a `t()` bound to the active locale; re-renders when it changes. */
export function useT(): Translate {
  const locale = useI18nStore((state) => state.locale)
  const resolved = resolveLocale(locale)
  return (source, vars) => translate(resolved, source, vars)
}

/** Non-react helper for one-off strings (toasts, event handlers). */
export function t(source: string, vars?: Record<string, string | number>): string {
  return translate(resolveLocale(useI18nStore.getState().locale), source, vars)
}
