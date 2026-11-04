import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useT } from '../i18n'
import Icon, { type IconName } from './Icon'

export interface MenuItem {
  label: string
  icon?: IconName
  danger?: boolean
  onClick: () => void
}

const ITEM_HEIGHT = 36

/** The less common actions of a row, behind one button. */
export default function Menu({ items, label }: { items: MenuItem[]; label?: string }) {
  const t = useT()
  const [at, setAt] = useState<{ top: number; right: number } | null>(null)
  const button = useRef<HTMLButtonElement>(null)
  const popup = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!at) return
    popup.current?.querySelector('button')?.focus()
    const close = () => setAt(null)
    const onPointer = (event: PointerEvent) => {
      if (!popup.current?.contains(event.target as Node)) close()
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      close()
      button.current?.focus()
    }
    window.addEventListener('pointerdown', onPointer)
    window.addEventListener('keydown', onKey)
    window.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
    return () => {
      window.removeEventListener('pointerdown', onPointer)
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('resize', close)
    }
  }, [at])

  function toggle() {
    if (at || !button.current) {
      setAt(null)
      return
    }
    const rect = button.current.getBoundingClientRect()
    const height = items.length * ITEM_HEIGHT + 12
    const top = rect.bottom + height + 4 > window.innerHeight ? rect.top - height - 4 : rect.bottom + 4
    setAt({ top, right: window.innerWidth - rect.right })
  }

  return (
    <>
      <button
        ref={button}
        type="button"
        className="btn small ghost"
        aria-haspopup="menu"
        aria-expanded={Boolean(at)}
        title={label ?? t('More actions')}
        aria-label={label ?? t('More actions')}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={toggle}
      >
        {label ?? <Icon name="more" size={14} />}
      </button>
      {at &&
        createPortal(
          <div ref={popup} className="menu-pop" role="menu" style={at}>
            {items.map((item) => (
              <button
                key={item.label}
                type="button"
                role="menuitem"
                className={item.danger ? 'danger' : ''}
                onClick={() => {
                  setAt(null)
                  item.onClick()
                }}
              >
                {item.icon && <Icon name={item.icon} size={14} />} {item.label}
              </button>
            ))}
          </div>,
          document.body,
        )}
    </>
  )
}
