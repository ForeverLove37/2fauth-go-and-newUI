import type { Account } from '../api/types'

/** Seconds left in the current TOTP window. */
export function remainingSeconds(account: Account, now: number): number {
  const otp = account.otp
  if (!otp || !account.period) return 0
  const generatedAt = otp.generated_at ?? Math.floor(now / 1000)
  const elapsed = Math.floor(now / 1000) - generatedAt
  return Math.max(0, account.period - (elapsed % account.period))
}

/** Progress of the current window, between 0 and 1. */
export function otpProgress(account: Account, now: number): number {
  if (!account.period) return 0
  return remainingSeconds(account, now) / account.period
}

/** The code to display, masked when the preference asks for it. */
export function displayCode(account: Account, dots: boolean, revealed: boolean): string {
  const code = account.otp?.password ?? '•'.repeat(account.digits || 6)
  if (!dots || revealed) return code
  return '•'.repeat(code.length)
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
  return account.service || account.account || `Account #${account.id}`
}

/** Secondary line of an account. */
export function accountSubLabel(account: Account): string {
  if (account.service && account.account) return account.account
  if (account.is_borrowed) return `shared by ${account.borrowed_by ?? 'another user'}`
  return account.otp_type.toUpperCase()
}

/** Copies text, falling back to a temporary textarea on insecure contexts. */
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
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
  anchor.click()
  URL.revokeObjectURL(url)
}

export function stamp(): string {
  return new Date().toISOString().slice(0, 10)
}
