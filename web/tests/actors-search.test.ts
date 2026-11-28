import { expect, test } from 'vitest'

import { validateActorsSearch } from '@/features/actors/search'

test('actor follow filter round-trips through the URL search', () => {
  expect(validateActorsSearch({ follow: 'followed' }).follow).toBe('followed')
  expect(validateActorsSearch({ follow: 'unfollowed' }).follow).toBe('unfollowed')
  expect(validateActorsSearch({}).follow).toBeUndefined()
})

test('invalid actor follow filter is rejected', () => {
  expect(() => validateActorsSearch({ follow: 'all' })).toThrow('关注筛选无效')
})
