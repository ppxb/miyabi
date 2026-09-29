import assert from 'node:assert/strict'
import { test, onTestFinished, vi } from 'vitest'
import { setImmediate } from 'node:timers/promises'
import { QueryClient, QueryObserver } from '@tanstack/react-query'

import {
  createMovieDetailLoader,
  discoverKeys,
  findCachedMovieCard,
  subscribeMovieCard
} from '@/api/movie-detail-cache'
import type { DiscoverMovieDetail } from '@/api/discover'
import { observeRecommendation } from '@/features/movie-detail/recommendation-visibility'

function movie(id: string, title = `Title ${id}`): DiscoverMovieDetail {
  return {
    id,
    code: `ABP-${id}`,
    title,
    cover: `https://media.example/${id}.jpg`,
    origin_title: title,
    release_date: '',
    duration: 0,
    rating: 0,
    thumbnail: '',
    preview_images: [],
    preview_video: '',
    magnets_count: 0,
    has_subtitle: false,
    has_preview: false,
    actors: [],
    tags: [],
    state: 'not_in_library',
    release_status: 'unknown',
    zone: 'censored',
    actor_movies: [],
    related_movies: []
  }
}

function fixture() {
  const client = new QueryClient({ defaultOptions: { queries: { gcTime: Infinity } } })
  const requests: ({
    id: string
    signal?: AbortSignal
  } & PromiseWithResolvers<DiscoverMovieDetail>)[] = []
  const releases: (() => void)[] = []
  const loader = createMovieDetailLoader((id, signal) => {
    const pending = Promise.withResolvers<DiscoverMovieDetail>()
    signal?.addEventListener('abort', () => pending.reject(signal.reason), { once: true })
    requests.push({ id, signal, ...pending })
    return pending.promise
  })
  onTestFinished(() => {
    for (const release of releases) release()
    client.clear()
  })
  return {
    client,
    loader,
    requests,
    recommend(id: string) {
      const release = loader.request(client, id)
      releases.push(release)
      return release
    },
    observe(id: string, enabled = true) {
      const observer = new QueryObserver(client, { ...loader.options(id), enabled })
      observer.subscribe(() => {})
      releases.push(() => observer.destroy())
      return observer
    }
  }
}

test('unseen cards read cache without starting detail requests', async () => {
  const { observe, requests } = fixture()
  for (let id = 0; id < 16; id++) observe(String(id), false)
  await setImmediate()
  assert.equal(requests.length, 0)
})

test('both recommendation groups share two background slots and cancel unstarted work', async () => {
  const { recommend, requests } = fixture()
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
  requests[0]!.resolve(movie('one'))
  await setImmediate()
  assert.deepEqual(
    requests.map(request => request.id),
    ['one', 'two', 'four']
  )
  requests[1]!.resolve(movie('two'))
  requests[2]!.resolve(movie('four'))
  await setImmediate()
  assert.equal(requests.length, 3)
})

test('browsed and searched cards avoid detail requests without populating full-detail cache', async () => {
  const { client, loader, recommend, requests } = fixture()
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
  const full: DiscoverMovieDetail = {
    ...browsed,
    actor_movies: [],
    related_movies: [],
    zone: 'censored'
  }
  requests[0]!.resolve(full)
  await setImmediate()
  assert.deepEqual(client.getQueryData(discoverKeys.movie('one')), full)
})

test('stale card data remains usable while details refresh and unrelated caches are ignored', async () => {
  const { client, recommend, requests } = fixture()
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
  requests[0]!.resolve(movie('one', 'Updated title'))
  await setImmediate()
  assert.equal(findCachedMovieCard(client, 'one')?.title, 'Updated title')
})

test('queued cards recheck newly available list data before using an upstream slot', async () => {
  const { client, recommend, requests } = fixture()
  recommend('one')
  recommend('two')
  recommend('three')
  await setImmediate()
  client.setQueryData(discoverKeys.search({ query: 'ABP-3' }), [movie('three')])
  requests[0]!.resolve(movie('one'))
  requests[1]!.resolve(movie('two'))
  await setImmediate()
  assert.deepEqual(
    requests.map(request => request.id),
    ['one', 'two']
  )
})

test('clicking a queued recommendation bypasses background work and reuses the request after navigation', async () => {
  const { client, loader, recommend, requests, observe } = fixture()
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
  requests[2]!.resolve(full)
  await setImmediate()
  assert.deepEqual(detail.getCurrentResult().data, full)
  assert.equal(requests.filter(request => request.id === 'four').length, 1)
})

test('a click before visibility loading also survives the observer gap', async () => {
  const { client, loader, requests, observe } = fixture()
  const card = observe('one', false)
  loader.prefetch(client, 'one')
  card.destroy()
  await setImmediate()
  const detail = observe('one')
  requests[0]!.resolve(movie('one'))
  await setImmediate()
  assert.equal(requests.length, 1)
  assert.equal(detail.getCurrentResult().data?.title, 'Title one')
})

