import { clamp } from './math'

export type PageItem = number | 'ellipsis-left' | 'ellipsis-right'

const WINDOW = 3
// first + ellipsis + window + ellipsis + last
const MAX_SLOTS = WINDOW + 4

export function getPageNumbers(page: number, totalPages: number): PageItem[] {
  const total = Math.max(1, Math.floor(totalPages))
  if (total <= MAX_SLOTS) return Array.from({ length: total }, (_, i) => i + 1)

  const current = clamp(Math.floor(page), 1, total)

  const start = clamp(current - 1, 1, Math.max(1, total - WINDOW + 1))
  const end = Math.min(total, start + WINDOW - 1)

  const items: PageItem[] = []
  if (start > 1) {
    items.push(1)
    if (start === 3) items.push(2)
    else if (start > 3) items.push('ellipsis-left')
  }
  for (let p = start; p <= end; p++) items.push(p)
  if (end < total) {
    if (end === total - 2) items.push(total - 1)
    else if (end < total - 2) items.push('ellipsis-right')
    items.push(total)
  }
  return items
}
