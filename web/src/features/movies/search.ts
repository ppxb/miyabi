import { parseSearchPage } from '@/lib/search-schema'
import type { MoviesFilter } from './types'

/** URL search state for the movies page. List filters are comma-joined strings. */
export type MoviesSearch = {
  q?: string
  sort?: 'recent' | 'rating' | 'release'
  source?: '115' | 'local'
  tags?: string
  actors?: string
  makers?: string
  series?: string
  page?: number
}

function parseText(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed === '' ? undefined : trimmed
}

function splitList(value?: string): string[] | undefined {
  if (!value) return undefined
  const items = value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean)
  return items.length ? items : undefined
}

function joinList(value?: string[]): string | undefined {
  return value && value.length ? value.join(',') : undefined
}

export function validateMoviesSearch(search: Record<string, unknown>): MoviesSearch {
  const sort = search.sort
  const source = search.source
  if (sort !== undefined && sort !== 'recent' && sort !== 'rating' && sort !== 'release') {
    throw new Error('排序无效')
  }
  if (source !== undefined && source !== '115' && source !== 'local') {
    throw new Error('来源无效')
  }
  return {
    q: parseText(search.q),
    sort: sort === 'rating' || sort === 'release' ? sort : undefined,
    source: source === '115' || source === 'local' ? source : undefined,
    tags: parseText(search.tags),
    actors: parseText(search.actors),
    makers: parseText(search.makers),
    series: parseText(search.series),
    page: parseSearchPage(search.page)
  }
}

export function moviesFilterFromSearch(search: MoviesSearch): MoviesFilter {
  return {
    q: search.q,
    sort: search.sort,
    source: search.source,
    tagNames: splitList(search.tags),
    actorNames: splitList(search.actors),
    makerNames: splitList(search.makers),
    seriesNames: splitList(search.series)
  }
}

export function moviesSearchFromFilter(filter: MoviesFilter): MoviesSearch {
  return {
    q: filter.q,
    sort: filter.sort,
    source: filter.source,
    tags: joinList(filter.tagNames),
    actors: joinList(filter.actorNames),
    makers: joinList(filter.makerNames),
    series: joinList(filter.seriesNames)
  }
}
