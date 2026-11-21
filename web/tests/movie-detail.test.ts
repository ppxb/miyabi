import assert from 'node:assert/strict'
import { test } from 'vitest'
import { QueryClient } from '@tanstack/react-query'

import { discoverKeys, findCachedMovieCard } from '@/api/movie-detail-cache'
import type { DiscoverMovieDetail } from '@/api/discover'

function movie(id: string, title = `Title ${id}`): DiscoverMovieDetail {
  return {
    id,
    code: `ABP-${id}`,
    title,
    origin_title: title,
    release_date: '',
    duration: 0,
    rating: 0,
    thumbnail: '',
    cover: `https://media.example/${id}.jpg`,
    preview_images: [],
    preview_video: '',
    magnets_count: 0,
    has_subtitle: false,
    has_preview: false,
    actors: [],
    tags: [],
    release_status: 'unknown',
    zone: 'censored',
    actor_movies: [],
    related_movies: []
  }
}

function clientFixture() {
  return new QueryClient({ defaultOptions: { queries: { gcTime: Infinity } } })
}

test('card lookup follows list replacement and removal without retaining orphaned entries', () => {
  const client = clientFixture()
  const older = movie('one', 'Older list result')
  const newer = movie('one', 'Newer list result')
  client.setQueryData(discoverKeys.movies({ page: 1 }), [older], { updatedAt: 100 })
  client.setQueryData(discoverKeys.movies({ page: 2 }), [newer], { updatedAt: 200 })
  assert.equal(findCachedMovieCard(client, 'one'), newer)
  client.setQueryData(discoverKeys.movies({ page: 2 }), [movie('two')])
  assert.equal(findCachedMovieCard(client, 'one'), older)
  client.removeQueries({ queryKey: discoverKeys.movies({ page: 1 }) })
  assert.equal(findCachedMovieCard(client, 'one'), undefined)
})

test('invalidated card data stays displayable but is excluded when fresh-only', async () => {
  const client = clientFixture()
  const card = movie('one')
  client.setQueryData(discoverKeys.movies({ page: 1 }), [card])
  await client.invalidateQueries({ queryKey: discoverKeys.movies({ page: 1 }) })
  assert.equal(findCachedMovieCard(client, 'one'), card)
  assert.equal(findCachedMovieCard(client, 'one', true), undefined)
})

test('full detail cache counts as a card source', () => {
  const client = clientFixture()
  const detail = movie('one')
  client.setQueryData(discoverKeys.movie('one'), detail)
  assert.deepEqual(findCachedMovieCard(client, 'one'), detail)
})
