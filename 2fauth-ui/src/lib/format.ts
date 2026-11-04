import type { Account } from '../api/types'
import { t } from '../i18n'

/** Seconds left in the current TOTP window. Windows are aligned on the Unix epoch. */
export function remainingSeconds(account: Account, now: number): number {
  if (!account.otp || !account.period) return 0
  return account.period - (Math.floor(now / 1000) % account.period)
}

/** Progress of the current window, between 0 and 1. */
export function otpProgress(account: Account, now: number): number {
  if (!account.period) return 0
  return remainingSeconds(account, now) / account.period
}

/**
 * The code that is valid now. Between two refreshes the window can roll over, and
 * the code loaded for the next window is the right one from that moment on.
 */
export function currentCode(account: Account, now: number): string {
  const otp = account.otp
  if (!otp?.password) return ''
  const { period } = account
  if (period && otp.generated_at && otp.next_password) {
    if (Math.floor(now / 1000 / period) > Math.floor(otp.generated_at / period)) return otp.next_password
  }
  return otp.password
}

/** True when the click that just ended on this element was really the end of selecting text in it. */
export function isSelecting(element: Element): boolean {
  const selection = window.getSelection()
  return Boolean(selection && !selection.isCollapsed && selection.containsNode(element, true))
}

/** What stands for a hidden code. */
export const MASK = '*'

/** A mask as long as the code of this account. */
export function maskedCode(account: Account): string {
  return MASK.repeat(account.otp?.password?.length || account.digits || 6)
}

/**
 * A stable hue per service, so that a wall of cards is not a wall of the same card.
 * The same service keeps its colour wherever it is shown.
 */
export function identityHue(account: Account): number {
  const source = (account.service || account.account || String(account.id)).toLowerCase()
  let hash = 0
  for (let index = 0; index < source.length; index += 1) {
    hash = (hash * 31 + source.charCodeAt(index)) >>> 0
  }
  // the golden ratio spreads similar names over the wheel
  return Math.round((hash * 0.61803398875 * 360) % 360)
}

/** Initials used by the avatar placeholder. */
export function initials(account: Account): string {
  const source = account.service || account.account || '?'
  const words = source.replace(/[^\p{L}\p{N}\s]/gu, ' ').trim().split(/\s+/)
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase()
  return source.slice(0, 2).toUpperCase()
}

/** Human readable label of an account. */
export function accountLabel(account: Account): string {
  return account.service || account.account || t('Account #{id}', { id: account.id })
}

/** Secondary line of an account. */
export function accountSubLabel(account: Account): string {
  if (account.service && account.account) return account.account
  if (account.is_borrowed) {
    return t('Shared by {name}', { name: account.borrowed_by ?? t('another user') })
  }
  return account.otp_type.toUpperCase()
}

/** Copies text, falling back to a temporary textarea on insecure contexts. */
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    if (typeof document.execCommand !== 'function') return false
    const area = document.createElement('textarea')
    area.value = text
    area.style.position = 'fixed'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(area)
    return ok
  }
}

/** Formats a duration in seconds as a short countdown. */
export function formatCountdown(seconds: number): string {
  if (seconds >= 60) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  return `${seconds}s`
}

/** Creates an object URL download. */
export function download(filename: string, content: string | Blob, type = 'application/json') {
  const blob = content instanceof Blob ? content : new Blob([content], { type })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  // Revoking at once can cancel the download in some browsers.
  setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

export function stamp(): string {
  return new Date().toISOString().slice(0, 10)
}
