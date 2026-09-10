import assert from 'node:assert/strict'
import { test } from 'node:test'
import { setImmediate } from 'node:timers/promises'
import { QueryClient, QueryObserver } from '@tanstack/react-query'

import {
  createMovieStateLoader,
  invalidateMovieStates,
  movieStateKeys,
  movieStateOptions,
  resetMovieStates
} from '../src/api/movie-state-cache.ts'
import { isOfflineTaskActive } from '../src/lib/offline-state.ts'

function queryClient(t) {
  const client = new QueryClient()
  t.after(() => client.clear())
  return client
}

function observe(t, client, options) {
  const observer = new QueryObserver(client, options)
  const unsubscribe = observer.subscribe(() => {})
  t.after(unsubscribe)
  return observer
}

function settled(observer) {
  if (observer.getCurrentResult().fetchStatus === 'idle') {
    return Promise.resolve(observer.getCurrentResult())
  }
  return new Promise(resolve => {
    const unsubscribe = observer.subscribe(result => {
      if (result.fetchStatus !== 'idle') return
      unsubscribe()
      resolve(result)
    })
  })
}

test('a grid and detail share movie state and batch requests without fetching catalogue data', async t => {
  const client = queryClient(t)
  let catalogueLoads = 0
  const catalogue = observe(t, client, {
    queryKey: ['discover', 'movie', '0'],
    queryFn: () => {
      catalogueLoads++
      return { id: '0', code: 'ABP-0', title: 'Cached detail', state: 'not_in_library' }
    },
    staleTime: 300_000
  })
  await settled(catalogue)

  let state = 'saving'
  const requests = []
  const load = createMovieStateLoader(async movies => {
    requests.push(movies)
    return movies.map(({ id }) => ({ id, state, ...(state === 'in_library' && { library_id: 7 }) }))
  })
  const cards = Array.from({ length: 24 }, (_, id) =>
    observe(t, client, movieStateOptions(load, { id: String(id), code: `ABP-${id}` }))
  )
  const detail = observe(t, client, movieStateOptions(load, { id: '0', code: 'ABP-0' }))
  await Promise.all([...cards, detail].map(settled))
  assert.equal(requests.length, 1)
  assert.equal(requests[0].length, 24)
  assert.deepEqual(detail.getCurrentResult().data, cards[0].getCurrentResult().data)

  state = 'in_library'
  await invalidateMovieStates(client)
  assert.equal(requests.length, 2)
  assert.equal(catalogueLoads, 1)
  assert.equal(client.getQueryState(['discover', 'movie', '0']).isInvalidated, false)
  for (const view of [cards[0], detail]) {
    assert.deepEqual(view.getCurrentResult().data, { state: 'in_library', library_id: 7 })
  }

  // A slower catalogue response carries stale local fields but cannot roll back the shared state.
  client.setQueryData(['discover', 'movie', '0'], {
    ...catalogue.getCurrentResult().data,
    state: 'not_in_library'
  })
  assert.deepEqual(detail.getCurrentResult().data, { state: 'in_library', library_id: 7 })
})

test('newly opened and previously inactive movies read current state after a missed event', async t => {
  const client = queryClient(t)
  let state = 'saving'
  const load = createMovieStateLoader(async movies => movies.map(({ id }) => ({ id, state })))
  const identity = { id: 'one', code: 'ABP-001', state: 'in_library', library_id: 99 }
  const first = observe(t, client, movieStateOptions(load, identity))
  await settled(first)
  first.destroy()

  state = 'not_in_library'
  await invalidateMovieStates(client)
  const reopened = observe(t, client, movieStateOptions(load, identity))
  await settled(reopened)
  assert.deepEqual(reopened.getCurrentResult().data, { state: 'not_in_library' })

  const newMovie = observe(t, client, movieStateOptions(load, { ...identity, id: 'two' }))
  // Old catalogue projections never provide playback before local verification.
  assert.deepEqual(newMovie.getCurrentResult().data, { state: 'not_in_library' })
  await settled(newMovie)
  assert.deepEqual(newMovie.getCurrentResult().data, { state: 'not_in_library' })
})

test('switching source clears playback and late responses cannot restore it', async t => {
  const client = queryClient(t)
  const requests = []
  const load = createMovieStateLoader(movies => {
    const response = Promise.withResolvers()
    requests.push({ movies, ...response })
    return response.promise
  })
  const observer = observe(t, client, movieStateOptions(load, { id: 'one', code: 'ABP-001' }))
  await setImmediate()
  requests[0].resolve([{ id: 'one', state: 'in_library', library_id: 1 }])
  await settled(observer)

  const previousRefresh = invalidateMovieStates(client)
  await setImmediate()
  assert.equal(requests.length, 2)
  const reset = resetMovieStates(client)
  await setImmediate()
  assert.equal(requests.length, 3)
  assert.deepEqual(observer.getCurrentResult().data, { state: 'not_in_library' })

  requests[2].resolve([{ id: 'one', state: 'not_in_library' }])
  await reset
  requests[1].resolve([{ id: 'one', state: 'in_library', library_id: 1 }])
  await previousRefresh
  await setImmediate()
  assert.deepEqual(client.getQueryData(movieStateKeys.movie('one')), { state: 'not_in_library' })
})

test('state requests respect the batch limit and skip cancelled subscriptions', async () => {
  const requests = []
  const load = createMovieStateLoader(async movies => {
    requests.push(movies)
    return movies.map(({ id }) => ({ id, state: 'not_in_library' }))
  })
  const controller = new AbortController()
  const aborted = load({ id: 'aborted', code: 'ABP-000' }, controller.signal)
  controller.abort()
  const rejection = assert.rejects(aborted, { name: 'AbortError' })
  const active = Array.from({ length: 205 }, (_, id) =>
    load(
      { id: String(id), code: `ABP-${id}`, title: 'Do not send catalogue metadata' },
      new AbortController().signal
    )
  )
  await Promise.all([...active, rejection])
  assert.deepEqual(
    requests.map(movies => movies.length),
    [100, 100, 5]
  )
  assert.deepEqual(Object.keys(requests[0][0]).sort(), ['code', 'id'])
})

test('an incomplete state response fails only the missing movie and can recover on refresh', async t => {
  const client = queryClient(t)
  let missing = true
  const load = createMovieStateLoader(async movies =>
    movies
      .filter(({ id }) => !missing || id !== 'one')
      .map(({ id }) => ({ id, state: 'in_library', library_id: 7 }))
  )
  const one = observe(t, client, movieStateOptions(load, { id: 'one', code: 'ABP-001' }))
  const two = observe(t, client, movieStateOptions(load, { id: 'two', code: 'ABP-002' }))
  await Promise.all([one, two].map(settled))
  assert.equal(one.getCurrentResult().isError, true)
  assert.equal(two.getCurrentResult().isSuccess, true)
  missing = false
  await invalidateMovieStates(client)
  assert.equal(one.getCurrentResult().isSuccess, true)
  assert.deepEqual(one.getCurrentResult().data, { state: 'in_library', library_id: 7 })
})

test('playable downloads remain active while background processing continues', () => {
  assert.equal(isOfflineTaskActive({ phase: 'in_library', processing: true }), true)
  assert.equal(isOfflineTaskActive({ phase: 'in_library', processing: false }), false)
  assert.equal(isOfflineTaskActive({ phase: 'downloading', processing: false }), true)
  assert.equal(isOfflineTaskActive({ phase: 'processing', processing: true }), true)
  assert.equal(isOfflineTaskActive({ phase: 'available', processing: false }), false)
})
