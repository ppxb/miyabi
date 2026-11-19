import type { BrowseLibraryParams } from '@/api/library'

export type MoviesFilter = Omit<BrowseLibraryParams, 'page' | 'limit'>
