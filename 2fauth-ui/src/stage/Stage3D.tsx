import { useEffect, useRef } from 'react'
import { layoutMeta, layoutPoint, project, type StageLayout } from './layouts'

interface Stage3DProps {
  layout: StageLayout
  count: number
  autoRotate?: boolean
  zoom?: number
  className?: string
  dim?: number
  onDragStateChange?: (dragging: boolean) => void
  items: React.ReactNode[]
}

/**
 * Card layouts are pre-rendered at these scales (LOD). The transform then only
 * scales by ~1, so the text is rasterised at its final size and stays sharp.
 */
const LOD_STEPS = [0.8, 1, 1.2, 1.4, 1.6]

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
 * every code stays readable while the scene spins. Resolution adapts to depth
 * (LOD buckets), rotation is free on both axes, wheel zooms, double click resets.
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
  const items = useRef<(HTMLDivElement | null)[]>([])
  const lods = useRef<number[]>([])
  const drag = useRef<DragState>({ active: false, moved: 0, lastX: 0, lastY: 0, vx: 0, vy: 0 })
  const state = useRef({
    rx: 0,
    ry: 0,
    tRx: 0,
    tRy: 0,
    zoom: 1,
    tZoom: 1,
    spinning: autoRotate,
    spinSpeed: 0.18,
    dirty: true,
  })

  useEffect(() => {
    const meta = layoutMeta(layout)
    const current = state.current
    current.rx = meta.rx
    current.ry = meta.ry
    current.tRx = meta.rx
    current.tRy = meta.ry
    current.zoom = meta.zoom
    current.tZoom = meta.zoom * zoom
    current.dirty = true
  }, [layout, zoom])

  useEffect(() => {
    state.current.spinning = autoRotate
  }, [autoRotate])

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
    }
  }, [onDragStateChange])

  useEffect(() => {
    const current = state.current
    let frame = 0
    let last = performance.now()

    const render = (now: number) => {
      const dt = Math.min(0.05, (now - last) / 1000)
      last = now
      let moving = false

      if (current.spinning && !drag.current.active) {
        current.tRy += dt * current.spinSpeed
        moving = true
      }
      if (!drag.current.active && (Math.abs(drag.current.vx) > 0.0005 || Math.abs(drag.current.vy) > 0.0005)) {
        current.tRy += drag.current.vx
        current.tRx = clamp(current.tRx + drag.current.vy, -1.2, 1.2)
        drag.current.vx *= 0.94
        drag.current.vy *= 0.94
        moving = true
      }

      const ease = Math.min(1, dt * 12)
      if (Math.abs(current.tRy - current.ry) > 0.0001) {
        current.ry += (current.tRy - current.ry) * ease
        moving = true
      }
      if (Math.abs(current.tRx - current.rx) > 0.0001) {
        current.rx += (current.tRx - current.rx) * ease
        moving = true
      }
      if (Math.abs(current.tZoom - current.zoom) > 0.0005) {
        current.zoom += (current.tZoom - current.zoom) * Math.min(1, dt * 10)
        moving = true
      }

      if (moving || current.dirty) {
        const focal = 1150
        for (let index = 0; index < count; index += 1) {
          const element = items.current[index]
          if (!element) continue
          const point = layoutPoint(layout, index, count)
          const projected = project(point, current.rx, current.ry, current.zoom, focal)
          const scale = clamp(projected.scale, 0.2, 2.6)

          let bucket = 0
          let best = Infinity
          for (let candidate = 0; candidate < LOD_STEPS.length; candidate += 1) {
            const delta = Math.abs(LOD_STEPS[candidate] - scale)
            if (delta < best) {
              best = delta
              bucket = candidate
            }
          }
          if (lods.current[index] !== bucket) {
            lods.current[index] = bucket
            element.dataset.lod = String(bucket)
          }

          const residual = clamp(scale / LOD_STEPS[bucket], 0.74, 1.06)
          const opacity = clamp(1 - Math.max(0, -projected.depth - 320) / 1200, 0.22, 1) * (1 - clamp(dim, 0, 1) * 0.45)

          element.style.transform = `translate(-50%, -50%) translate3d(${projected.x.toFixed(1)}px, ${projected.y.toFixed(1)}px, 0) scale(${residual.toFixed(3)})`
          element.style.zIndex = String(Math.round(2000 - projected.depth))
          element.style.opacity = opacity.toFixed(3)
        }
        current.dirty = false
      }

      frame = requestAnimationFrame(render)
    }

    frame = requestAnimationFrame(render)
    return () => cancelAnimationFrame(frame)
  }, [layout, count, dim])

  function onPointerDown(event: React.PointerEvent) {
    if (event.pointerType === 'mouse' && event.button !== 0) return
    drag.current = { active: true, moved: 0, lastX: event.clientX, lastY: event.clientY, vx: 0, vy: 0 }
    if (viewport.current) viewport.current.dataset.dragged = '0'
    onDragStateChange?.(true)
  }

  function onWheel(event: React.WheelEvent) {
    const current = state.current
    current.tZoom = clamp(current.tZoom * (event.deltaY > 0 ? 0.94 : 1.06), 0.45, 2.2)
    current.dirty = true
  }

  function reset() {
    const meta = layoutMeta(layout)
    const current = state.current
    current.tRx = meta.rx
    current.tRy = meta.ry
    current.tZoom = meta.zoom * zoom
    current.dirty = true
  }

  return (
    <div
      ref={viewport}
      className={`stage3d${className ? ` ${className}` : ''}`}
      onPointerDown={onPointerDown}
      onWheel={onWheel}
      onDoubleClick={reset}
    >
      <div className="stage3d-world">
        {cards.map((card, index) => (
          <div
            key={index}
            className="stage3d-item"
            data-lod="1"
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

/** True when the pointer sequence that produced this event was a drag. */
export function wasDragged(element: Element | null): boolean {
  const stage = element?.closest('.stage3d') as HTMLElement | null
  return stage?.dataset.dragged === '1'
}
