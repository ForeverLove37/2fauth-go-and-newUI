import { useEffect, useRef, useState } from 'react'
import type { Account } from '../api/types'
import { AccountCard } from './AccountCard'

interface OrbitRingProps {
  accounts: Account[]
  selectedId: number | null
  now: number
  dots: boolean
  showNext: boolean
  onSelect: (id: number) => void
  onCopy: (message: string) => void
  onFavorite: (id: number) => void
  onOpen: (id: number) => void
}

/**
 * Accounts arranged on a CSS 3D ring. Dragging spins the ring, releasing snaps
 * to the closest card, the wheel steps through the orbit.
 */
export default function OrbitRing({
  accounts,
  selectedId,
  now,
  dots,
  showNext,
  onSelect,
  onCopy,
  onFavorite,
  onOpen,
}: OrbitRingProps) {
  const count = accounts.length
  const step = count > 1 ? 360 / count : 0
  const radius = count > 1 ? Math.min(660, Math.max(340, 150 + count * 32)) : 0

  const selectedIndex = Math.max(
    0,
    accounts.findIndex((account) => account.id === selectedId),
  )

  const [ry, setRy] = useState(0)
  const [snapping, setSnapping] = useState(false)
  const [dragging, setDragging] = useState(false)
  const drag = useRef({ active: false, startX: 0, startRy: 0, moved: 0 })

  // Keep the selected card at the front of the ring.
  useEffect(() => {
    setSnapping(true)
    setRy(-selectedIndex * step)
    const timer = window.setTimeout(() => setSnapping(false), 560)
    return () => window.clearTimeout(timer)
  }, [selectedIndex, step])

  function onPointerDown(event: React.PointerEvent) {
    if (count === 0) return
    drag.current = { active: true, startX: event.clientX, startRy: ry, moved: 0 }
    setDragging(true)
    setSnapping(false)
    ;(event.target as HTMLElement).setPointerCapture?.(event.pointerId)
  }

  function onPointerMove(event: React.PointerEvent) {
    if (!drag.current.active) return
    const dx = event.clientX - drag.current.startX
    drag.current.moved = Math.abs(dx)
    setRy(drag.current.startRy + dx * 0.32)
  }

  function onPointerUp() {
    if (!drag.current.active) return
    drag.current.active = false
    setDragging(false)
    if (count <= 1) return
    const normalized = ((-ry / step) % count + count) % count
    const snapped = Math.round(normalized) % count
    if (drag.current.moved < 6) {
      setSnapping(true)
      setRy(-selectedIndex * step)
      return
    }
    onSelect(accounts[snapped].id)
  }

  function onWheel(event: React.WheelEvent) {
    if (count <= 1) return
    const direction = event.deltaY > 0 ? 1 : -1
    const next = (selectedIndex + direction + count) % count
    onSelect(accounts[next].id)
  }

  if (count === 0) {
    return (
      <div className="empty">
        <strong>No accounts yet</strong>
        <span>Add your first 2FA account, import an export or scan a QR code.</span>
      </div>
    )
  }

  return (
    <div
      className={`orbit-viewport${dragging ? ' dragging' : ''}`}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onWheel={onWheel}
    >
      <div
        className={`orbit-ring${snapping ? ' snapping' : ''}`}
        style={{ '--ry': `${ry}deg` } as React.CSSProperties}
      >
        {accounts.map((account, index) => (
          <AccountCard
            key={account.id}
            account={account}
            index={index}
            step={step}
            radius={radius}
            selected={account.id === selectedId}
            now={now}
            dots={dots}
            showNext={showNext}
            onSelect={() => onSelect(account.id)}
            onCopy={onCopy}
            onFavorite={() => onFavorite(account.id)}
            onOpen={() => onOpen(account.id)}
          />
        ))}
      </div>
    </div>
  )
}
