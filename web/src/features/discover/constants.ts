import type { Zone } from '@/api/discover'

export const DISCOVER_PAGE_SIZE = 20

export const DISCOVER_ZONES: Array<{ value: Zone; label: string }> = [
  { value: 'censored', label: '有码' },
  { value: 'uncensored', label: '无码' },
  { value: 'fc2', label: 'FC2' },
  { value: 'western', label: '欧美' },
  { value: 'anime', label: '动漫' }
]
