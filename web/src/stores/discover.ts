import { create } from 'zustand'

import type { JavDBZone } from '@/api/discover'

export type DiscoverView = 'released' | 'upcoming' | 'category'

export type CategoryFilters = {
  zone: JavDBZone
  categoryID: string
  tagID: string
  main: string
}

type DiscoverState = {
  view: DiscoverView
  pages: Record<DiscoverView, number>
  category: CategoryFilters
  setView: (view: DiscoverView) => void
  setPage: (view: DiscoverView, page: number) => void
  updateCategory: (filters: Partial<CategoryFilters>) => void
}

// Keep the browsing context when a movie detail page unmounts the list.
export const useDiscoverStore = create<DiscoverState>(set => ({
  view: 'released',
  pages: { released: 1, upcoming: 1, category: 1 },
  category: { zone: 'censored', categoryID: '', tagID: '', main: '' },
  setView: view => set({ view }),
  setPage: (view, page) => set(state => ({ pages: { ...state.pages, [view]: page } })),
  updateCategory: filters =>
    set(state => ({
      category: { ...state.category, ...filters },
      pages: { ...state.pages, category: 1 }
    }))
}))
