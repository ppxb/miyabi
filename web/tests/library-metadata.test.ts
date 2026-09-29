import assert from 'node:assert/strict'
import { test } from 'vitest'

import type { LibraryMovie } from '@/api/library'
import { libraryMovieMetadata } from '@/features/library/movie-metadata'

test('library hover links use scraped JavDB IDs instead of local database IDs', () => {
  const metadata = libraryMovieMetadata({
    id: 1,
    code: 'ABP-001',
    title: 'Fixture',
    duration: 0,
    rating: 0,
    scrape_status: 'done',
    maker: { id: 'maker-remote', name: '厂牌' },
    series: { id: 'series-remote', name: '系列' },
    director: { id: 'director-remote', name: '导演' },
    actors: [{ id: 'actor-remote', name: '演员' }],
    tags: [{ id: 42, javdb_id: 'tag-remote', name: '标签' }]
  })
  assert.deepEqual(metadata, {
    maker: { id: 'maker-remote', name: '厂牌' },
    series: { id: 'series-remote', name: '系列' },
    director: { id: 'director-remote', name: '导演' },
    actors: [{ id: 'actor-remote', name: '演员' }],
    tags: [{ id: 'tag-remote', name: '标签' }]
  })
})

test('incomplete local metadata keeps names without inventing searchable IDs', () => {
  const metadata = libraryMovieMetadata({
    id: 1,
    code: 'ABP-001',
    title: 'Fixture',
    duration: 0,
    rating: 0,
    scrape_status: 'done',
    actors: [],
    maker: { name: '本地厂牌' },
    // Legacy persisted tags may predate javdb_id.
    tags: [{ id: 42, name: '旧标签' } as LibraryMovie['tags'][number]]
  })
  assert.deepEqual(metadata.maker, { name: '本地厂牌' })
  assert.deepEqual(metadata.actors, [])
  assert.deepEqual(metadata.tags, [{ id: undefined, name: '旧标签' }])
  assert.equal(metadata.zone, undefined)
})
