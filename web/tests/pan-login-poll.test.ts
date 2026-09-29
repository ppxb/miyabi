import assert from 'node:assert/strict'
import { beforeEach, onTestFinished, test, vi } from 'vitest'
import { QueryClient, QueryObserver } from '@tanstack/react-query'
import { panLoginOptions } from '@/api/pan'

// QueryObserver must detect a browser before the query module is evaluated.
vi.hoisted(() => vi.stubGlobal('window', {}))
beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('window', { location: { origin: 'http://localhost' } })
})

function observeLogin(id = 'fixture') {
  const client = new QueryClient()
  const observer = new QueryObserver(client, panLoginOptions(id))
  const seen: ReturnType<typeof observer.getCurrentResult>[] = []
  const stop = observer.subscribe(result => seen.push(result))
  onTestFinished(() => {
    stop()
    client.clear()
  })
  return { observer, seen }
}

test('an unreachable backend stops after five failures without further polling', async () => {
  const fetch = vi.fn().mockRejectedValue(new TypeError('connection refused'))
  vi.stubGlobal('fetch', fetch)
  const { observer, seen } = observeLogin()
  await vi.advanceTimersByTimeAsync(6000)
  assert.equal(fetch.mock.calls.length, 5)
  assert.equal(observer.getCurrentResult().isError, true)
  assert.equal(observer.getCurrentResult().failureCount, 5)
  assert.ok(seen.some(result => !result.isError && result.failureCount > 0))
  await vi.advanceTimersByTimeAsync(30_000)
  assert.equal(fetch.mock.calls.length, 5)
})

test('transient failures keep the scanned answer and resume polling', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ state: 'scanned' }))
    .mockRejectedValueOnce(new TypeError('connection reset'))
    .mockRejectedValueOnce(new TypeError('connection reset'))
    .mockImplementation(() => Promise.resolve(Response.json({ state: 'scanned' })))
  vi.stubGlobal('fetch', fetch)
  const { observer, seen } = observeLogin()
  await vi.advanceTimersByTimeAsync(4500)
  assert.equal(fetch.mock.calls.length, 4)
  assert.ok(seen.every(result => !result.isError))
  const retrying = seen.filter(result => result.failureCount > 0)
  assert.equal(Math.max(...retrying.map(result => result.failureCount)), 2)
  assert.ok(retrying.every(result => result.data?.state === 'scanned'))
  assert.equal(observer.getCurrentResult().failureCount, 0)
  await vi.advanceTimersByTimeAsync(1500)
  assert.equal(fetch.mock.calls.length, 5)
})

for (const state of ['authorized', 'expired', 'canceled'] as const) {
  test(`${state} stops further login polling`, async () => {
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(Response.json({ state })))
    vi.stubGlobal('fetch', fetch)
    const { observer } = observeLogin()
    await vi.advanceTimersByTimeAsync(30_000)
    assert.equal(observer.getCurrentResult().data?.state, state)
    assert.equal(fetch.mock.calls.length, 1)
  })
}

test('waiting polls advance to scanned and authorized using the real endpoint', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ state: 'waiting' }))
    .mockResolvedValueOnce(Response.json({ state: 'scanned' }))
    .mockResolvedValueOnce(Response.json({ state: 'authorized' }))
  vi.stubGlobal('fetch', fetch)
  const { observer } = observeLogin('qr/id')
  await vi.advanceTimersByTimeAsync(0)
  assert.equal(observer.getCurrentResult().data?.state, 'waiting')
  assert.equal(fetch.mock.calls[0]![0], '/api/pan/login/qr%2Fid')
  await vi.advanceTimersByTimeAsync(1500)
  assert.equal(observer.getCurrentResult().data?.state, 'scanned')
  await vi.advanceTimersByTimeAsync(1500)
  assert.equal(observer.getCurrentResult().data?.state, 'authorized')
  await vi.advanceTimersByTimeAsync(30_000)
  assert.equal(fetch.mock.calls.length, 3)
})

test('an empty session ID never starts a login request', async () => {
  const fetch = vi.fn()
  vi.stubGlobal('fetch', fetch)
  observeLogin('')
  await vi.advanceTimersByTimeAsync(30_000)
  assert.equal(fetch.mock.calls.length, 0)
})
