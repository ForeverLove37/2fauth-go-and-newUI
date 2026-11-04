import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import type { Account } from '../api/types'

interface Props {
  accounts: Account[]
  enabled: boolean
  /** the theme's "enable transitions" switch */
  animate: boolean
  renderCard: (account: Account) => ReactNode
  /** called with the new order once a drag has really changed it */
  onReorder: (ids: number[]) => void
}

const DRAG_THRESHOLD = 6
const MOVE_MS = 240
const DROP_MS = 220
const MOVE_EASING = 'cubic-bezier(0.2, 0.8, 0.2, 1)'
const SCROLL_EDGE = 70
const SCROLL_SPEED = 900 // px per second at the very edge
const MAX_TILT = 5

interface Drag {
  id: number
  pointerId: number
  startX: number
  startY: number
  x: number
  y: number
  grabX: number
  grabY: number
  width: number
  height: number
  active: boolean
  frame: number
  lastTime: number
  lastX: number
  tilt: number
}

function reducedMotion() {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
}

/**
 * The grid of the 2D mode. While dragging, the card itself follows the pointer on a
 * layer of its own and the others make way, each sliding from where it was.
 *
 * The target slot comes from the pointer and the layout positions alone, not from
 * what is under the pointer: cards in mid-flight cannot make the order flip back
 * and forth.
 */
