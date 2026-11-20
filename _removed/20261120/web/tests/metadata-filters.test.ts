import assert from 'node:assert/strict'
import { test } from 'vitest'

import { metadataBrowseParams } from '@/features/discover/metadata-params'

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

for (const kind of ['actor', 'maker', 'series', 'director'] as const) {
  test(`${kind} searches apply the common filter without sending a forbidden zone or tag filter`, () => {
    const search = { kind, id: 'entity-1', name: '条目', page: 1, main: 'c', zone: 'uncensored' }
    // Exercise malformed persisted search state with a forbidden zone.
    // @ts-expect-error Entity searches do not accept a zone.
    assert.deepEqual(metadataBrowseParams(search), {
      entityType: kind,
      entityID: 'entity-1',
      main: ['c']
    })
    // @ts-expect-error Preserve the malformed zone when clearing the filter.
    assert.deepEqual(metadataBrowseParams({ ...search, main: '' }), {
      entityType: kind,
      entityID: 'entity-1'
    })
  })
}
