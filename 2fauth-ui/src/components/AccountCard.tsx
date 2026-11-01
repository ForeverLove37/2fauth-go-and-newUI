import { useMemo, useState } from 'react'
import type { Account } from '../api/types'
import { api } from '../api/client'
import { useT } from '../i18n'
import {
  accountLabel,
  accountSubLabel,
  copyText,
  displayCode,
  initials,
  otpProgress,
  remainingSeconds,
} from '../lib/format'
import { wasDragged } from '../stage/Stage3D'
import Icon from './Icon'

interface CardProps {
  account: Account
  selected: boolean
  now: number
  dots: boolean
  showNext: boolean
  compact?: boolean
  onSelect: () => void
  onCopy: (message: string) => void
  onFavorite: () => void
  onOpen: () => void
}

/** One account rendered as a glass card. Its position is owned by the parent (3D stage or grid). */
export function AccountCard({
  account,
  selected,
  now,
  dots,
  showNext,
  compact = false,
  onSelect,
  onCopy,
  onFavorite,
  onOpen,
}: CardProps) {
  const [revealed, setRevealed] = useState(false)
  const t = useT()
  const code = account.otp?.password ?? ''
  const remaining = remainingSeconds(account, now)
  const progress = otpProgress(account, now)
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
    if (!code) return
    const ok = await copyText(code)
    onCopy(ok ? t('Code copied') : t('Copy failed'))
  }

  function select(event: React.MouseEvent) {
    if (wasDragged(event.currentTarget)) return
    onSelect()
  }

  return (
    <article
      className={`orbit-card${selected ? ' selected' : ' dim'}${compact ? ' compact' : ''}`}
      onClick={select}
      onDoubleClick={(event) => {
        if (wasDragged(event.currentTarget)) return
        onOpen()
      }}
      tabIndex={0}
      role="button"
      aria-label={`${accountLabel(account)} — ${hidden ? t('Code hidden') : code}`}
      onKeyDown={(event) => {
        if (event.key === 'Enter') onSelect()
      }}
    >
      <div className="card-top">
        <div className="card-avatar">
          {account.icon ? <img src={api.iconUrl(account.icon)} alt="" /> : initials(account)}
        </div>
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
        <button className={`otp-digits${hidden ? ' dots' : ''}`} onClick={copy} title={t('Copy code')}>
          {displayCode(account, dots, revealed)}
        </button>
        {!isHotp && (
          <div className="otp-meta">
            <div className="ring" style={{ '--progress': 1 - progress } as React.CSSProperties}>
              <b>{remaining}</b>
            </div>
          </div>
        )}
      </div>

      {showNext && !isHotp && account.otp?.next_password && !compact && (
        <div className="card-next">
          {t('Next')} <span className="mono">{hidden ? '••••••' : account.otp.next_password}</span>
        </div>
      )}

      <div className="card-actions">
        <button
          className={`icon-btn${account.is_favorite ? ' on' : ''}`}
          title={t('Pin')}
          onClick={(event) => {
            event.stopPropagation()
            onFavorite()
          }}
        >
          <Icon name={account.is_favorite ? 'star-filled' : 'star'} size={14} />
        </button>
        {dots && (
          <button
            className="icon-btn"
            title={revealed ? t('Hide') : t('Show')}
            onClick={(event) => {
              event.stopPropagation()
              setRevealed((value) => !value)
            }}
          >
            <Icon name={revealed ? 'eye-off' : 'eye'} size={14} />
          </button>
        )}
        <button
          className="icon-btn"
          title={t('Details')}
          onClick={(event) => {
            event.stopPropagation()
            onOpen()
          }}
        >
          <Icon name="sliders" size={14} />
        </button>
      </div>
    </article>
  )
}
