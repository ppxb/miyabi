import assert from 'node:assert/strict'
import { test } from 'vitest'

import { categoryBrowseParams } from '@/features/discover/category-params'
import {
  categoryFromSearch,
  updateCategorySearch,
  validateDiscoverSearch
} from '@/features/discover/search'

test('subcategory changes preserve the common filter and reset only the category page', () => {
  const search = validateDiscoverSearch({
    view: 'category',
    releasedPage: 4,
    upcomingPage: 2,
    categoryPage: 3,
    categoryID: 'category-1',
    tagID: 'tag-1',
    main: 'm'
  })
  const next = updateCategorySearch(search, { tagID: 'tag-2' })
  assert.deepEqual(categoryBrowseParams(categoryFromSearch(next)), {
    zone: 'censored',
    main: ['m'],
    tagIds: ['tag-2']
  })
  assert.equal(next.categoryPage, undefined)
  assert.equal(next.releasedPage, 4)
  assert.equal(next.upcomingPage, 2)
})

test('year browsing combines the common filter with a year instead of a tag ID', () => {
  const search = validateDiscoverSearch({ categoryID: 'category-1', tagID: 'tag-1', main: 'm' })
  const year = updateCategorySearch(search, { categoryID: 'year', tagID: '' })
  assert.deepEqual(categoryBrowseParams(categoryFromSearch(year)), {
    zone: 'censored',
    main: ['m']
  })
  const next = updateCategorySearch(year, { tagID: '2026' })
  assert.deepEqual(categoryBrowseParams(categoryFromSearch(next)), {
    zone: 'censored',
    main: ['m'],
    year: '2026'
  })
})

test('the common filter and selected subcategory can be cleared independently', () => {
  const search = validateDiscoverSearch({
    categoryID: 'category-1',
    tagID: 'tag-1',
    main: 'm',
    categoryPage: 3
  })
  const noMain = updateCategorySearch(search, { main: '' })
  assert.deepEqual(categoryBrowseParams(categoryFromSearch(noMain)), {
    zone: 'censored',
    tagIds: ['tag-1']
  })
  assert.equal(noMain.categoryPage, undefined)
  const noTag = updateCategorySearch(search, { tagID: '' })
  assert.deepEqual(categoryBrowseParams(categoryFromSearch(noTag)), {
    zone: 'censored',
    main: ['m']
  })
})

test('selecting a common filter replaces the previous choice and resets pagination', () => {
  const search = validateDiscoverSearch({
    categoryID: 'category-1',
    tagID: 'tag-1',
    main: 'm',
    categoryPage: 3
  })
  const next = updateCategorySearch(search, { main: 'c' })
  assert.deepEqual(categoryBrowseParams(categoryFromSearch(next)), {
    zone: 'censored',
    main: ['c'],
    tagIds: ['tag-1']
  })
  assert.equal(next.categoryPage, undefined)
  assert.deepEqual(categoryBrowseParams(categoryFromSearch(validateDiscoverSearch({}))), {
    zone: 'censored'
  })
})
