import { forwardRef, useEffect, useImperativeHandle, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import type { Account, Group } from '../api/types'
import {
  extent,
  gridMetrics,
  loadSpots,
  packInOrder,
  packInRows,
  packNearest,
  placeMissing,
  readingOrder,
  saveSpots,
  type Spots,
} from '../lib/arrange'

export type ArrangeKind = 'group' | 'name' | 'nearest'

export interface FreeGridHandle {
  arrange: (kind: ArrangeKind) => void
}

interface Props {
  accounts: Account[]
  groups: Group[]
  /** false while a filter is on: the matches are shown one after the other and stay put */
  enabled: boolean
  /** the theme's "enable transitions" switch */
  animate: boolean
  /** where the positions are remembered, one set per user */
  storageKey: string
  renderCard: (account: Account) => ReactNode
  /** called with the order the cards now read in, after they have been tidied */
  onArranged: (ids: number[]) => void
}

const DRAG_THRESHOLD = 6
const SCROLL_EDGE = 70
const SCROLL_SPEED = 900 // px per second at the very edge
const MAX_TILT = 5
/** What a press may start on without moving the card: its text is for selecting. */
const NOT_A_HANDLE = '.card-text, .card-actions, button, a, input, select, textarea'

interface Drag {
  id: number
  pointerId: number
  element: HTMLElement
  startX: number
  startY: number
  clientX: number
  clientY: number
  grabX: number
  grabY: number
  x: number
  y: number
  active: boolean
  frame: number
  lastTime: number
  lastClientX: number
  tilt: number
  /** the listeners belong to the render that started the drag, and so does this */
  cancel: () => void
}

function byName(a: Account, b: Account): number {
  const collate = (x: string | null | undefined, y: string | null | undefined) =>
    (x ?? '').localeCompare(y ?? '', undefined, { numeric: true, sensitivity: 'base' })
  return collate(a.service, b.service) || collate(a.account, b.account) || a.id - b.id
}

function restTransform(x: number, y: number): string {
  return `translate(${x}px, ${y}px)`
}

/**
 * The 2D mode, like the icon view of a folder: a card stays wherever it is dropped,
 * on top of the others if need be, and nothing else moves until the user asks for
 * the cards to be tidied.
 */
const FreeGrid = forwardRef<FreeGridHandle, Props>(function FreeGrid(
  { accounts, groups, enabled, animate, storageKey, renderCard, onArranged },
  ref,
) {
  const [placed, setPlaced] = useState<Spots>(() => loadSpots(storageKey))
  const [width, setWidth] = useState(0)
  const [draggingId, setDraggingId] = useState<number | null>(null)
  const [grown, setGrown] = useState({ width: 0, height: 0 })
  const grid = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLDivElement>(null)
  const drag = useRef<Drag | null>(null)

  const metrics = useMemo(() => gridMetrics(width), [width])
  const ids = useMemo(() => accounts.map((account) => account.id), [accounts])
  const spots = useMemo(
    () => (enabled ? placeMissing(ids, placed, metrics) : packInOrder(ids, metrics)),
    [enabled, ids, placed, metrics],
  )
  const area = extent(spots, metrics)
  const latest = useRef({ spots, area })
  latest.current = { spots, area }

  useEffect(() => saveSpots(storageKey, placed), [storageKey, placed])

  useLayoutEffect(() => {
    const element = grid.current
    if (!element) return
    const measure = () => setWidth(element.clientWidth || 1200)
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  useImperativeHandle(ref, () => ({
    arrange(kind) {
      let next: Spots
      if (kind === 'nearest') {
        next = packNearest(ids, spots, metrics)
      } else if (kind === 'name') {
        next = packInOrder([...accounts].sort(byName).map((account) => account.id), metrics)
      } else {
        const known = new Set(groups.map((group) => group.id))
        const sorted = [...accounts].sort(byName)
        const lists = groups.map((group) => sorted.filter((account) => account.group_id === group.id))
        lists.push(sorted.filter((account) => account.group_id === null || !known.has(account.group_id)))
        next = packInRows(lists.map((list) => list.map((account) => account.id)), metrics)
      }
      setPlaced(next)
      if (kind !== 'nearest') grid.current?.scrollTo({ left: 0, top: 0 })
      onArranged(readingOrder(ids, next))
    },
  }))

  function placeCard(current: Drag) {
    current.element.style.transform = `${restTransform(current.x, current.y)} rotate(${current.tilt.toFixed(2)}deg) scale(1.04)`
  }

  // The pointer stays on the same point of the card, also while the grid scrolls under it.
  function follow(current: Drag) {
    const bounds = canvas.current?.getBoundingClientRect()
    if (!bounds) return
    current.x = Math.max(0, current.clientX - bounds.left - current.grabX)
    current.y = Math.max(0, current.clientY - bounds.top - current.grabY)
    const right = current.x + metrics.cellW + metrics.padX
    const bottom = current.y + metrics.cellH + metrics.padX
    setGrown((size) => (right > size.width || bottom > size.height
      ? { width: Math.max(size.width, right), height: Math.max(size.height, bottom) }
      : size))
  }

  function frame(time: number) {
    const current = drag.current
    if (!current?.active) return
    const dt = Math.min(0.05, (time - current.lastTime) / 1000)
    current.lastTime = time

    const velocity = dt > 0 ? (current.clientX - current.lastClientX) / dt : 0
    current.lastClientX = current.clientX
    const target = Math.max(-MAX_TILT, Math.min(MAX_TILT, velocity / 220))
    current.tilt += (target - current.tilt) * (1 - Math.exp(-dt * 14))

    const container = grid.current
    if (container) {
      const bounds = container.getBoundingClientRect()
      const push = (distance: number) => SCROLL_SPEED * (1 - Math.max(0, distance) / SCROLL_EDGE) * dt
      if (current.clientY - bounds.top < SCROLL_EDGE) container.scrollTop -= push(current.clientY - bounds.top)
      else if (bounds.bottom - current.clientY < SCROLL_EDGE) container.scrollTop += push(bounds.bottom - current.clientY)
      if (current.clientX - bounds.left < SCROLL_EDGE) container.scrollLeft -= push(current.clientX - bounds.left)
      else if (bounds.right - current.clientX < SCROLL_EDGE) container.scrollLeft += push(bounds.right - current.clientX)
    }
    follow(current)
    placeCard(current)
    current.frame = requestAnimationFrame(frame)
  }

  function activate(current: Drag) {
    current.active = true
    current.lastTime = performance.now()
    setDraggingId(current.id)
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
    setDraggingId(null)
    setGrown({ width: 0, height: 0 })

    const now = latest.current.spots
    if (!commit) {
      current.element.style.transform = restTransform(now[current.id].x, now[current.id].y)
      return
    }
    const x = Math.round(current.x)
    const y = Math.round(current.y)
    current.element.style.transform = restTransform(x, y)
    // From the first move on every card holds its place: a dropped card pushes nothing away.
    const top = Math.max(0, ...Object.values(now).map((spot) => spot.z ?? 0))
    setPlaced({ ...now, [current.id]: { x, y, z: top + 1 } })
  }

  function onPointerMove(event: PointerEvent) {
    const current = drag.current
    if (!current || event.pointerId !== current.pointerId) return
    current.clientX = event.clientX
    current.clientY = event.clientY
    if (!current.active && Math.hypot(current.clientX - current.startX, current.clientY - current.startY) >= DRAG_THRESHOLD) {
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

  function begin(event: React.PointerEvent<HTMLDivElement>, account: Account) {
    if (!enabled || drag.current) return
    if (event.pointerType === 'mouse' && event.button !== 0) return
    if ((event.target as Element).closest(NOT_A_HANDLE)) return
    const rect = event.currentTarget.getBoundingClientRect()
    const spot = spots[account.id]
    drag.current = {
      id: account.id,
      pointerId: event.pointerId,
      element: event.currentTarget,
      startX: event.clientX,
      startY: event.clientY,
      clientX: event.clientX,
      clientY: event.clientY,
      grabX: event.clientX - rect.left,
      grabY: event.clientY - rect.top,
      x: spot.x,
      y: spot.y,
      active: false,
      frame: 0,
      lastTime: 0,
      lastClientX: event.clientX,
      tilt: 0,
      cancel: () => {
        if (drag.current) finish(drag.current, false)
      },
    }
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerCancel)
    window.addEventListener('keydown', onKeyDown, { capture: true })
  }

  useEffect(() => () => drag.current?.cancel(), [])

  const smooth = animate && !(window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false)

  return (
    <div ref={grid} className={`grid2d${enabled ? ' movable' : ''}${smooth ? ' animated' : ''}`}>
      <div
        ref={canvas}
        className="grid2d-canvas"
        style={{ width: Math.max(area.width, grown.width), height: Math.max(area.height, grown.height) }}
      >
        {width > 0 &&
          accounts.map((account) => {
            const spot = spots[account.id]
            return (
              <div
                className={`grid2d-cell${draggingId === account.id ? ' dragging' : ''}`}
                key={account.id}
                data-account-id={account.id}
                style={{
                  width: metrics.cellW,
                  height: metrics.cellH,
                  transform: restTransform(spot.x, spot.y),
                  zIndex: spot.z ?? 0,
                }}
                onPointerDown={(event) => begin(event, account)}
              >
                {renderCard(account)}
              </div>
            )
          })}
      </div>
    </div>
  )
})

export default FreeGrid
