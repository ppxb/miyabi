import { expect, test } from 'vitest'

import {
  moviesFilterFromSearch,
  moviesSearchFromFilter,
  validateMoviesSearch
} from '@/features/movies/search'

test('movie status filter round-trips through the URL search', () => {
  const search = validateMoviesSearch({ status: 'failed', source: 'local' })
  expect(search.status).toBe('failed')
  const filter = moviesFilterFromSearch(search)
  expect(filter.status).toBe('failed')
  expect(moviesSearchFromFilter(filter).status).toBe('failed')
})

test('empty or invalid status is dropped', () => {
  expect(validateMoviesSearch({}).status).toBeUndefined()
  expect(() => validateMoviesSearch({ status: 'nope' })).toThrow('刮削状态无效')
})
