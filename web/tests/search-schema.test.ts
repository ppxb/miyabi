import assert from 'node:assert/strict'
import { test } from 'vitest'

import { parseSearchPage, MAX_PAGE } from '@/lib/search-schema'

test('parseSearchPage accepts valid pages', () => {
  assert.equal(parseSearchPage(undefined), undefined)
  assert.equal(parseSearchPage(null), undefined)
  assert.equal(parseSearchPage(''), undefined)
  assert.equal(parseSearchPage(1), undefined)
  assert.equal(parseSearchPage('1'), undefined)
  assert.equal(parseSearchPage(2), 2)
  assert.equal(parseSearchPage('42'), 42)
  assert.equal(parseSearchPage(MAX_PAGE), MAX_PAGE)
})

test('parseSearchPage rejects invalid pages', () => {
  for (const invalid of [0, -1, 1.5, 'abc', NaN, Infinity, MAX_PAGE + 1]) {
    assert.throws(
      () => parseSearchPage(invalid),
      err => err instanceof Error && err.message === '页码无效'
    )
  }
})
