import { useEffect, useState } from 'react'

/** Re-renders the caller every `ms` milliseconds with the current time. */
export function useTick(ms = 250): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), ms)
    return () => window.clearInterval(id)
  }, [ms])
  return now
}

/** Runs `callback` on an interval, pausing while the tab is hidden. */
export function usePolling(callback: () => void, ms: number) {
  useEffect(() => {
    let timer = window.setInterval(() => {
      if (!document.hidden) callback()
    }, ms)
    const onVisible = () => {
      if (!document.hidden) callback()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      window.clearInterval(timer)
      timer = 0
      document.removeEventListener('visibilitychange', onVisible)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ms])
}

/** Closes a modal on Escape. */
export function useEscape(handler: () => void) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') handler()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [handler])
}

/** Applies the browser preference to the document. */
export function useThemeClass(theme: string) {
  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: light)')
    const apply = () => {
      const resolved = theme === 'system' ? (media.matches ? 'light' : 'dark') : theme
      document.documentElement.dataset.theme = resolved === 'light' ? 'light' : 'dark'
    }
    apply()
    if (theme === 'system') media.addEventListener('change', apply)
    return () => media.removeEventListener('change', apply)
  }, [theme])
}

/** Resolves the effective theme (system preference expanded). */
export function useResolvedTheme(theme: string): 'dark' | 'light' {
  const [resolved, setResolved] = useState<'dark' | 'light'>('dark')
  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: light)')
    const apply = () => setResolved(theme === 'system' ? (media.matches ? 'light' : 'dark') : theme === 'light' ? 'light' : 'dark')
    apply()
    if (theme === 'system') media.addEventListener('change', apply)
    return () => media.removeEventListener('change', apply)
  }, [theme])
  return resolved
}
