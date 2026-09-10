import assert from 'node:assert/strict'
import { test } from 'node:test'
import { setImmediate } from 'node:timers/promises'
import { QueryClient, QueryObserver } from '@tanstack/react-query'

import {
  createMovieDetailLoader,
  discoverKeys,
  findCachedMovieCard
} from '../src/api/movie-detail-cache.ts'
import { observeRecommendation } from '../src/features/movie-detail/recommendation-visibility.ts'

function movie(id, title = `Title ${id}`) {
  return { id, code: `ABP-${id}`, title, cover: `https://media.example/${id}.jpg` }
}

function fixture(t) {
  const client = new QueryClient({ defaultOptions: { queries: { gcTime: Infinity } } })
  const requests = []
  const releases = []
  const loader = createMovieDetailLoader((id, signal) => {
    const pending = Promise.withResolvers()
    signal?.addEventListener('abort', () => pending.reject(signal.reason), { once: true })
    requests.push({ id, signal, ...pending })
    return pending.promise
  })
  t.after(() => {
    for (const release of releases) release()
    client.clear()
  })
  return {
    client,
    loader,
    requests,
    recommend(id) {
      const release = loader.request(client, id)
      releases.push(release)
      return release
    },
    observe(id, enabled = true) {
      const observer = new QueryObserver(client, { ...loader.options(id), enabled })
      observer.subscribe(() => {})
      releases.push(() => observer.destroy())
      return observer
    }
  }
}

test('unseen cards read cache without starting detail requests', async t => {
  const { observe, requests } = fixture(t)
  for (let id = 0; id < 16; id++) observe(String(id), false)
  await setImmediate()
  assert.equal(requests.length, 0)
})

test('both recommendation groups share two background slots and cancel unstarted work', async t => {
  const { recommend, requests } = fixture(t)
  recommend('one')
  recommend('two')
  const leaveThree = recommend('three')
  const leaveFourFirst = recommend('four')
  recommend('four')
  await setImmediate()
  assert.deepEqual(
    requests.map(request => request.id),
    ['one', 'two']
  )

  leaveThree()
  leaveThree()
  leaveFourFirst()
  requests[0].resolve(movie('one'))
  await setImmediate()
  assert.deepEqual(
    requests.map(request => request.id),
    ['one', 'two', 'four']
  )
  requests[1].resolve(movie('two'))
  requests[2].resolve(movie('four'))
  await setImmediate()
  assert.equal(requests.length, 3)
})

test('browsed and searched cards avoid detail requests without populating full-detail cache', async t => {
  const { client, loader, recommend, requests } = fixture(t)
  const browsed = movie('one')
  const searched = movie('two')
  client.setQueryData(discoverKeys.movies({ page: 1 }), [browsed])
  client.setQueryData(discoverKeys.search({ query: 'ABP' }), [searched])
  recommend('one')
  recommend('two')
  await setImmediate()
  assert.equal(requests.length, 0)
  assert.equal(findCachedMovieCard(client, 'one'), browsed)
  assert.equal(findCachedMovieCard(client, 'two'), searched)
  assert.equal(client.getQueryData(discoverKeys.movie('one')), undefined)

  // Clicking still fetches the real detail, including its recommendation lists.
  loader.prefetch(client, 'one')
  assert.deepEqual(
    requests.map(request => request.id),
    ['one']
  )
  const full = { ...browsed, actor_movies: [], related_movies: [], zone: 'censored' }
  requests[0].resolve(full)
  await setImmediate()
  assert.deepEqual(client.getQueryData(discoverKeys.movie('one')), full)
})

test('stale card data remains usable while details refresh and unrelated caches are ignored', async t => {
  const { client, recommend, requests } = fixture(t)
  const old = movie('one', 'Cached title')
  client.setQueryData(discoverKeys.movies({ page: 1 }), [old], { updatedAt: Date.now() - 600_000 })
  client.setQueryData(discoverKeys.magnets('one'), [{ id: 'one', title: 'Not a movie' }])
  client.setQueryData(['library', 'movies'], [movie('one', 'Different ID namespace')])
  assert.equal(findCachedMovieCard(client, 'one'), old)
  recommend('one')
  await setImmediate()
  assert.deepEqual(
    requests.map(request => request.id),
    ['one']
  )
  assert.equal(findCachedMovieCard(client, 'one'), old)
  requests[0].resolve(movie('one', 'Updated title'))
  await setImmediate()
  assert.equal(findCachedMovieCard(client, 'one').title, 'Updated title')
})

