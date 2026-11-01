import { useMemo, useState } from 'react'
import type { Account } from '../api/types'
import { api } from '../api/client'
import {
  accountLabel,
  accountSubLabel,
  copyText,
  displayCode,
  initials,
  otpProgress,
  remainingSeconds,
} from '../lib/format'
import Icon from './Icon'

interface CardProps {
  account: Account
  index: number
  step: number
  radius: number
  selected: boolean
  now: number
  dots: boolean
  showNext: boolean
  onSelect: () => void
  onCopy: (message: string) => void
  onFavorite: () => void
  onOpen: () => void
}

/** One account rendered as a glass card floating on the orbit. */
export function AccountCard({
  account,
  index,
  step,
  radius,
  selected,
  now,
  dots,
  showNext,
  onSelect,
  onCopy,
  onFavorite,
  onOpen,
}: CardProps) {
  const [revealed, setRevealed] = useState(false)
  const code = account.otp?.password ?? ''
  const remaining = remainingSeconds(account, now)
  const progress = otpProgress(account, now)
  const isHotp = account.otp_type === 'hotp'

  const style = useMemo(
    () =>
      ({
        '--i': index,
        '--step': `${step}deg`,
        '--radius': `${radius}px`,
      }) as React.CSSProperties,
    [index, step, radius],
  )

  async function copy() {
    if (!code) return
    const ok = await copyText(code)
    onCopy(ok ? 'Code copied' : 'Copy failed')
  }

  return (
    <article
      className={`orbit-card${selected ? ' selected' : ' dim'}`}
      style={style}
      onClick={onSelect}
      onDoubleClick={onOpen}
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

      <div className="card-flags">
        {account.is_favorite && <span className="flag hot">pinned</span>}
        {account.is_borrowed && <span className="flag">shared · {account.borrowed_by}</span>}
        {account.is_shared_with_all && <span className="flag">everyone</span>}
        {!account.is_borrowed && account.is_shared && !account.is_shared_with_all && (
          <span className="flag">shared</span>
        )}
        {isHotp && <span className="flag warn">counter {account.counter ?? 0}</span>}
      </div>

      <div className="card-actions">
        <button
          className={`icon-btn${account.is_favorite ? ' on' : ''}`}
          title="Pin"
          onClick={(event) => {
            event.stopPropagation()
            onFavorite()
          }}
        >
          <Icon name={account.is_favorite ? 'star-filled' : 'star'} size={14} />
        </button>
        <button
          className="icon-btn"
          title="Details"
          onClick={(event) => {
            event.stopPropagation()
            onOpen()
          }}
        >
          <Icon name="sliders" size={14} />
        </button>
        {dots && (
          <button
            className="icon-btn"
            title="Reveal"
            onClick={(event) => {
              event.stopPropagation()
              setRevealed((value) => !value)
            }}
          >
            <Icon name={revealed ? 'eye-off' : 'eye'} size={14} />
          </button>
        )}
      </div>

      <div className="card-otp">
        <button
          className={`otp-digits${dots && !revealed ? ' dots' : ''}`}
          onClick={(event) => {
            event.stopPropagation()
            void copy()
          }}
          title="Copy the code"
        >
          {displayCode(account, dots, revealed)}
        </button>
        {!isHotp && (
          <div className="otp-meta">
            <div className="ring" style={{ '--progress': 1 - progress } as React.CSSProperties}>
              <b>{remaining}</b>
            </div>
            {showNext && account.otp?.next_password && dotNext(account.otp.next_password, dots && !revealed)}
          </div>
        )}
      </div>
    </article>
  )
}

function dotNext(next: string, hidden: boolean) {
  return <span className="mono" style={{ opacity: 0.55 }}>next {hidden ? '••••••' : next}</span>
}
