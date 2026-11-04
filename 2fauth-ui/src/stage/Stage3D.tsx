import { useEffect, useMemo, useRef } from 'react'
import { useT } from '../i18n'
import { layoutMeta, layoutPoint, project, spread, type StageLayout } from './layouts'

interface Stage3DProps {
  layout: StageLayout
  count: number
  autoRotate?: boolean
  zoom?: number
  /** multiplies the radius of the layouts that have one */
  radius?: number
  className?: string
  /** frame rate cap, 0 for the refresh rate of the display */
  fps?: number
  /** frosted glass: the blur stays on the item, so only the card itself may fade */
  glass?: boolean
  /** the card brought to the front; all the others fade */
  focusIndex?: number | null
  /** a click outside the card brought to the front */
  onBackgroundClick?: () => void
  onDragStateChange?: (dragging: boolean) => void
  /** shown in a corner of the stage; using them does not touch the scene */
  controls?: React.ReactNode
  items: React.ReactNode[]
}

/**
 * The size a card is laid out at, compared with the size it has at scale 1. Every card
 * keeps this one layout at any zoom, only a transform scales it, so nothing in it can
 * change its proportions. See also --card-fs in the stylesheet.
 */
const CARD_RESOLUTION = 1.5
/** Once this long without movement the browser draws the cards again at their exact size. */
const REST_MS = 160
/** The on-screen scale of a card at the middle of the scene, as far as the wheel can take it. */
const MIN_ZOOM = 0.12
const MAX_ZOOM = 3
/** What one notch of a wheel (100 pixels) changes the zoom by, as an exponent. */
const WHEEL_SPEED = 0.0009

/** Opacity of a card at the back and of the cards around a focused one. */
const FAR_OPACITY = 0.14
const UNFOCUSED_OPACITY = 0.1
const NEAR_TIER = 0.6
const FAR_TIER = 0.28
const FOCUS_SCALE = 2

const BASE_FOCAL = 1150
const NEAR_FOCAL_RATIO = 4
/** A press that moved further than this is not a click. */
const DRAG_SLOP = 6

interface DragState {
  active: boolean
  moved: number
  lastX: number
  lastY: number
  vx: number
  vy: number
}

/**
 * A CSS-3D stage: cards are placed in 3D space, projected, and billboarded so
 * every code stays readable while the scene spins. Depth reads as size and opacity,
 * a clicked card comes to the front. Each frame only writes transform and opacity,
 * which the compositor handles without repainting anything.
 */
