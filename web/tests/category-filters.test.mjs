import assert from 'node:assert/strict'
import { test } from 'node:test'

import { categoryBrowseParams } from '../src/features/discover/category-params.ts'
import { useDiscoverStore } from '../src/stores/discover.ts'

function discoverStore(t) {
  const previous = useDiscoverStore.getState()
  useDiscoverStore.setState(useDiscoverStore.getInitialState(), true)
  t.after(() => useDiscoverStore.setState(previous, true))
  return useDiscoverStore
}

test('subcategory changes preserve the common filter and reset only the category page', t => {
  const store = discoverStore(t)
  const { updateCategory, setPage } = store.getState()
  setPage('released', 4)
  setPage('upcoming', 2)
  updateCategory({ categoryID: 'category-1', tagID: 'tag-1' })
  updateCategory({ main: 'm' })
  assert.deepEqual(categoryBrowseParams(store.getState().category), {
    zone: 'censored',
    main: ['m'],
    tagIds: ['tag-1']
  })

  setPage('category', 3)
  updateCategory({ tagID: 'tag-2' })
  assert.deepEqual(categoryBrowseParams(store.getState().category), {
    zone: 'censored',
    main: ['m'],
    tagIds: ['tag-2']
  })
  assert.deepEqual(store.getState().pages, { released: 4, upcoming: 2, category: 1 })
})

test('year browsing combines the common filter with a year instead of a tag ID', t => {
  const store = discoverStore(t)
  const { updateCategory } = store.getState()
  updateCategory({ categoryID: 'category-1', tagID: 'tag-1', main: 'm' })
  updateCategory({ categoryID: 'year', tagID: '' })
  assert.deepEqual(categoryBrowseParams(store.getState().category), {
    zone: 'censored',
    main: ['m']
  })

  updateCategory({ tagID: '2026' })
  assert.deepEqual(categoryBrowseParams(store.getState().category), {
    zone: 'censored',
    main: ['m'],
    year: '2026'
  })
})

test('the common filter and selected subcategory can be cleared independently', t => {
  const store = discoverStore(t)
  const { updateCategory, setPage } = store.getState()
  updateCategory({ categoryID: 'category-1', tagID: 'tag-1', main: 'm' })
  setPage('category', 3)
  updateCategory({ main: '' })
  assert.deepEqual(categoryBrowseParams(store.getState().category), {
    zone: 'censored',
    tagIds: ['tag-1']
  })
  assert.equal(store.getState().pages.category, 1)

  updateCategory({ main: 'm' })
  updateCategory({ tagID: '' })
  assert.deepEqual(categoryBrowseParams(store.getState().category), {
    zone: 'censored',
    main: ['m']
  })
})

test('selecting a common filter replaces the previous choice and resets pagination', t => {
  const store = discoverStore(t)
  const { updateCategory, setPage } = store.getState()
  updateCategory({ categoryID: 'category-1', tagID: 'tag-1', main: 'm' })
  setPage('category', 3)
  updateCategory({ main: 'c' })
  assert.deepEqual(categoryBrowseParams(store.getState().category), {
    zone: 'censored',
    main: ['c'],
    tagIds: ['tag-1']
  })
  assert.equal(store.getState().pages.category, 1)
  assert.deepEqual(categoryBrowseParams(store.getInitialState().category), { zone: 'censored' })
})
