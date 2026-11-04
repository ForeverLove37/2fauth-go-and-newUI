/** Where the cards of the free grid are, and the standard positions they can be tidied into. */

export interface Spot {
  x: number
  y: number
  /** stacking order, higher is on top */
  z?: number
}

export type Spots = Record<number, Spot>

export interface GridMetrics {
  cellW: number
  cellH: number
  gapX: number
  gapY: number
  padX: number
  padY: number
  columns: number
}

const CARD_W = 300
const CARD_H = 196
const GAP = 14
const PAD_X = 24
const PAD_Y = 4
/** How far two cards may touch before a position counts as taken. */
const OVERLAP_TOLERANCE = 8

export function gridMetrics(width: number): GridMetrics {
  const cellW = Math.max(200, Math.min(CARD_W, width - PAD_X * 2))
  const columns = Math.max(1, Math.floor((width - PAD_X * 2 + GAP) / (cellW + GAP)))
  return { cellW, cellH: CARD_H, gapX: GAP, gapY: GAP, padX: PAD_X, padY: PAD_Y, columns }
}

function slot(metrics: GridMetrics, index: number): Spot {
  const column = index % metrics.columns
  const row = Math.floor(index / metrics.columns)
  return {
    x: metrics.padX + column * (metrics.cellW + metrics.gapX),
    y: metrics.padY + row * (metrics.cellH + metrics.gapY),
  }
}

/** The cards one after the other, in reading order. */
export function packInOrder(ids: number[], metrics: GridMetrics): Spots {
  const spots: Spots = {}
  ids.forEach((id, index) => {
    spots[id] = slot(metrics, index)
  })
  return spots
}

/** Like packInOrder, each list starting on a row of its own. */
export function packInRows(lists: number[][], metrics: GridMetrics): Spots {
  const spots: Spots = {}
  let index = 0
  for (const ids of lists) {
    if (ids.length === 0) continue
    for (const id of ids) {
      spots[id] = slot(metrics, index)
      index += 1
    }
    index = Math.ceil(index / metrics.columns) * metrics.columns
  }
  return spots
}

function overlaps(a: Spot, b: Spot, metrics: GridMetrics): boolean {
  return (
    Math.abs(a.x - b.x) < metrics.cellW - OVERLAP_TOLERANCE &&
    Math.abs(a.y - b.y) < metrics.cellH - OVERLAP_TOLERANCE
  )
}

/** Keeps the cards that have a place; the others take the first positions nothing covers. */
export function placeMissing(ids: number[], placed: Spots, metrics: GridMetrics): Spots {
  const spots: Spots = {}
  const taken: Spot[] = []
  for (const id of ids) {
    if (placed[id]) {
      spots[id] = placed[id]
      taken.push(placed[id])
    }
  }
  let index = 0
  for (const id of ids) {
    if (spots[id]) continue
    let candidate = slot(metrics, index)
    while (taken.some((spot) => overlaps(spot, candidate, metrics))) {
      index += 1
      candidate = slot(metrics, index)
    }
    spots[id] = candidate
    taken.push(candidate)
    index += 1
  }
  return spots
}

/**
 * Every card goes to the nearest position still free. The shortest moves are decided
 * first, so a card already close to a position keeps it and no card travels far to
 * make room for another.
 */
export function packNearest(ids: number[], current: Spots, metrics: GridMetrics): Spots {
  let lowest = 0
  for (const id of ids) lowest = Math.max(lowest, current[id]?.y ?? 0)
  const rowsCovered = Math.ceil(lowest / (metrics.cellH + metrics.gapY)) + 1
  const rows = Math.max(rowsCovered, Math.ceil(ids.length / metrics.columns))
  const count = Math.max(rows * metrics.columns, ids.length)
  const slots = Array.from({ length: count }, (_, index) => slot(metrics, index))

  const pairs: { card: number; position: number; distance: number }[] = []
  ids.forEach((id, card) => {
    const from = current[id] ?? { x: 0, y: 0 }
    slots.forEach((to, position) => {
      pairs.push({ card, position, distance: Math.hypot(from.x - to.x, from.y - to.y) })
    })
  })
  pairs.sort((a, b) => a.distance - b.distance || a.card - b.card || a.position - b.position)

  const spots: Spots = {}
  const free = new Set(slots.keys())
  for (const pair of pairs) {
    const id = ids[pair.card]
    if (spots[id] || !free.has(pair.position)) continue
    spots[id] = slots[pair.position]
    free.delete(pair.position)
  }
  return spots
}

/** The ids as they read on screen, left to right and top to bottom. */
export function readingOrder(ids: number[], spots: Spots): number[] {
  return [...ids].sort((a, b) => spots[a].y - spots[b].y || spots[a].x - spots[b].x)
}

/** The bottom right corner of the area the cards need. */
export function extent(spots: Spots, metrics: GridMetrics): { width: number; height: number } {
  let width = 0
  let height = 0
  for (const spot of Object.values(spots)) {
    width = Math.max(width, spot.x + metrics.cellW + metrics.padX)
    height = Math.max(height, spot.y + metrics.cellH + metrics.padX)
  }
  return { width, height }
}

export function loadSpots(key: string): Spots {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(key) ?? '{}')
    const spots: Spots = {}
    for (const [id, value] of Object.entries(parsed as Record<string, Partial<Spot>>)) {
      if (Number.isFinite(value?.x) && Number.isFinite(value?.y)) {
        spots[Number(id)] = { x: Number(value.x), y: Number(value.y), z: Number(value.z) || 0 }
      }
    }
    return spots
  } catch {
    return {}
  }
}

export function saveSpots(key: string, spots: Spots) {
  try {
    localStorage.setItem(key, JSON.stringify(spots))
  } catch {
    /* the positions only last until the page is closed */
  }
}