test('queued cards recheck newly available list data before using an upstream slot', async t => {
  const { client, recommend, requests } = fixture(t)
  recommend('one')
  recommend('two')
  recommend('three')
  await setImmediate()
  client.setQueryData(discoverKeys.search({ query: 'ABP-3' }), [movie('three')])
  requests[0].resolve(movie('one'))
  requests[1].resolve(movie('two'))
  await setImmediate()
  assert.deepEqual(
    requests.map(request => request.id),
    ['one', 'two']
  )
})

test('clicking a queued recommendation bypasses background work and reuses the request after navigation', async t => {
  const { client, loader, recommend, requests, observe } = fixture(t)
  recommend('one')
  recommend('two')
  recommend('three')
  const leaveFour = recommend('four')
  const card = observe('four', false)
  await setImmediate()
  loader.prefetch(client, 'four')
  assert.deepEqual(
    requests.map(request => request.id),
    ['one', 'two', 'four']
  )

  leaveFour()
  card.destroy()
  await setImmediate()
  const detail = observe('four')
  await setImmediate()
  assert.equal(requests.length, 3)
  const full = { ...movie('four'), actor_movies: [], related_movies: [] }
  requests[2].resolve(full)
  await setImmediate()
  assert.deepEqual(detail.getCurrentResult().data, full)
  assert.equal(requests.filter(request => request.id === 'four').length, 1)
})

test('a click before visibility loading also survives the observer gap', async t => {
  const { client, loader, requests, observe } = fixture(t)
  const card = observe('one', false)
  loader.prefetch(client, 'one')
  card.destroy()
  await setImmediate()
  const detail = observe('one')
  requests[0].resolve(movie('one'))
  await setImmediate()
  assert.equal(requests.length, 1)
  assert.equal(detail.getCurrentResult().data.title, 'Title one')
})

test('a failed card releases its slot and retries only on explicit demand', async t => {
  const { client, loader, recommend, requests } = fixture(t)
  recommend('one')
  recommend('two')
  recommend('three')
  await setImmediate()
  requests[0].reject(new Error('Fixture upstream unavailable'))
  await setImmediate()
  assert.deepEqual(
    requests.map(request => request.id),
    ['one', 'two', 'three']
  )
  assert.equal(client.getQueryState(discoverKeys.movie('one')).status, 'error')
  recommend('one')
  await setImmediate()
  assert.equal(requests.length, 3)
  loader.prefetch(client, 'one')
  requests[3].resolve(movie('one'))
  await setImmediate()
  assert.equal(client.getQueryState(discoverKeys.movie('one')).status, 'success')
})

test('normal foreground detail requests remain cancelable', async t => {
  const { observe, requests } = fixture(t)
  const detail = observe('one')
  assert.equal(requests.length, 1)
  assert.equal(requests[0].signal.aborted, false)
  detail.destroy()
  assert.equal(requests[0].signal.aborted, true)
  await setImmediate()
})

test('brief intersections do not enqueue work and leaving or unmounting releases it', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  let observer
  const original = Object.getOwnPropertyDescriptor(globalThis, 'IntersectionObserver')
  globalThis.IntersectionObserver = class {
    constructor(notify, options) {
      this.notify = notify
      this.options = options
      observer = this
    }
    observe(element) {
      this.element = element
    }
    disconnect() {
      this.disconnected = true
    }
    enter(visible) {
      this.notify([{ isIntersecting: visible }])
    }
  }
  t.after(() => {
    if (original) Object.defineProperty(globalThis, 'IntersectionObserver', original)
    else delete globalThis.IntersectionObserver
  })
  let requested = 0
  let released = 0
  const element = {}
  const dispose = observeRecommendation(element, () => {
    requested++
    return () => released++
  })
  assert.equal(observer.element, element)
  observer.enter(true)
  t.mock.timers.tick(199)
  observer.enter(false)
  t.mock.timers.tick(1000)
  assert.equal(requested, 0)

  observer.enter(true)
  t.mock.timers.tick(200)
  assert.equal(requested, 1)
  observer.enter(true)
  t.mock.timers.tick(200)
  assert.equal(requested, 1)
  observer.enter(false)
  assert.equal(released, 1)

  observer.enter(true)
  t.mock.timers.tick(200)
  dispose()
  assert.equal(released, 2)
  assert.equal(observer.disconnected, true)
  t.mock.timers.tick(1000)
  assert.equal(requested, 2)
})
