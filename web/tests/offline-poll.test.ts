import assert from 'node:assert/strict'
import { onTestFinished, test, vi } from 'vitest'
import { QueryClient, QueryObserver } from '@tanstack/react-query'
import { offlineActivityOptions } from '@/api/offline'
import type { OfflineActivity } from '@/api/offline'

// QueryObserver must detect a browser before the query module is evaluated.
vi.hoisted(() => vi.stubGlobal('window', {}))

function activity(phase: 'downloading' | 'available'): OfflineActivity {
  return {
    tasks: [
      {
        task_id: 1,
        code: 'ABP-001',
        account_id: 'acc',
        directory_id: 'dir',
        hash: 'hash',
        status: 'running',
        phase,
        processing: false,
        progress: 10
      }
    ]
  }
}

function observeActivity(poll: boolean, phase: 'downloading' | 'available') {
  const client = new QueryClient()
  const observer = new QueryObserver(client, offlineActivityOptions(poll))
  const fetch = vi.fn().mockImplementation(() => Promise.resolve(Response.json(activity(phase))))
  vi.stubGlobal('fetch', fetch)
  vi.useFakeTimers()
  vi.stubGlobal('window', { location: { origin: 'http://localhost' } })
  const stop = observer.subscribe(() => {})
  onTestFinished(() => {
    stop()
    client.clear()
  })
  return { observer, fetch }
}

test('a connected client does not poll active offline tasks', async () => {
  const { observer, fetch } = observeActivity(false, 'downloading')
  await vi.advanceTimersByTimeAsync(16_000)
  assert.equal(fetch.mock.calls.length, 1)
  assert.equal(observer.getCurrentResult().data?.tasks[0]?.phase, 'downloading')
})

test('a disconnected client polls active offline tasks every five seconds', async () => {
  const { fetch } = observeActivity(true, 'downloading')
  await vi.advanceTimersByTimeAsync(16_000)
  assert.equal(fetch.mock.calls.length, 4)
})

test('idle offline tasks are never polled', async () => {
  const { fetch } = observeActivity(true, 'available')
  await vi.advanceTimersByTimeAsync(16_000)
  assert.equal(fetch.mock.calls.length, 1)
})
