import { memo, useMemo } from 'react'
import type { Account } from '../api/types'
import { api } from '../api/client'
import { useT } from '../i18n'
import {
  accountLabel,
  accountSubLabel,
  copyText,
  currentCode,
  identityHue,
  initials,
  maskedCode,
  otpProgress,
  remainingSeconds,
} from '../lib/format'
import { useSecond, useTimedReveal } from '../lib/hooks'
import { wasDragged } from '../stage/Stage3D'
import Icon from './Icon'

interface CardProps {
  account: Account
  selected: boolean
  dots: boolean
  showNext: boolean
  showIcon?: boolean
  compact?: boolean
  onSelect: (id: number) => void
  onCopy: (message: string) => void
  onFavorite: (id: number) => void
  onOpen: (account: Account) => void
}

/** Account picture: its icon, or its initials on the colour of the service. */
export function CardAvatar({ account, showIcon = true, size }: { account: Account; showIcon?: boolean; size?: number }) {
  const style = size ? { width: size, height: size } : undefined
  return (
    <div className={`card-avatar${account.icon && showIcon ? ' has-icon' : ''}`} style={style}>
      {account.icon && showIcon ? <img src={api.iconUrl(account.icon)} alt="" loading="lazy" draggable={false} /> : initials(account)}
    </div>
  )
}

/** The code as it should read this second. Only mounted while a code is visible. */
export function LiveCode({ account, next = false }: { account: Account; next?: boolean }) {
  const now = useSecond()
  if (next) return <>{account.otp?.next_password ?? ''}</>
  return <>{currentCode(account, now) || maskedCode(account)}</>
}

/** Countdown ring of a TOTP account. It re-renders alone, not with the whole card. */
function CountdownRing({ account }: { account: Account }) {
  const now = useSecond()
  return (
    <div className="ring" style={{ '--progress': 1 - otpProgress(account, now) } as React.CSSProperties}>
      <b>{remainingSeconds(account, now)}</b>
    </div>
  )
}

/** One account rendered as a glass card. Its position is owned by the parent (3D stage or grid). */
export const AccountCard = memo(function AccountCard({
  account,
  selected,
  dots,
  showNext,
  showIcon = true,
  compact = false,
  onSelect,
  onCopy,
  onFavorite,
  onOpen,
}: CardProps) {
  const t = useT()
  const { revealed, reveal, handlers } = useTimedReveal()
  const isHotp = account.otp_type === 'hotp'
  const hidden = dots && !revealed

  const flags = useMemo(() => {
    const list: { text: string; tone?: 'hot' | 'warn' }[] = []
    if (account.is_favorite) list.push({ text: t('Pinned'), tone: 'hot' })
    if (account.is_borrowed) list.push({ text: t('Borrowed · {name}', { name: account.borrowed_by ?? '' }).trim() })
    else if (account.is_shared_with_all) list.push({ text: t('Everyone') })
    else if (account.is_shared) list.push({ text: t('Shared') })
    if (isHotp) list.push({ text: `#${account.counter ?? 0}`, tone: 'warn' })
    return list
  }, [account, isHotp, t])

  async function copy(event: React.MouseEvent) {
    event.stopPropagation()
    const code = currentCode(account, Date.now())
    if (!code) return
    const ok = await copyText(code)
    onCopy(ok ? t('Code copied') : t('Copy failed'))
  }

  function select(event: React.MouseEvent) {
    if (wasDragged(event.currentTarget)) return
    onSelect(account.id)
  }

  return (
    <article
      className={`orbit-card${selected ? ' selected' : ''}${compact ? ' compact' : ''}`}
      style={{ '--card-hue': identityHue(account) } as React.CSSProperties}
      onClick={select}
      onDoubleClick={(event) => {
        event.stopPropagation()
        if (wasDragged(event.currentTarget)) return
        onOpen(account)
      }}
      tabIndex={0}
      role="button"
      aria-label={`${accountLabel(account)} — ${hidden ? t('Code hidden') : currentCode(account, Date.now())}`}
      onKeyDown={(event) => {
        if (event.key === 'Enter') onSelect(account.id)
      }}
    >
      <div className="card-top">
        <CardAvatar account={account} showIcon={showIcon} />
        <div className="card-title">
          <strong>{accountLabel(account)}</strong>
          <span>{accountSubLabel(account)}</span>
        </div>
      </div>

      {flags.length > 0 && (
        <div className="card-flags">
          {flags.map((flag) => (
            <span key={flag.text} className={`flag${flag.tone ? ` ${flag.tone}` : ''}`}>
              {flag.text}
            </span>
          ))}
        </div>
      )}

      <div className="card-otp">
        <button className={`otp-digits${hidden ? ' dots' : ''}`} onClick={copy} title={t('Copy code')} {...handlers}>
          {hidden ? maskedCode(account) : <LiveCode account={account} />}
        </button>
        {!isHotp && (
          <div className="otp-meta">
            <CountdownRing account={account} />
          </div>
        )}
      </div>

      {showNext && !isHotp && account.otp?.next_password && !compact && (
        <div className="card-next">
          {t('Next')} <span className="mono">{hidden ? maskedCode(account) : account.otp.next_password}</span>
        </div>
      )}

      <div className="card-actions">
        <button
          className={`icon-btn${account.is_favorite ? ' on' : ''}`}
          title={t('Pin')}
          onClick={(event) => {
            event.stopPropagation()
            onFavorite(account.id)
          }}
        >
          <Icon name={account.is_favorite ? 'star-filled' : 'star'} size={14} />
        </button>
        {dots && (
          <button
            className="icon-btn"
            title={t('Show for 5 seconds')}
            onClick={(event) => {
              event.stopPropagation()
              reveal()
            }}
          >
            <Icon name="eye" size={14} />
          </button>
        )}
        <button
          className="icon-btn"
          title={t('Details')}
          onClick={(event) => {
            event.stopPropagation()
            onOpen(account)
          }}
        >
          <Icon name="sliders" size={14} />
        </button>
      </div>
    </article>
  )
})
