export type StageLayout = 'orbit' | 'helix' | 'wall' | 'sphere'

export interface Vec3 {
  x: number
  y: number
  z: number
}

export interface LayoutMeta {
  id: StageLayout
  name: string
  hint: string
  /** default zoom for the layout */
  zoom: number
  /** initial camera rotations, radians */
  rx: number
  ry: number
}

export const layouts: LayoutMeta[] = [
  { id: 'orbit', name: '环绕轨道', hint: '卡片绕中心成环，拖拽自由旋转', zoom: 1, rx: -0.18, ry: 0 },
  { id: 'helix', name: '螺旋塔', hint: '卡片沿垂直螺旋排列，上下滚动转塔', zoom: 0.92, rx: -0.12, ry: 0 },
  { id: 'wall', name: '卡片墙', hint: '斜置网格墙，一览所有验证码', zoom: 0.86, rx: -0.34, ry: 0.08 },
  { id: 'sphere', name: '球面', hint: '卡片分布在球面，适合账号很多时', zoom: 0.8, rx: -0.1, ry: 0 },
]

export function layoutMeta(layout: StageLayout): LayoutMeta {
  return layouts.find((item) => item.id === layout) ?? layouts[0]
}

const GOLDEN = Math.PI * (3 - Math.sqrt(5))

/** Places `count` cards in 3D space for the given layout, centred on the origin. */
export function layoutPoint(layout: StageLayout, index: number, count: number): Vec3 {
  if (count <= 0) return { x: 0, y: 0, z: 0 }
  const n = Math.max(1, count)

  switch (layout) {
    case 'orbit': {
      // Past a dozen cards a single ring only stacks them: rings are added instead.
      const perRing = 14
      const rings = Math.ceil(n / perRing)
      const ring = Math.floor(index / perRing)
      const inRing = ring < rings - 1 ? perRing : n - perRing * (rings - 1)
      const radius = clampRadius(inRing, 260, 620)
      const angle = ((index % perRing) / inRing) * Math.PI * 2 - Math.PI / 2 + ring * 0.37
      return { x: Math.cos(angle) * radius, y: (ring - (rings - 1) / 2) * 230, z: Math.sin(angle) * radius }
    }
    case 'helix': {
      const perTurn = Math.max(5, Math.round(n / 6))
      const radius = Math.max(230, (perTurn * 320) / (Math.PI * 2))
      const angle = (index / perTurn) * Math.PI * 2
      const y = (index - (n - 1) / 2) * Math.max(220 / perTurn, n < 8 ? 96 : 0)
      return { x: Math.cos(angle) * radius, y, z: Math.sin(angle) * radius }
    }
    case 'wall': {
      const columns = Math.max(1, Math.ceil(Math.sqrt(n * 1.7)))
      const rows = Math.max(1, Math.ceil(n / columns))
      const col = index % columns
      const row = Math.floor(index / columns)
      const cellW = 320
      const cellH = 214
      const x = (col - (columns - 1) / 2) * cellW
      const y = -(row - (rows - 1) / 2) * cellH
      const z = Math.sin((col + 0.5) * 0.7 + (row + 0.5) * 0.5) * 70
      return { x, y, z }
    }
    case 'sphere': {
      // enough surface for every card
      const radius = Math.max(300, Math.sqrt((n * 62000) / (Math.PI * 4)))
      const y = n === 1 ? 0 : 1 - (index / (n - 1)) * 2
      const ring = Math.sqrt(Math.max(0, 1 - y * y))
      const theta = index * GOLDEN
      return { x: Math.cos(theta) * ring * radius, y: y * radius * 0.8, z: Math.sin(theta) * ring * radius }
    }
    default:
      return { x: 0, y: 0, z: 0 }
  }
}

/** The layouts built around a radius the user can change. */
export function hasRadius(layout: string): layout is 'orbit' | 'sphere' {
  return layout === 'orbit' || layout === 'sphere'
}

/** A point of the layout with its radius multiplied. The rings of an orbit keep their height. */
export function spread(layout: StageLayout, point: Vec3, factor: number): Vec3 {
  if (layout === 'orbit') return { x: point.x * factor, y: point.y, z: point.z * factor }
  if (layout === 'sphere') return { x: point.x * factor, y: point.y * factor, z: point.z * factor }
  return point
}

function clampRadius(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, 120 + n * 34))
}

/** Rotates a point around X then Y and returns screen coordinates. */
export function project(
  point: Vec3,
  rx: number,
  ry: number,
  zoom: number,
  focal = 1100,
): { x: number; y: number; depth: number; scale: number } {
  // rotate around X
  const cosX = Math.cos(rx)
  const sinX = Math.sin(rx)
  const y1 = point.y * cosX - point.z * sinX
  const z1 = point.y * sinX + point.z * cosX
  // rotate around Y
  const cosY = Math.cos(ry)
  const sinY = Math.sin(ry)
  const x2 = point.x * cosY - z1 * sinY
  const z2 = point.x * sinY + z1 * cosY

  const scale = (focal / (focal + z2)) * zoom
  return { x: x2 * scale, y: y1 * scale, depth: z2, scale }
}
