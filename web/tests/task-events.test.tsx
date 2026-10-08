import { useEffect } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider, QueryObserver } from '@tanstack/react-query'
import { beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { apiGet } from '@/api/client'
import { libraryKeys } from '@/api/library'
import { movieStateKeys } from '@/api/movie-states'
import { offlineKeys } from '@/api/offline'
import { subscriptionKeys } from '@/api/subscriptions'
import { taskKeys, type Task } from '@/api/tasks'
import { TaskEventsProvider, useTaskConnection } from '@/features/tasks/task-events'

// Capture the production effect so Node tests can drive its setup/cleanup without a browser.
vi.mock('react', async importOriginal => ({
  ...(await importOriginal<typeof import('react')>()),
  useEffect: vi.fn()
}))

class EventSourceStub {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2
  static instances: EventSourceStub[] = []
  readyState = EventSourceStub.CONNECTING
  onerror?: () => void
  listeners = new Map<string, (event: MessageEvent<string>) => void | Promise<void>>()

  constructor(readonly url: string) {
    EventSourceStub.instances.push(this)
  }

  addEventListener(type: string, listener: (event: MessageEvent<string>) => void | Promise<void>) {
    this.listeners.set(type, listener)
  }

  async send(type: string, data: unknown) {
    this.readyState = EventSourceStub.OPEN
    await this.listeners.get(type)?.({ data: JSON.stringify(data) } as MessageEvent<string>)
  }

  fail() {
    this.readyState = EventSourceStub.CONNECTING
    this.onerror?.()
  }

  close() {
    this.readyState = EventSourceStub.CLOSED
  }
}

const running: Task = {
  id: 1,
  type: 'subscription_batch',
  status: 'running',
  progress: 0,
  created_at: '',
  updated_at: '',
  batch: { total: 1, processed: 0, submitted: 0, waiting: 0, failed: 0, failures: [] }
}
const done: Task = { ...running, status: 'done', progress: 100 }

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  vi.stubGlobal('window', { location: { origin: 'http://localhost' } })
  vi.stubGlobal('EventSource', EventSourceStub)
  EventSourceStub.instances = []
})

function fixture() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(taskKeys.all, [running])
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(async path => {
    if (path === '/api/tasks/events') return new Response(null, { status: 200 })
    expect(path).toBe('/api/tasks')
    return Response.json([done])
  })
  vi.stubGlobal('fetch', fetch)
  const observer = new QueryObserver(client, {
    queryKey: taskKeys.all,
    queryFn: ({ signal }) => apiGet<Task[]>('/api/tasks', undefined, signal),
    staleTime: Infinity
  })
  const unsubscribe = observer.subscribe(() => {})
  let reconnect!: () => void
  function CaptureConnection() {
    const connection = useTaskConnection()
    useEffect(() => {
      reconnect = connection.reconnect
    }, [connection.reconnect])
    return null
  }
  renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <TaskEventsProvider>
        <CaptureConnection />
      </TaskEventsProvider>
    </QueryClientProvider>
  )
  const setup = vi.mocked(useEffect).mock.calls.at(-2)![0]
  vi.mocked(useEffect).mock.calls.at(-1)![0]()
  let cleanup = setup()
  const dispose = () => {
    if (cleanup) cleanup()
    cleanup = undefined
  }
  onTestFinished(() => {
    dispose()
    unsubscribe()
    client.clear()
  })
  return {
    client,
    fetch,
    dispose,
    get events() {
      return EventSourceStub.instances.at(-1)!
    },
    taskReads: () => fetch.mock.calls.filter(([path]) => path === '/api/tasks').length,
    reconnect() {
      reconnect()
      // React reruns this effect after the attempt changes; its refs survive that transition.
      dispose()
      cleanup = setup()
    }
  }
}

test('manual reconnect accepts the SSE snapshot without an additional HTTP task read', async () => {
  const f = fixture()
  await f.events.send('tasks', [running])
  f.events.fail()
  await vi.advanceTimersByTimeAsync(0)
  expect(f.taskReads()).toBe(1)
  f.reconnect()
  expect(f.taskReads()).toBe(1)
  await f.events.send('tasks', [done])
  await vi.advanceTimersByTimeAsync(15_000)
  expect(f.taskReads()).toBe(1)
  expect(f.client.getQueryData(taskKeys.all)).toEqual([done])
})

test('a silent manual reconnect falls back to HTTP once at the connection timeout', async () => {
  const f = fixture()
  await f.events.send('tasks', [running])
  f.events.fail()
  await vi.advanceTimersByTimeAsync(0)
  f.reconnect()
  await vi.advanceTimersByTimeAsync(14_999)
  expect(f.taskReads()).toBe(1)
  await vi.advanceTimersByTimeAsync(1)
  expect(f.taskReads()).toBe(2)
  expect(f.client.getQueryData(taskKeys.all)).toEqual([done])
})

test('a failed manual reconnect can request a fresh fallback, once per outage', async () => {
  const f = fixture()
  await f.events.send('tasks', [running])
  f.events.fail()
  await vi.advanceTimersByTimeAsync(0)
  f.reconnect()
  f.events.fail()
  await vi.advanceTimersByTimeAsync(0)
  expect(f.taskReads()).toBe(2)
  f.events.fail()
  await vi.advanceTimersByTimeAsync(0)
  expect(f.taskReads()).toBe(2)
})

test('an older HTTP fallback cannot overwrite a reconnected SSE snapshot', async () => {
  const f = fixture()
  const response = Promise.withResolvers<Response>()
  f.fetch.mockImplementation(() => response.promise)
  await f.events.send('tasks', [running])
  f.events.fail()
  const signal = f.fetch.mock.calls[0]![1]?.signal
  expect(signal?.aborted).toBe(false)
  f.reconnect()
  await f.events.send('tasks', [done])
  expect(signal?.aborted).toBe(true)
  response.resolve(Response.json([running]))
  await vi.advanceTimersByTimeAsync(0)
  expect(f.taskReads()).toBe(1)
  expect(f.client.getQueryData(taskKeys.all)).toEqual([done])
})

test('initial and reconnected revisions still reconcile business data', async () => {
  const f = fixture()
  const keys = [movieStateKeys.all, offlineKeys.all, libraryKeys.all, subscriptionKeys.all]
  for (const key of keys) f.client.setQueryData(key, [])
  const revisions = { library: 0, offline: 0, monitor: 0 }
  await f.events.send('changes', revisions)
  await vi.advanceTimersByTimeAsync(0)
  for (const key of keys) {
    expect(f.client.getQueryState(key)?.isInvalidated).toBe(true)
    f.client.setQueryData(key, [])
  }
  await f.events.send('changes', revisions)
  await vi.advanceTimersByTimeAsync(0)
  for (const key of keys) expect(f.client.getQueryState(key)?.isInvalidated).toBe(false)
  f.reconnect()
  await f.events.send('changes', revisions)
  await vi.advanceTimersByTimeAsync(0)
  for (const key of keys) expect(f.client.getQueryState(key)?.isInvalidated).toBe(true)
})

test('cleanup removes the pending connection timeout', async () => {
  const f = fixture()
  f.reconnect()
  f.dispose()
  await vi.advanceTimersByTimeAsync(60_000)
  expect(f.fetch).not.toHaveBeenCalled()
})
