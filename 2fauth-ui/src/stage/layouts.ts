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
      const radius = clampRadius(n, 260, 520)
      const angle = (index / n) * Math.PI * 2 - Math.PI / 2
      return { x: Math.cos(angle) * radius, y: 0, z: Math.sin(angle) * radius }
    }
    case 'helix': {
      const radius = clampRadius(n, 230, 430)
      const turns = Math.max(1, Math.round(n / 5))
      const angle = (index / n) * Math.PI * 2 * turns
      const spacing = Math.min(160, Math.max(96, 900 / n))
      const y = (index - (n - 1) / 2) * spacing
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
      const radius = clampRadius(n, 300, 460)
      const y = n === 1 ? 0 : 1 - (index / (n - 1)) * 2
      const ring = Math.sqrt(Math.max(0, 1 - y * y))
      const theta = index * GOLDEN
      return { x: Math.cos(theta) * ring * radius, y: y * radius * 0.8, z: Math.sin(theta) * ring * radius }
    }
    default:
      return { x: 0, y: 0, z: 0 }
  }
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
