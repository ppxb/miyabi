import { QueryClient, QueryObserver, type QueryKey } from '@tanstack/react-query'
import { beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { cancelQueryRefresh, refreshQueries } from '@/api/query-refresh'

beforeEach(() => vi.useFakeTimers())

function fixture(key: QueryKey = ['offline'], seed = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const reads: (PromiseWithResolvers<number> & { signal: AbortSignal })[] = []
  if (seed) client.setQueryData(key, 0)
  const observer = new QueryObserver(client, {
    queryKey: key,
    staleTime: Infinity,
    queryFn: ({ signal }) => {
      const read = { ...Promise.withResolvers<number>(), signal }
      reads.push(read)
      return read.promise
    }
  })
  const stop = observer.subscribe(() => {})
  onTestFinished(() => {
    stop()
    client.clear()
  })
  return { client, reads, key }
}

test('a burst shares one read and each query client has its own queue', async () => {
  const a = fixture()
  const b = fixture()
  const first = refreshQueries(a.client, a.key, a.key)
  const repeated = refreshQueries(a.client, a.key)
  const other = refreshQueries(b.client, b.key)
  await vi.advanceTimersByTimeAsync(0)
  expect(a.reads).toHaveLength(1)
  expect(b.reads).toHaveLength(1)
  a.reads[0]!.resolve(1)
  b.reads[0]!.resolve(2)
  await Promise.all([first, repeated, other])
  expect(a.client.getQueryData(a.key)).toBe(1)
  expect(b.client.getQueryData(b.key)).toBe(2)
})

test('changes during a read share one following read without cancelling the current one', async () => {
  const f = fixture()
  const first = refreshQueries(f.client, f.key)
  await vi.advanceTimersByTimeAsync(0)
  const following = Array.from({ length: 12 }, () => refreshQueries(f.client, f.key))
  await vi.advanceTimersByTimeAsync(0)
  expect(f.reads).toHaveLength(1)
  expect(f.reads[0]!.signal.aborted).toBe(false)
  f.reads[0]!.resolve(1)
  await first // This caller need not wait for later changes to settle.
  await vi.advanceTimersByTimeAsync(0)
  expect(f.reads).toHaveLength(2)
  f.reads[1]!.resolve(2)
  await Promise.all(following)
  expect(f.client.getQueryData(f.key)).toBe(2)
})

test('a change discards an initial read started before it, even without cached data', async () => {
  const f = fixture(['offline'], false)
  const changed = refreshQueries(f.client, f.key)
  await vi.advanceTimersByTimeAsync(0)
  expect(f.reads[0]!.signal.aborted).toBe(true)
  expect(f.reads).toHaveLength(2)
  f.reads[1]!.resolve(2)
  await changed
  f.reads[0]!.resolve(1)
  await vi.advanceTimersByTimeAsync(0)
  expect(f.client.getQueryData(f.key)).toBe(2)
})

test('an SSE snapshot cancels queued work but a later change still refreshes', async () => {
  const f = fixture(['tasks'])
  const first = refreshQueries(f.client, f.key)
  await vi.advanceTimersByTimeAsync(0)
  const queued = refreshQueries(f.client, f.key)
  await cancelQueryRefresh(f.client, f.key)
  f.client.setQueryData(f.key, 2)
  await Promise.all([first, queued])
  expect(f.reads[0]!.signal.aborted).toBe(true)
  expect(f.reads).toHaveLength(1)
  f.reads[0]!.resolve(1)
  await vi.advanceTimersByTimeAsync(0)
  expect(f.client.getQueryData(f.key)).toBe(2)

  const stale = refreshQueries(f.client, f.key)
  const snapshot = cancelQueryRefresh(f.client, f.key)
  const latest = refreshQueries(f.client, f.key)
  await snapshot
  await vi.advanceTimersByTimeAsync(0)
  expect(f.reads).toHaveLength(2)
  f.reads[1]!.resolve(3)
  await Promise.all([stale, latest])
  expect(f.client.getQueryData(f.key)).toBe(3)
})

test('a failed refresh releases the queue and later changes can recover', async () => {
  const f = fixture()
  const first = refreshQueries(f.client, f.key)
  await vi.advanceTimersByTimeAsync(0)
  f.reads[0]!.reject(new Error('temporary failure'))
  await first
  expect(f.client.getQueryState(f.key)?.status).toBe('error')
  const retry = refreshQueries(f.client, f.key)
  await vi.advanceTimersByTimeAsync(0)
  expect(f.reads).toHaveLength(2)
  f.reads[1]!.resolve(2)
  await retry
  expect(f.client.getQueryData(f.key)).toBe(2)
})