test('a failed card releases its slot and retries only on explicit demand', async () => {
  const { client, loader, recommend, requests } = fixture()
  recommend('one')
  recommend('two')
  recommend('three')
  await setImmediate()
  requests[0]!.reject(new Error('Fixture upstream unavailable'))
  await setImmediate()
  assert.deepEqual(
    requests.map(request => request.id),
    ['one', 'two', 'three']
  )
  assert.equal(client.getQueryState(discoverKeys.movie('one'))?.status, 'error')
  recommend('one')
  await setImmediate()
  assert.equal(requests.length, 3)
  loader.prefetch(client, 'one')
  requests[3]!.resolve(movie('one'))
  await setImmediate()
  assert.equal(client.getQueryState(discoverKeys.movie('one'))?.status, 'success')
})

test('normal foreground detail requests remain cancelable', async () => {
  const { observe, requests } = fixture()
  const detail = observe('one')
  assert.equal(requests.length, 1)
  assert.equal(requests[0]!.signal?.aborted, false)
  detail.destroy()
  assert.equal(requests[0]!.signal?.aborted, true)
  await setImmediate()
})

test('brief intersections do not enqueue work and leaving or unmounting releases it', () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  const observers: MockIntersectionObserver[] = []
  class MockIntersectionObserver {
    element?: Element
    disconnected = false
    constructor(
      private notify: (entries: Pick<IntersectionObserverEntry, 'isIntersecting'>[]) => void
    ) {
      observers.push(this)
    }
    observe(element: Element) {
      this.element = element
    }
    disconnect() {
      this.disconnected = true
    }
    enter(visible: boolean) {
      this.notify([{ isIntersecting: visible }])
    }
  }
  vi.stubGlobal('IntersectionObserver', MockIntersectionObserver)
  let requested = 0
  let released = 0
  // The observer only uses element identity; no DOM methods are invoked.
  const element = {} as Element
  const dispose = observeRecommendation(element, () => {
    requested++
    return () => released++
  })
  const observer = observers[0]
  assert.ok(observer)
  assert.equal(observer.element, element)
  observer.enter(true)
  vi.advanceTimersByTime(199)
  observer.enter(false)
  vi.advanceTimersByTime(1000)
  assert.equal(requested, 0)

  observer.enter(true)
  vi.advanceTimersByTime(200)
  assert.equal(requested, 1)
  observer.enter(true)
  vi.advanceTimersByTime(200)
  assert.equal(requested, 1)
  observer.enter(false)
  assert.equal(released, 1)

  observer.enter(true)
  vi.advanceTimersByTime(200)
  dispose()
  assert.equal(released, 2)
  assert.equal(observer.disconnected, true)
  vi.advanceTimersByTime(1000)
  assert.equal(requested, 2)
})

test('card lookup follows list replacement and removal without retaining orphaned entries', () => {
  const { client } = fixture()
  const older = movie('one', 'Older search result')
  const newer = movie('one', 'Newer list result')
  client.setQueryData(discoverKeys.search({ query: 'one' }), [older], { updatedAt: 100 })
  client.setQueryData(discoverKeys.movies({ page: 1 }), [newer], { updatedAt: 200 })
  assert.equal(findCachedMovieCard(client, 'one'), newer)
  client.setQueryData(discoverKeys.movies({ page: 1 }), [movie('two')])
  assert.equal(findCachedMovieCard(client, 'one'), older)
  client.removeQueries({ queryKey: discoverKeys.search({ query: 'one' }) })
  assert.equal(findCachedMovieCard(client, 'one'), undefined)
})

test('invalidated card data stays displayable but no longer suppresses a detail request', async () => {
  const { client, recommend, requests } = fixture()
  const card = movie('one')
  client.setQueryData(discoverKeys.movies({ page: 1 }), [card])
  await client.invalidateQueries({ queryKey: discoverKeys.movies({ page: 1 }) })
  assert.equal(findCachedMovieCard(client, 'one'), card)
  assert.equal(findCachedMovieCard(client, 'one', true), undefined)
  recommend('one')
  await setImmediate()
  assert.equal(requests.length, 1)
  requests[0]!.resolve(movie('one'))
})

test('card subscriptions follow relevant cache changes and unsubscribe on disposal', () => {
  const { client } = fixture()
  const notify = vi.fn()
  const stop = subscribeMovieCard(client, 'one', notify)
  onTestFinished(stop)
  const card = movie('one')
  client.setQueryData(discoverKeys.search({ query: 'one' }), [card])
  assert.equal(notify.mock.calls.length, 1)
  assert.equal(findCachedMovieCard(client, 'one'), card)
  client.setQueryData(discoverKeys.magnets('one'), [])
  client.setQueryData(discoverKeys.movie('two'), movie('two'))
  client.removeQueries({ queryKey: discoverKeys.magnets('one') })
  client.removeQueries({ queryKey: discoverKeys.movie('two') })
  assert.equal(notify.mock.calls.length, 1)
  client.removeQueries({ queryKey: discoverKeys.search({ query: 'one' }) })
  assert.equal(notify.mock.calls.length, 2)
  assert.equal(findCachedMovieCard(client, 'one'), undefined)
  stop()
  client.setQueryData(discoverKeys.movie('one'), card)
  assert.equal(notify.mock.calls.length, 2)
})
