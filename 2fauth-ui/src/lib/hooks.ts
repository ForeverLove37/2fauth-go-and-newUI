import { useEffect, useRef, useState, useSyncExternalStore } from 'react'

const secondListeners = new Set<() => void>()
let secondTimer = 0
let currentSecond = Math.floor(Date.now() / 1000)

function scheduleSecond() {
  secondTimer = window.setTimeout(() => {
    currentSecond = Math.floor(Date.now() / 1000)
    secondListeners.forEach((listener) => listener())
    if (secondListeners.size > 0) scheduleSecond()
    else secondTimer = 0
  }, 1000 - (Date.now() % 1000) + 4)
}

function subscribeSecond(listener: () => void) {
  secondListeners.add(listener)
  if (!secondTimer) {
    currentSecond = Math.floor(Date.now() / 1000)
    scheduleSecond()
  }
  return () => {
    secondListeners.delete(listener)
  }
}

/**
 * The current time, updated once on each second boundary. One timer serves every
 * caller, so a hundred countdowns tick together and cost one wake-up.
 */
export function useSecond(): number {
  return useSyncExternalStore(subscribeSecond, () => currentSecond) * 1000
}

/** How long a code stays visible once revealed. */
export const REVEAL_MS = 5000

/**
 * Shows a code when the mouse moves onto it and hides it again after a fixed time,
 * even if the mouse stays there: moving away and back shows it again. Only a real
 * movement counts, so a 3D card passing under a resting cursor reveals nothing.
 */
export function useTimedReveal(ms = REVEAL_MS) {
  const [revealed, setRevealed] = useState(false)
  const timer = useRef(0)
  const armed = useRef(true)

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const reveal = () => {
    setRevealed(true)
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setRevealed(false), ms)
  }

  const handlers = {
    onPointerMove(event: React.PointerEvent) {
      if (!armed.current || event.pointerType !== 'mouse' || event.buttons !== 0) return
      if (event.movementX === 0 && event.movementY === 0) return
      armed.current = false
      reveal()
    },
    onPointerLeave() {
      armed.current = true
    },
    onFocus(event: React.FocusEvent) {
      if (event.target.matches(':focus-visible')) reveal()
    },
  }

  return { revealed, reveal, handlers }
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
