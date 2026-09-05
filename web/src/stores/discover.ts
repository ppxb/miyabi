import { create } from 'zustand'

import type { JavDBZone } from '@/api/discover'

export type DiscoverView = 'released' | 'upcoming' | 'category'

type DiscoverFilters = {
  zone: JavDBZone
  view: DiscoverView
  page: number
  keyword: string
  categoryID: string
  tagID: string
}

type DiscoverState = DiscoverFilters & {
  update: (filters: Partial<DiscoverFilters>) => void
}

// Keep the browsing context when a movie detail page unmounts the list.
export const useDiscoverStore = create<DiscoverState>(set => ({
  zone: 'censored',
  view: 'released',
  page: 1,
  keyword: '',
  categoryID: '',
  tagID: '',
  update: filters => set(filters)
}))
