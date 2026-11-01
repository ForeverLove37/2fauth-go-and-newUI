import { useEffect, useRef } from 'react'
import { layoutMeta, layoutPoint, project, type StageLayout } from './layouts'

interface Stage3DProps {
  layout: StageLayout
  count: number
  autoRotate?: boolean
  /** scales the whole scene */
  zoom?: number
  className?: string
  dim?: number
  onDragStateChange?: (dragging: boolean) => void
  items: React.ReactNode[]
}

interface DragState {
  active: boolean
  moved: number
  lastX: number
  lastY: number
  vx: number
  vy: number
}

/**
 * A CSS-3D stage: cards are placed in 3D space, projected to the screen and
 * always face the viewer, so every code stays readable while the scene spins.
 * Drag to rotate on both axes, wheel to zoom, double click to reset.
 */
export default function Stage3D({
  layout,
  count,
  autoRotate = false,
  zoom = 1,
  className = '',
  dim = 0,
  onDragStateChange,
  items: cards,
}: Stage3DProps) {
  const viewport = useRef<HTMLDivElement>(null)
  const world = useRef<HTMLDivElement>(null)
  const items = useRef<(HTMLDivElement | null)[]>([])
  const drag = useRef<DragState>({ active: false, moved: 0, lastX: 0, lastY: 0, vx: 0, vy: 0 })
  const state = useRef({
    rx: layoutMeta(layout).rx,
    ry: layoutMeta(layout).ry,
    tRx: 0,
    tRy: 0,
    zoom,
    tZoom: zoom,
    spinning: autoRotate,
    spinSpeed: 0.18,
  })

  // reset the camera when the layout changes
  useEffect(() => {
    const meta = layoutMeta(layout)
    state.current.rx = meta.rx
    state.current.ry = meta.ry
    state.current.tRx = 0
    state.current.tRy = 0
    state.current.zoom = meta.zoom
    state.current.tZoom = meta.zoom * zoom
  }, [layout, zoom])

  useEffect(() => {
    state.current.spinning = autoRotate
  }, [autoRotate])

  useEffect(() => {
    let frame = 0
    let last = performance.now()

    const tick = (now: number) => {
      const dt = Math.min(0.05, (now - last) / 1000)
      last = now
      const current = state.current

      if (current.spinning && !drag.current.active) {
        current.tRy += dt * current.spinSpeed
      }

      // inertia after a drag
      if (!drag.current.active) {
        if (Math.abs(drag.current.vx) > 0.0005 || Math.abs(drag.current.vy) > 0.0005) {
          current.tRy += drag.current.vx
          current.tRx = clamp(current.tRx + drag.current.vy, -1.2, 1.2)
          drag.current.vx *= 0.94
          drag.current.vy *= 0.94
        }
      }

      // damping
      current.ry += (current.tRy - current.ry) * Math.min(1, dt * 12)
      current.rx += (current.tRx - current.rx) * Math.min(1, dt * 12)
      current.zoom += (current.tZoom - current.zoom) * Math.min(1, dt * 10)

      const rx = current.rx + layoutMeta(layout).rx
      const ry = current.ry
      const focal = 1150

      for (let index = 0; index < count; index += 1) {
        const element = items.current[index]
        if (!element) continue
        const point = layoutPoint(layout, index, count)
        const projected = project(point, rx, ry, current.zoom * (1150 / focal), focal)
        const depth = projected.depth
        const scale = clamp(projected.scale, 0.25, 2.4)
        const blur = Math.max(0, Math.min(2.6, (-depth - 120) / 320))
        const opacity = clamp(1 - Math.max(0, -depth - 260) / 900 - dim, 0.16, 1)
        element.style.transform = `translate(-50%, -50%) translate3d(${projected.x.toFixed(1)}px, ${projected.y.toFixed(1)}px, 0) scale(${scale.toFixed(3)})`
        element.style.zIndex = String(Math.round(2000 - depth))
        element.style.opacity = opacity.toFixed(3)
        element.style.filter = blur > 0.05 ? `blur(${blur.toFixed(2)}px)` : 'none'
      }

      frame = requestAnimationFrame(tick)
    }

    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [layout, count, dim])

  function onPointerDown(event: React.PointerEvent) {
    drag.current = { active: true, moved: 0, lastX: event.clientX, lastY: event.clientY, vx: 0, vy: 0 }
    if (viewport.current) viewport.current.dataset.dragged = '0'
    onDragStateChange?.(true)
    ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
  }

  function onPointerMove(event: React.PointerEvent) {
    if (!drag.current.active) return
    const dx = event.clientX - drag.current.lastX
    const dy = event.clientY - drag.current.lastY
    drag.current.lastX = event.clientX
    drag.current.lastY = event.clientY
    drag.current.moved += Math.abs(dx) + Math.abs(dy)
    if (viewport.current && drag.current.moved > 6) viewport.current.dataset.dragged = '1'
    const current = state.current
    current.tRy += dx * 0.006
    current.tRx = clamp(current.tRx + dy * 0.0045, -1.2, 1.2)
    drag.current.vx = dx * 0.0025
    drag.current.vy = dy * 0.0018
  }

  function onPointerUp() {
    if (!drag.current.active) return
    drag.current.active = false
    onDragStateChange?.(false)
    window.setTimeout(() => {
      drag.current.moved = 0
    }, 30)
  }

  function onWheel(event: React.WheelEvent) {
    const current = state.current
    const delta = event.deltaY > 0 ? 0.94 : 1.06
    current.tZoom = clamp(current.tZoom * delta, 0.45, 2.2)
  }

  function reset() {
    const meta = layoutMeta(layout)
    const current = state.current
    current.tRx = 0
    current.tRy = 0
    current.tZoom = meta.zoom * zoom
  }

  return (
    <div
      ref={viewport}
      className={`stage3d${className ? ` ${className}` : ''}`}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onWheel={onWheel}
      onDoubleClick={reset}
    >
      <div className="stage3d-world" ref={world}>
        {cards.map((card, index) => (
          <div
            key={index}
            className="stage3d-item"
            ref={(element) => {
              items.current[index] = element
            }}
          >
            {card}
          </div>
        ))}
      </div>
      <div className="stage3d-hint" aria-hidden>
        拖拽旋转 · 滚轮缩放 · 双击复位
      </div>
    </div>
  )
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

/** True when the last pointer sequence on the stage was a drag, not a click. */
export function wasDragged(element: Element | null): boolean {
  const stage = element?.closest('.stage3d') as HTMLElement | null
  return stage?.dataset.dragged === '1'
}
