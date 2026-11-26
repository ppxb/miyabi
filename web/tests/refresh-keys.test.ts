import assert from 'node:assert/strict'
import { test } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { libraryKeys } from '@/api/library'
import { taskKeys, taskRefreshKeys } from '@/api/tasks'

test('card refresh keys match library card lists only', () => {
  const client = new QueryClient()
  client.setQueryData(libraryKeys.movies(2), {})
  client.setQueryData(libraryKeys.browse({ page: 1 }), {})
  client.setQueryData(libraryKeys.topRated(12), {})
  client.setQueryData(libraryKeys.detail(7), {})
  client.setQueryData(libraryKeys.filterOptions, {})
  client.setQueryData(['library', 'pending'], {})

  const matched = libraryKeys.cards.flatMap(queryKey =>
    client
      .getQueryCache()
      .findAll({ queryKey })
      .map(query => query.queryKey[1])
  )
  assert.deepEqual(new Set(matched), new Set(['movies', 'browse', 'top-rated']))
})

test('task refresh only runs while the SSE stream is unavailable', () => {
  assert.deepEqual(taskRefreshKeys(true), [])
  assert.deepEqual(taskRefreshKeys(false), [taskKeys.all])
})
