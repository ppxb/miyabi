import assert from 'node:assert/strict'
import { setImmediate } from 'node:timers/promises'
import { QueryClient, QueryObserver } from '@tanstack/react-query'
import { onTestFinished, test, vi } from 'vitest'

import {
  browseHistoryKeys,
  recordMovieViewOptions,
  viewedMoviesOptions
} from '@/api/browse-history'

function fixture() {
  vi.stubGlobal('window', { location: { origin: 'http://localhost' }, dispatchEvent: vi.fn() })
  const client = new QueryClient({
    defaultOptions: { queries: { gcTime: Infinity, retry: false } }
  })
  onTestFinished(() => client.clear())
  return client
}

function record(client: QueryClient, id: string) {
  const mutation = client
    .getMutationCache()
    .build(client, { ...recordMovieViewOptions(client, id), retryDelay: 0 })
  return { mutation, finished: mutation.execute(undefined) }
}

function observe(client: QueryClient) {
  const observer = new QueryObserver(client, viewedMoviesOptions)
  const unsubscribe = observer.subscribe(() => {})
  onTestFinished(unsubscribe)
  return observer
}

test('cards share the viewed query instead of fetching once per card', async () => {
  const client = fixture()
  const response = Promise.withResolvers<Response>()
  const fetch = vi.fn(() => response.promise)
  vi.stubGlobal('fetch', fetch)
  const first = observe(client)
  const second = observe(client)
  assert.equal(fetch.mock.calls.length, 1)
  response.resolve(Response.json(['one']))
  await setImmediate()
  assert.deepEqual(first.getCurrentResult().data, ['one'])
  assert.deepEqual(second.getCurrentResult().data, ['one'])
})

test('a view posts immediately and is pending until the server confirms it', async () => {
  const client = fixture()
  client.setQueryData(browseHistoryKeys.viewed, ['old'])
  const response = Promise.withResolvers<Response>()
  const fetch = vi.fn(() => response.promise)
  vi.stubGlobal('fetch', fetch)
  const { mutation, finished } = record(client, 'new')
  await setImmediate()
  assert.equal(client.isMutating({ mutationKey: browseHistoryKeys.record('new'), exact: true }), 1)
  assert.deepEqual(client.getQueryData(browseHistoryKeys.viewed), ['old'])
  assert.equal(fetch.mock.calls.length, 1)
  const [url, options] = fetch.mock.calls[0] as unknown as [string, RequestInit]
  assert.equal(url, '/api/discover/viewed')
  assert.equal(options.method, 'POST')
  assert.deepEqual(JSON.parse(options.body as string), { ids: ['new'] })
  response.resolve(Response.json(null))
  await finished
  assert.equal(mutation.state.status, 'success')
  assert.deepEqual(client.getQueryData(browseHistoryKeys.viewed), ['new', 'old'])
})

test('a failed optimistic view does not remove a concurrent successful view', async () => {
  const client = fixture()
  client.setQueryData(browseHistoryKeys.viewed, ['old'])
  const firstResponse = Promise.withResolvers<Response>()
  vi.stubGlobal(
    'fetch',
    vi.fn((_url: string, options: RequestInit) => {
      const { ids } = JSON.parse(options.body as string)
      return ids[0] === 'first' ? firstResponse.promise : Promise.resolve(Response.json(null))
    })
  )
  const first = record(client, 'first')
  const failure = assert.rejects(first.finished)
  const second = record(client, 'second')
  await second.finished
  assert.equal(first.mutation.state.status, 'pending')
  firstResponse.resolve(Response.json({ error: 'invalid' }, { status: 400 }))
  await failure
  assert.equal(first.mutation.state.status, 'error')
  assert.deepEqual(client.getQueryData(browseHistoryKeys.viewed), ['second', 'old'])
})

test('a first GET preserves existing server history and concurrent confirmed writes', async () => {
  const client = fixture()
  const stale = Promise.withResolvers<Response>()
  let signal: AbortSignal | null | undefined
  vi.stubGlobal(
    'fetch',
    vi.fn((_url: string, options: RequestInit) => {
      if (options.method === 'POST') return Promise.resolve(Response.json(null))
      signal = options.signal
      return stale.promise
    })
  )
  const read = client.fetchQuery(viewedMoviesOptions).catch(() => undefined)
  await record(client, 'new').finished
  assert.equal(signal?.aborted, false)
  stale.resolve(Response.json(['old']))
  await read
  await setImmediate()
  assert.deepEqual(client.getQueryData(browseHistoryKeys.viewed), ['new', 'old'])
})

test('writes avoid full-list requests and preserve staleness until the next page visit', async () => {
  const client = fixture()
  const updatedAt = Date.now() - 30_000
  client.setQueryData(browseHistoryKeys.viewed, ['old', 'evicted'], { updatedAt })
  const fetch = vi.fn((_url: string, options: RequestInit) =>
    Promise.resolve(Response.json(options.method === 'POST' ? null : ['new', 'old']))
  )
  vi.stubGlobal('fetch', fetch)
  observe(client)
  await record(client, 'new').finished
  assert.equal(fetch.mock.calls.length, 1)
  assert.equal(fetch.mock.calls[0]![1].method, 'POST')
  assert.deepEqual(client.getQueryData(browseHistoryKeys.viewed), ['new', 'old', 'evicted'])
  assert.equal(client.getQueryState(browseHistoryKeys.viewed)?.dataUpdatedAt, updatedAt)
  observe(client)
  assert.equal(fetch.mock.calls.length, 1)
  vi.spyOn(Date, 'now').mockReturnValue(updatedAt + 61_000)
  observe(client)
  await setImmediate()
  assert.equal(fetch.mock.calls.length, 2)
  assert.deepEqual(client.getQueryData(browseHistoryKeys.viewed), ['new', 'old'])
})

test('transient writes retry but rejected requests do not', async () => {
  const client = fixture()
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new TypeError('offline'))
    .mockResolvedValueOnce(Response.json({ error: 'busy' }, { status: 503 }))
    .mockResolvedValueOnce(Response.json(null))
  vi.stubGlobal('fetch', fetch)
  await record(client, 'retried').finished
  assert.equal(fetch.mock.calls.length, 3)
  fetch.mockReset().mockResolvedValue(Response.json({ error: 'invalid' }, { status: 400 }))
  await assert.rejects(record(client, 'rejected').finished)
  assert.equal(fetch.mock.calls.length, 1)
  assert.deepEqual(client.getQueryData(browseHistoryKeys.viewed), ['retried'])
})

test('initial history lookup can recover after a transient failure', async () => {
  const client = fixture()
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new TypeError('offline'))
    .mockResolvedValueOnce(Response.json(['saved']))
  vi.stubGlobal('fetch', fetch)
  const data = await client.fetchQuery({ ...viewedMoviesOptions, retryDelay: 0 })
  assert.deepEqual(data, ['saved'])
  assert.equal(fetch.mock.calls.length, 2)
})

test('local confirmed views stay bounded and deduplicated without refreshing', async () => {
  const client = fixture()
  client.setQueryData(
    browseHistoryKeys.viewed,
    Array.from({ length: 5000 }, (_, i) => String(i))
  )
  const fetch = vi.fn(() => Promise.resolve(Response.json(null)))
  vi.stubGlobal('fetch', fetch)
  await record(client, 'new').finished
  await record(client, 'new').finished
  const ids = client.getQueryData<string[]>(browseHistoryKeys.viewed)!
  assert.equal(ids.length, 5000)
  assert.equal(ids[0], 'new')
  assert.equal(ids.includes('4999'), false)
  assert.equal(fetch.mock.calls.length, 2)
})