export default function SortableGrid({ accounts, enabled, animate, renderCard, onReorder }: Props) {
  const [order, setOrder] = useState<number[] | null>(null)
  const [floating, setFloating] = useState<{ id: number; width: number; height: number } | null>(null)
  const grid = useRef<HTMLDivElement>(null)
  const overlay = useRef<HTMLDivElement | null>(null)
  const cells = useRef(new Map<number, HTMLDivElement>())
  const positions = useRef(new Map<number, { left: number; top: number }>())
  const drag = useRef<Drag | null>(null)
  const orderRef = useRef<number[] | null>(null)
  const original = useRef<number[]>([])
  const dropping = useRef(false)

  const smooth = () => animate && !reducedMotion()

  const shown = (() => {
    if (!order) return accounts
    const byId = new Map(accounts.map((account) => [account.id, account]))
    const list = order.map((id) => byId.get(id)).filter((account): account is Account => Boolean(account))
    for (const account of accounts) if (!order.includes(account.id)) list.push(account)
    return list
  })()

  // After each layout, cards that changed place slide there from where they were.
  useLayoutEffect(() => {
    const animated = smooth()
    cells.current.forEach((element, id) => {
      const next = { left: element.offsetLeft, top: element.offsetTop }
      const before = positions.current.get(id)
      positions.current.set(id, next)
      if (!before || !animated) return
      if (before.left === next.left && before.top === next.top) return
      // A card already on its way continues from where it is on screen.
      const running = new DOMMatrixReadOnly(getComputedStyle(element).transform)
      const dx = before.left - next.left + running.m41
      const dy = before.top - next.top + running.m42
      element.getAnimations().forEach((animation) => animation.cancel())
      element.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'translate(0, 0)' }], {
        duration: MOVE_MS,
        easing: MOVE_EASING,
      })
    })
    positions.current.forEach((_, id) => {
      if (!cells.current.has(id)) positions.current.delete(id)
    })
  })

  function placeOverlay(current: Drag) {
    const element = overlay.current
    if (!element) return
    element.style.transform = `translate3d(${current.x - current.grabX}px, ${current.y - current.grabY}px, 0) rotate(${current.tilt.toFixed(2)}deg) scale(1.04)`
  }

  // Moves the dragged card to the free slot nearest to the middle of the floating card.
  function updateSlot(current: Drag) {
    const container = grid.current
    const ids = orderRef.current
    if (!container || !ids) return
    const bounds = container.getBoundingClientRect()
    const centerX = current.x - current.grabX + current.width / 2 - bounds.left + container.scrollLeft
    const centerY = current.y - current.grabY + current.height / 2 - bounds.top + container.scrollTop

    let best = -1
    let bestDistance = Infinity
    ids.forEach((id, index) => {
      const cell = cells.current.get(id)
      if (!cell) return
      const dx = cell.offsetLeft + cell.offsetWidth / 2 - centerX
      const dy = cell.offsetTop + cell.offsetHeight / 2 - centerY
      const distance = dx * dx + dy * dy
      if (distance < bestDistance) {
        bestDistance = distance
        best = index
      }
    })
    const from = ids.indexOf(current.id)
    if (best < 0 || from < 0 || best === from) return
    const next = [...ids]
    next.splice(from, 1)
    next.splice(best, 0, current.id)
    orderRef.current = next
    setOrder(next)
  }

  function frame(time: number) {
    const current = drag.current
    if (!current?.active) return
    const dt = Math.min(0.05, (time - current.lastTime) / 1000)
    current.lastTime = time

    const velocity = dt > 0 ? (current.x - current.lastX) / dt : 0
    current.lastX = current.x
    const target = Math.max(-MAX_TILT, Math.min(MAX_TILT, velocity / 220))
    current.tilt += (target - current.tilt) * (1 - Math.exp(-dt * 14))
    placeOverlay(current)

    const container = grid.current
    if (container) {
      const bounds = container.getBoundingClientRect()
      const fromTop = current.y - bounds.top
      const fromBottom = bounds.bottom - current.y
      if (fromTop < SCROLL_EDGE) container.scrollTop -= (SCROLL_SPEED * (1 - Math.max(0, fromTop) / SCROLL_EDGE)) * dt
      else if (fromBottom < SCROLL_EDGE) container.scrollTop += (SCROLL_SPEED * (1 - Math.max(0, fromBottom) / SCROLL_EDGE)) * dt
    }
    updateSlot(current)
    current.frame = requestAnimationFrame(frame)
  }

  function activate(current: Drag) {
    current.active = true
    current.lastTime = performance.now()
    current.lastX = current.x
    original.current = accounts.map((account) => account.id)
    orderRef.current = original.current
    setOrder(original.current)
    setFloating({ id: current.id, width: current.width, height: current.height })
    document.body.classList.add('card-dragging')
    current.frame = requestAnimationFrame(frame)
  }

  function finish(current: Drag, commit: boolean) {
    cancelAnimationFrame(current.frame)
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerCancel)
    window.removeEventListener('keydown', onKeyDown, { capture: true })
    drag.current = null
    if (!current.active) return

    // A drag must not also count as a click on whatever is under the pointer.
    const swallow = (event: Event) => {
      event.stopPropagation()
      event.preventDefault()
    }
    window.addEventListener('click', swallow, { capture: true, once: true })
    window.setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 60)
    document.body.classList.remove('card-dragging')

    const finalOrder = commit ? orderRef.current : original.current
    if (!commit) {
      orderRef.current = original.current
      setOrder(original.current)
    }
    const changed = Boolean(finalOrder && finalOrder.some((id, index) => id !== original.current[index]))

    const settle = () => {
      dropping.current = false
      setFloating(null)
      if (changed && finalOrder) onReorder(finalOrder)
      setOrder(null)
      orderRef.current = null
    }

    const element = overlay.current
    if (!element || !smooth()) {
      settle()
      return
    }
    // Lands on the slot, read once the layout has caught up with the last move.
    dropping.current = true
    requestAnimationFrame(() => {
      const cell = cells.current.get(current.id)
      const rect = cell?.getBoundingClientRect()
      if (!rect) {
        settle()
        return
      }
      const landing = element.animate(
        [
          { transform: element.style.transform },
          { transform: `translate3d(${rect.left}px, ${rect.top}px, 0) rotate(0deg) scale(1)` },
        ],
        { duration: DROP_MS, easing: MOVE_EASING, fill: 'forwards' },
      )
      landing.onfinish = settle
      landing.oncancel = settle
    })
  }

  function onPointerMove(event: PointerEvent) {
    const current = drag.current
    if (!current || event.pointerId !== current.pointerId) return
    current.x = event.clientX
    current.y = event.clientY
    if (!current.active && Math.hypot(current.x - current.startX, current.y - current.startY) >= DRAG_THRESHOLD) {
      activate(current)
    }
  }

  function onPointerUp(event: PointerEvent) {
    const current = drag.current
    if (current && event.pointerId === current.pointerId) finish(current, true)
  }

  function onPointerCancel(event: PointerEvent) {
    const current = drag.current
    if (current && event.pointerId === current.pointerId) finish(current, false)
  }

  function onKeyDown(event: KeyboardEvent) {
    if (event.key === 'Escape' && drag.current) {
      event.stopPropagation()
      finish(drag.current, false)
    }
  }

  function begin(event: React.PointerEvent, account: Account) {
    if (!enabled || drag.current || dropping.current) return
    if (event.pointerType === 'mouse' && event.button !== 0) return
    if ((event.target as Element).closest('.card-actions')) return
    const rect = event.currentTarget.getBoundingClientRect()
    drag.current = {
      id: account.id,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      x: event.clientX,
      y: event.clientY,
      grabX: event.clientX - rect.left,
      grabY: event.clientY - rect.top,
      width: rect.width,
      height: rect.height,
      active: false,
      frame: 0,
      lastTime: 0,
      lastX: event.clientX,
      tilt: 0,
    }
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerCancel)
    window.addEventListener('keydown', onKeyDown, { capture: true })
  }

  const floatingAccount = floating ? accounts.find((account) => account.id === floating.id) : null

  return (
    <div ref={grid} className={`grid2d${enabled ? ' reorderable' : ''}${floating ? ' dragging' : ''}`}>
      {shown.map((account) => (
        <div
          className={`grid2d-cell${floating?.id === account.id ? ' placeholder' : ''}`}
          key={account.id}
          data-account-id={account.id}
          ref={(element) => {
            // Called with null on each render too, so positions must not be forgotten here.
            if (element) cells.current.set(account.id, element)
            else cells.current.delete(account.id)
          }}
          onPointerDown={(event) => begin(event, account)}
        >
          {renderCard(account)}
        </div>
      ))}
      {floating &&
        floatingAccount &&
        createPortal(
          <div
            ref={(element) => {
              overlay.current = element
              if (element && drag.current) placeOverlay(drag.current)
            }}
            className="card-floating"
            style={{ width: floating.width, height: floating.height }}
          >
            {renderCard(floatingAccount)}
          </div>,
          document.body,
        )}
    </div>
  )
}