export default function Stage3D({
  layout,
  count,
  autoRotate = false,
  zoom = 1,
  radius = 1,
  className = '',
  fps = 0,
  glass = false,
  focusIndex = null,
  onBackgroundClick,
  onDragStateChange,
  controls,
  items: cards,
}: Stage3DProps) {
  const t = useT()
  const viewport = useRef<HTMLDivElement>(null)
  const items = useRef<(HTMLDivElement | null)[]>([])
  const tiers = useRef<string[]>([])
  const focusAmount = useRef<number[]>([])
  const written = useRef<{ transform: string; z: number }[]>([])
  const drag = useRef<DragState>({ active: false, moved: 0, lastX: 0, lastY: 0, vx: 0, vy: 0 })
  const hovered = useRef(false)
  const focus = useRef<number | null>(focusIndex)
  const press = useRef({ x: 0, y: 0, inFocus: false })
  const collapsedAt = useRef(0)
  const shownLayout = useRef<StageLayout | null>(null)
  const state = useRef({
    rx: 0,
    ry: 0,
    tRx: 0,
    tRy: 0,
    zoom: 1,
    tZoom: 1,
    fit: 1,
    userZoom: 1,
    radius,
    tRadius: radius,
    spinning: autoRotate,
    spinSpeed: 0.18,
    dim: 0,
    dirty: true,
  })

  const points = useMemo(
    () => Array.from({ length: count }, (_, index) => layoutPoint(layout, index, count)),
    [layout, count],
  )
  const reach = useMemo(() => {
    let farthest = 0
    for (const point of points) farthest = Math.max(farthest, Math.hypot(point.x, point.z))
    return farthest
  }, [points])

  // While a card is enlarged the scene behind it holds still, whatever was moving it.
  useEffect(() => {
    focus.current = focusIndex
    const current = state.current
    current.dirty = true
    if (focusIndex === null) {
      const selection = window.getSelection()
      if (selection?.anchorNode && viewport.current?.contains(selection.anchorNode)) selection.removeAllRanges()
      return
    }
    drag.current.vx = 0
    drag.current.vy = 0
    current.tRx = current.rx
    current.tRy = current.ry
    current.tZoom = current.zoom
  }, [focusIndex])

  useEffect(() => {
    state.current.tRadius = radius
    state.current.dirty = true
  }, [radius])

  // Rests the layout on the available space, whatever the number of cards.
  useEffect(() => {
    const element = viewport.current
    if (!element) return
    const current = state.current
    const meta = layoutMeta(layout)

    // Measured on the layout as it comes: a larger radius really makes the scene larger.
    const fit = () => {
      let height = 0
      for (const point of points) height = Math.max(height, Math.abs(point.y))
      const focal = Math.max(BASE_FOCAL, reach * NEAR_FOCAL_RATIO)
      // The nearest cards are the largest and the camera looks slightly from above,
      // which is what decides whether the scene fits.
      const near = focal / Math.max(focal - reach, 1)
      const tilt = Math.abs(Math.sin(meta.rx)) * reach
      const halfW = element.clientWidth / 2
      const halfH = element.clientHeight / 2
      current.fit = clamp(Math.min(halfW / (reach + 170), halfH / ((height + tilt + 110) * near)), 0.3, 1)
      current.tZoom = baseZoom() * current.userZoom
      current.dirty = true
    }

    // A filter or a new account changes the fit, not the point of view.
    if (shownLayout.current !== layout) {
      shownLayout.current = layout
      current.rx = meta.rx
      current.ry = meta.ry
      current.tRx = meta.rx
      current.tRy = meta.ry
      current.userZoom = 1
      current.radius = current.tRadius
      fit()
      current.zoom = current.tZoom
    } else {
      fit()
    }

    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(fit)
    observer.observe(element)
    return () => observer.disconnect()
  }, [layout, zoom, points, reach])

  useEffect(() => {
    state.current.spinning = autoRotate
  }, [autoRotate])

  // Not through React: its wheel listener is passive, and a pinch must not zoom the page.
  useEffect(() => {
    const element = viewport.current
    if (!element) return
    const onWheel = (event: WheelEvent) => {
      event.preventDefault()
      if (focus.current !== null) return
      const current = state.current
      const pixels = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? 400 : 1)
      const base = baseZoom()
      current.userZoom = clamp(
        current.userZoom * Math.exp(-clamp(pixels, -300, 300) * WHEEL_SPEED),
        Math.min(1, MIN_ZOOM / base),
        Math.max(1, MAX_ZOOM / base),
      )
      current.tZoom = base * current.userZoom
      current.dirty = true
    }
    element.addEventListener('wheel', onWheel, { passive: false })
    return () => element.removeEventListener('wheel', onWheel)
    // baseZoom only reads the layout and the zoom
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layout, zoom])

  // Pointer handling lives on the window and never captures the pointer, so the
  // card buttons (copy / edit / pin) receive ordinary clicks.
  useEffect(() => {
    const current = state.current

    function onMove(event: PointerEvent) {
      if (!drag.current.active) return
      const dx = event.clientX - drag.current.lastX
      const dy = event.clientY - drag.current.lastY
      drag.current.lastX = event.clientX
      drag.current.lastY = event.clientY
      drag.current.moved += Math.abs(dx) + Math.abs(dy)
      if (drag.current.moved > 6 && viewport.current) viewport.current.dataset.dragged = '1'
      if (drag.current.moved > 2) {
        current.tRy += dx * 0.006
        current.tRx = clamp(current.tRx + dy * 0.0045, -1.2, 1.2)
        drag.current.vx = dx * 0.0025
        drag.current.vy = dy * 0.0018
        current.dirty = true
      }
    }

    function onUp() {
      if (!drag.current.active) return
      drag.current.active = false
      document.body.classList.remove('stage-rotating')
      onDragStateChange?.(false)
      current.dirty = true
      window.setTimeout(() => {
        drag.current.moved = 0
      }, 40)
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
      document.body.classList.remove('stage-rotating')
    }
  }, [onDragStateChange])

  useEffect(() => {
    const current = state.current
    const depths = new Float64Array(count)
    const screenX = new Float64Array(count)
    const screenY = new Float64Array(count)
    const scales = new Float64Array(count)
    const interval = fps > 0 ? 1000 / fps : 0
    let frame = 0
    let last = performance.now()
    let lastDrawn = 0
    let restAt = 0
    let inMotion = false

    // While things move the cards are moved as finished pictures, which costs nothing;
    // at rest the browser draws them once at their exact size, like a vector drawing.
    const setInMotion = (value: boolean) => {
      if (inMotion === value || !viewport.current) return
      inMotion = value
      viewport.current.dataset.moving = value ? '1' : '0'
    }

    const render = (now: number) => {
      frame = requestAnimationFrame(render)
      if (interval > 0 && now - lastDrawn < interval - 1) return
      lastDrawn = now

      // Everything below scales with the time elapsed, so the motion is the same at
      // 30 and at 240 frames per second.
      const dt = Math.min(0.05, (now - last) / 1000)
      last = now
      const frames = dt * 60
      let moving = false
      const focused = focus.current

      if (current.spinning && !drag.current.active && !hovered.current && focused === null) {
        current.tRy += dt * current.spinSpeed
        moving = true
      }
      if (!drag.current.active && (Math.abs(drag.current.vx) > 0.0005 || Math.abs(drag.current.vy) > 0.0005)) {
        current.tRy += drag.current.vx * frames
        current.tRx = clamp(current.tRx + drag.current.vy * frames, -1.2, 1.2)
        const decay = Math.pow(0.94, frames)
        drag.current.vx *= decay
        drag.current.vy *= decay
        moving = true
      }

      const ease = 1 - Math.exp(-dt * 12)
      if (Math.abs(current.tRy - current.ry) > 0.0001) {
        current.ry += (current.tRy - current.ry) * ease
        moving = true
      }
      if (Math.abs(current.tRx - current.rx) > 0.0001) {
        current.rx += (current.tRx - current.rx) * ease
        moving = true
      }
      // In equal ratios, not in equal steps: zooming keeps the same pace at any size.
      if (Math.abs(current.tZoom / current.zoom - 1) > 0.0005) {
        current.zoom *= Math.pow(current.tZoom / current.zoom, 1 - Math.exp(-dt * 10))
        moving = true
      } else if (current.zoom !== current.tZoom) {
        current.zoom = current.tZoom
        moving = true
      }
      if (Math.abs(current.tRadius - current.radius) > 0.0005) {
        current.radius += (current.tRadius - current.radius) * ease
        moving = true
      } else if (current.radius !== current.tRadius) {
        current.radius = current.tRadius
        moving = true
      }

      const focusEase = 1 - Math.exp(-dt * 11)
      const dimTarget = focused === null ? 0 : 1
      if (Math.abs(dimTarget - current.dim) > 0.002) {
        current.dim += (dimTarget - current.dim) * focusEase
        moving = true
      } else if (current.dim !== dimTarget) {
        // the final value still has to reach the screen
        current.dim = dimTarget
        moving = true
      }
      for (let index = 0; index < count; index += 1) {
        const target = index === focused ? 1 : 0
        const amount = focusAmount.current[index] ?? 0
        if (Math.abs(target - amount) > 0.002) {
          focusAmount.current[index] = amount + (target - amount) * focusEase
          moving = true
        } else if (amount !== target) {
          focusAmount.current[index] = target
          moving = true
        }
      }

      if (!moving && !current.dirty) {
        if (now >= restAt) setInMotion(false)
      } else {
        restAt = now + REST_MS
        setInMotion(true)
        const focal = Math.max(BASE_FOCAL, reach * current.radius * NEAR_FOCAL_RATIO)
        let nearest = Infinity
        let farthest = -Infinity
        for (let index = 0; index < count; index += 1) {
          const point = points[index]
          if (!point) continue
          const projected = project(spread(layout, point, current.radius), current.rx, current.ry, current.zoom, focal)
          depths[index] = projected.depth
          screenX[index] = projected.x
          screenY[index] = projected.y
          scales[index] = projected.scale
          if (projected.depth < nearest) nearest = projected.depth
          if (projected.depth > farthest) farthest = projected.depth
        }
        const range = farthest - nearest
        const focusScale = focusedScale(viewport.current)

        for (let index = 0; index < count; index += 1) {
          const element = items.current[index]
          if (!element) continue
          const amount = focusAmount.current[index] ?? 0

          // 1 at the front of the scene, 0 at the back; a flat scene is all front
          const front = range < 140 ? 1 : (farthest - depths[index]) / range
          const depthOpacity = FAR_OPACITY + (1 - FAR_OPACITY) * Math.pow(front, 1.5)
          const dimmed = depthOpacity + (Math.min(depthOpacity, UNFOCUSED_OPACITY) - depthOpacity) * current.dim
          const opacity = dimmed + (1 - dimmed) * amount

          const scale = scales[index] + (focusScale - scales[index]) * amount
          const x = screenX[index] * (1 - amount)
          const y = screenY[index] * (1 - amount)

          const tier = amount > 0.5 || front >= NEAR_TIER ? 'near' : front >= FAR_TIER ? 'mid' : 'far'
          if (tiers.current[index] !== tier) {
            tiers.current[index] = tier
            element.dataset.tier = tier
          }

          const transform = `translate(-50%, -50%) translate3d(${x.toFixed(2)}px, ${y.toFixed(2)}px, 0) scale(${(scale / CARD_RESOLUTION).toFixed(4)})`
          // written the way the browser reads it back
          const opacityText = String(Number(opacity.toFixed(3)))
          const z = amount > 0.01 ? 100000 : Math.round(50000 - depths[index])

          const before = written.current[index] ?? (written.current[index] = { transform: '', z: 0 })
          if (before.transform !== transform) {
            before.transform = transform
            element.style.transform = transform
          }
          const faded = glass ? ((element.firstElementChild as HTMLElement | null) ?? element) : element
          if (faded.style.opacity !== opacityText) faded.style.opacity = opacityText
          if (before.z !== z) {
            before.z = z
            element.style.zIndex = String(z)
          }
        }
        current.dirty = false
      }
    }

    frame = requestAnimationFrame(render)
    return () => cancelAnimationFrame(frame)
  }, [layout, points, reach, count, fps, glass])

  // A card replaced by another one starts without its opacity.
  useEffect(() => {
    if (glass) state.current.dirty = true
  })

  function onPointerDown(event: React.PointerEvent) {
    if (event.pointerType === 'mouse' && event.button !== 0) return
    press.current = { x: event.clientX, y: event.clientY, inFocus: Boolean((event.target as Element).closest('.is-focus')) }
    if (viewport.current) viewport.current.dataset.dragged = '0'
    if (focus.current !== null) return
    drag.current = { active: true, moved: 0, lastX: event.clientX, lastY: event.clientY, vx: 0, vy: 0 }
    document.body.classList.add('stage-rotating')
    onDragStateChange?.(true)
  }

  function baseZoom() {
    return layoutMeta(layout).zoom * zoom * state.current.fit
  }

  function reset() {
    // the second click of a double click that has just closed a card
    if (focus.current !== null || performance.now() - collapsedAt.current < 500) return
    const meta = layoutMeta(layout)
    const current = state.current
    current.tRx = meta.rx
    current.tRy = meta.ry
    current.userZoom = 1
    current.tZoom = baseZoom()
    current.dirty = true
  }

  // Resting on a card stops the rotation, so that it can be read and clicked.
  function onPointerOver(event: React.PointerEvent) {
    if (event.pointerType === 'mouse') hovered.current = Boolean((event.target as Element).closest('.orbit-card'))
  }

  // While a card is enlarged the other ones take no clicks, so a click either stays in
  // that card or lands here. Selecting text that ends outside the card is not a click.
  function onClick(event: React.MouseEvent) {
    if (focus.current === null) return
    const moved = Math.hypot(event.clientX - press.current.x, event.clientY - press.current.y)
    if (press.current.inFocus || moved > DRAG_SLOP || (event.target as Element).closest('.is-focus')) return
    collapsedAt.current = performance.now()
    onBackgroundClick?.()
  }

  return (
    <div
      ref={viewport}
      className={`stage3d${focusIndex !== null ? ' has-focus' : ''}${className ? ` ${className}` : ''}`}
      onPointerDown={onPointerDown}
      onPointerOver={onPointerOver}
      onPointerLeave={() => {
        hovered.current = false
      }}
      onClick={onClick}
      onDoubleClick={reset}
    >
      <div className="stage3d-world">
        {cards.map((card, index) => (
          <div
            key={index}
            className={`stage3d-item${index === focusIndex ? ' is-focus' : ''}`}
            data-tier="near"
            ref={(element) => {
              items.current[index] = element
            }}
          >
            {card}
          </div>
        ))}
      </div>
      {controls && (
        <div
          className="stage3d-controls"
          onPointerDown={(event) => event.stopPropagation()}
          onClick={(event) => event.stopPropagation()}
          onDoubleClick={(event) => event.stopPropagation()}
        >
          {controls}
        </div>
      )}
      <div className="stage3d-hint" aria-hidden>
        {focusIndex !== null
          ? t('Click outside the card to shrink it')
          : t('Drag to rotate · scroll to zoom · click a card to enlarge it · double-click to reset')}
      </div>
    </div>
  )
}

/** The on-screen scale of a focused card: large, but always within the stage. */
function focusedScale(element: HTMLElement | null): number {
  if (!element) return FOCUS_SCALE
  return clamp(Math.min((element.clientWidth * 0.8) / 300, (element.clientHeight * 0.8) / 196), 1, FOCUS_SCALE)
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

/** True when the pointer sequence that produced this event was a drag. */
export function wasDragged(element: Element | null): boolean {
  const stage = element?.closest('.stage3d') as HTMLElement | null
  return stage?.dataset.dragged === '1'
}
