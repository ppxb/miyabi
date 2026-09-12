import assert from 'node:assert/strict'
import { test } from 'node:test'

import { metadataBrowseParams } from '../src/features/discover/metadata-params.ts'

test('local tag searches work across zones while preserving the common filter', () => {
  assert.deepEqual(
    metadataBrowseParams({ kind: 'tag', id: 'tag-local', name: '标签', page: 1, main: 'm' }),
    { tagIds: ['tag-local'], main: ['m'] }
  )
})

test('tag searches combine the selected tag, its zone and the common filter', () => {
  assert.deepEqual(
    metadataBrowseParams({
      kind: 'tag',
      id: 'tag-1',
      name: '标签',
      zone: 'uncensored',
      page: 1,
      main: 'm'
    }),
    { zone: 'uncensored', tagIds: ['tag-1'], main: ['m'] }
  )
})

test('clearing the common filter preserves the tag search', () => {
  assert.deepEqual(
    metadataBrowseParams({
      kind: 'tag',
      id: 'tag-1',
      name: '标签',
      zone: 'censored',
      page: 2,
      main: ''
    }),
    { zone: 'censored', tagIds: ['tag-1'] }
  )
})

for (const kind of ['actor', 'maker', 'series', 'director']) {
  test(`${kind} searches apply the common filter without sending a forbidden zone or tag filter`, () => {
    const search = { kind, id: 'entity-1', name: '条目', page: 1, main: 'c', zone: 'uncensored' }
    assert.deepEqual(metadataBrowseParams(search), {
      entityType: kind,
      entityID: 'entity-1',
      main: ['c']
    })
    assert.deepEqual(metadataBrowseParams({ ...search, main: '' }), {
      entityType: kind,
      entityID: 'entity-1'
    })
  })
}
