import { useEffect } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import {
  QueryClient,
  QueryClientProvider,
  QueryObserver,
  useMutation,
  type UseMutationOptions
} from '@tanstack/react-query'
import { beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { apiGet } from '@/api/client'
import * as clientAPI from '@/api/client'
import { libraryKeys, useStartLibraryScan, useRescrapeLibraryMovie } from '@/api/library'
import { movieStateKeys } from '@/api/movie-states'
import { offlineKeys, useAddOffline, useOfflineControl, type OfflineActivity } from '@/api/offline'
import { panKeys } from '@/api/pan'
import { subscriptionKeys } from '@/api/subscriptions'
import { taskKeys, type Task, type ScanTask } from '@/api/tasks'
import { TaskEventsProvider, useTaskConnection } from '@/features/tasks/task-events'
import { offlineSubmission } from './fixtures'

// Exercise production stream callbacks and disposal; React effect scheduling is outside this fixture.
vi.mock('react', async importOriginal => ({
  ...(await importOriginal<typeof import('react')>()),
  useEffect: vi.fn()
}))

vi.mock('@tanstack/react-query', async importOriginal => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useMutation: vi.fn()
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
  const captureConnection = vi.fn<(connection: ReturnType<typeof useTaskConnection>) => void>()
  function CaptureConnection() {
    captureConnection(useTaskConnection())
    return null
  }
  renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <TaskEventsProvider>
        <CaptureConnection />
      </TaskEventsProvider>
    </QueryClientProvider>
  )
  // Identify the connection effect by its query client, independent of other effects' order.
  const connectionEffects = vi
    .mocked(useEffect)
    .mock.calls.filter(([, deps]) => deps?.includes(client))
  expect(connectionEffects).toHaveLength(1)
  const setup = connectionEffects[0]![0]
  const { reconnect } = captureConnection.mock.calls.at(-1)![0]
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
  await vi.advanceTimersByTimeAsync(0)
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

test('initial revisions reconcile once, same-instance reconnects stay quiet, and a restart reconciles again', async () => {
  const f = fixture()
  const keys = [movieStateKeys.all, offlineKeys.all, libraryKeys.all, subscriptionKeys.all]
  for (const key of keys) f.client.setQueryData(key, [])
  const revisions = { library: 0, offline: 0, monitor: 0, instance: 'server-a' }
  await f.events.send('changes', revisions)
  await vi.advanceTimersByTimeAsync(0)
  for (const key of keys) {
    expect(f.client.getQueryState(key)?.isInvalidated).toBe(true)
    f.client.setQueryData(key, [])
  }
  // Same revisions and same instance: nothing changed, so nothing is refetched.
  await f.events.send('changes', revisions)
  await vi.advanceTimersByTimeAsync(0)
  for (const key of keys) expect(f.client.getQueryState(key)?.isInvalidated).toBe(false)
  // Reconnecting to the same instance with unchanged revisions stays quiet.
  f.reconnect()
  await f.events.send('changes', revisions)
  await vi.advanceTimersByTimeAsync(0)
  for (const key of keys) expect(f.client.getQueryState(key)?.isInvalidated).toBe(false)
  // A restarted server (new instance) reset its counters, so the client re-baselines and refetches.
  await f.events.send('changes', { library: 0, offline: 0, monitor: 0, instance: 'server-b' })
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

function probeFixture() {
  const f = fixture()
  const unauthorized = vi.spyOn(clientAPI, 'notifyUnauthorized').mockImplementation(() => {})
  const headers = Promise.withResolvers<Response>()
  let signal!: AbortSignal
  f.fetch.mockImplementation(async (path, init) => {
    if (path === '/api/tasks/events') {
      signal = init!.signal as AbortSignal
      return headers.promise
    }
    expect(path).toBe('/api/tasks')
    return Response.json([done])
  })
  const timers = vi.spyOn(globalThis, 'setTimeout')
  f.events.close()
  f.events.onerror?.()
  return {
    ...f,
    headers,
    unauthorized,
    timers,
    get signal() {
      return signal
    }
  }
}

test('a stalled 401 body times out and schedules reconnect without reporting expired login', async () => {
  const f = probeFixture()
  const body = new ReadableStream({
    start(controller) {
      f.signal.addEventListener('abort', () => controller.error(f.signal.reason), { once: true })
    }
  })
  f.headers.resolve(new Response(body, { status: 401 }))
  await vi.advanceTimersByTimeAsync(4999)
  expect(f.signal.aborted).toBe(false)
  await vi.advanceTimersByTimeAsync(1)
  expect(f.signal.aborted).toBe(true)
  expect(f.unauthorized).not.toHaveBeenCalled()
  expect(f.timers.mock.calls.filter(([, delay]) => delay === 3000)).toHaveLength(1)
})

for (const phase of ['headers', 'body'] as const) {
  for (const action of ['dispose', 'reconnect'] as const) {
    test(`${action} cancels the probe during ${phase} and ignores its late 401 response`, async () => {
      const f = probeFixture()
      const body = Promise.withResolvers<unknown>()
      const response = new Response(null, { status: 401 })
      vi.spyOn(response, 'json').mockReturnValue(body.promise)
      if (phase === 'body') {
        f.headers.resolve(response)
        await vi.advanceTimersByTimeAsync(0)
      }
      expect(f.signal.aborted).toBe(false)
      f[action]()
      expect(f.signal.aborted).toBe(true)
      f.timers.mockClear()
      // Simulate a response settling after cancellation, before its continuation runs.
      f.headers.resolve(response)
      body.resolve({ code: 'UNAUTHORIZED' })
      await vi.advanceTimersByTimeAsync(0)
      expect(f.unauthorized).not.toHaveBeenCalled()
      expect(f.timers.mock.calls.filter(([, delay]) => delay === 3000)).toHaveLength(0)
    })
  }
}

test('a rejected probe clears its deadline and schedules one reconnect', async () => {
  const clearTimer = vi.spyOn(globalThis, 'clearTimeout')
  const f = probeFixture()
  const timeoutIndex = f.timers.mock.calls.findIndex(([, delay]) => delay === 5000)
  expect(timeoutIndex).toBeGreaterThanOrEqual(0)
  const timeout = f.timers.mock.results[timeoutIndex]!.value
  f.headers.reject(new TypeError('network unavailable'))
  await vi.advanceTimersByTimeAsync(0)
  expect(clearTimer).toHaveBeenCalledWith(timeout)
  expect(f.signal.aborted).toBe(true)
  expect(f.unauthorized).not.toHaveBeenCalled()
  expect(f.timers.mock.calls.filter(([, delay]) => delay === 3000)).toHaveLength(1)
})

test('repeated closed-stream errors share the pending probe', async () => {
  const f = probeFixture()
  f.events.onerror?.()
  f.events.onerror?.()
  expect(f.fetch.mock.calls.filter(([path]) => path === '/api/tasks/events')).toHaveLength(1)
  f.headers.resolve(new Response(null, { status: 200 }))
  await vi.advanceTimersByTimeAsync(0)
  expect(f.signal.aborted).toBe(true)
  expect(f.timers.mock.calls.filter(([, delay]) => delay === 3000)).toHaveLength(1)
})

test('disposed stream errors and heartbeats cannot start another probe', async () => {
  const f = fixture()
  const previous = f.events
  f.dispose()
  previous.onerror?.()
  await previous.send('ping', null)
  await vi.advanceTimersByTimeAsync(60_000)
  expect(f.fetch).not.toHaveBeenCalled()
})

for (const code of ['UNAUTHORIZED', 'PAN_UNAUTHORIZED', undefined]) {
  test(`a completed 401 probe preserves authentication handling for ${code ?? 'missing code'}`, async () => {
    const f = probeFixture()
    f.headers.resolve(Response.json({ code }, { status: 401 }))
    await vi.advanceTimersByTimeAsync(0)
    const expired = code !== 'PAN_UNAUTHORIZED'
    expect(f.unauthorized).toHaveBeenCalledTimes(expired ? 1 : 0)
    expect(f.signal.aborted).toBe(true)
    expect(f.timers.mock.calls.filter(([, delay]) => delay === 3000)).toHaveLength(expired ? 0 : 1)
  })
}

const source = { account_id: 'acc1', directory: { id: 'dir1', name: 'Movies', path: '/Movies' } }
const queuedScan: ScanTask = {
  id: 42,
  type: 'scan',
  source,
  status: 'queued',
  progress: 0,
  created_at: '',
  updated_at: '',
  scan: {
    stage: 'queued',
    current_path: '',
    directories_discovered: 0,
    directories_scanned: 0,
    files_scanned: 0,
    video_files: 0,
    matched_files: 0,
    unmatched_files: 0,
    movies: 0,
    removed_files: 0,
    removed_movies: 0,
    metadata_total: 0,
    metadata_completed: 0,
    metadata_failed: 0
  }
}
const finishedScan: ScanTask = { ...queuedScan, status: 'done', progress: 100 }

function operation(client: QueryClient, useOperation: () => unknown) {
  function CaptureOperation() {
    useOperation()
    return null
  }
  renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <CaptureOperation />
    </QueryClientProvider>
  )
  const options = vi.mocked(useMutation).mock.calls.at(-1)![0] as UseMutationOptions<
    unknown,
    Error,
    unknown,
    unknown
  >
  return client.getMutationCache().build(client, options)
}

function deferredResponses(f: ReturnType<typeof fixture>, readPath: string) {
  const result = Promise.withResolvers<Response>()
  const reads: (PromiseWithResolvers<Response> & { signal?: AbortSignal | null })[] = []
  f.fetch.mockImplementation(async (path, init) => {
    if (init?.method === 'POST') return result.promise
    expect(path).toBe(readPath)
    const read = { ...Promise.withResolvers<Response>(), signal: init?.signal }
    reads.push(read)
    return read.promise
  })
  return { result, reads }
}

function observeActivity(f: ReturnType<typeof fixture>, tasks: OfflineActivity['tasks']) {
  f.client.setQueryData(offlineKeys.activity, { source, tasks })
  f.client.setQueryData(panKeys.account, {
    connected: true,
    account: { id: source.account_id },
    directory: source.directory
  })
  const observer = new QueryObserver(f.client, {
    queryKey: offlineKeys.activity,
    queryFn: ({ signal }) => apiGet<OfflineActivity>('/api/offline/tasks', undefined, signal),
    staleTime: Infinity
  })
  onTestFinished(observer.subscribe(() => {}))
}

function useRescrapeFixture() {
  return useRescrapeLibraryMovie(42)
}

function useAddFixture() {
  return useAddOffline('one')
}

function useCancelFixture() {
  return useOfflineControl('cancel')
}

function useNextFixture() {
  return useOfflineControl('next')
}

for (const scan of [
  { name: 'scan', useOperation: useStartLibraryScan, input: false },
  { name: 'rescrape', useOperation: useRescrapeFixture, input: 'ABP-001' }
]) {
  test(`${scan.name}: an SSE snapshot arriving first survives an older mutation response`, async () => {
    const f = fixture()
    const { result, reads } = deferredResponses(f, '/api/tasks')
    const mutation = operation(f.client, scan.useOperation).execute(scan.input)
    await vi.advanceTimersByTimeAsync(0)
    await f.events.send('tasks', [finishedScan])
    result.resolve(Response.json(queuedScan))
    await vi.advanceTimersByTimeAsync(0)
    expect(f.client.getQueryData(taskKeys.all)).toEqual([finishedScan])
    expect(reads).toHaveLength(1)
    reads[0]!.resolve(Response.json([finishedScan]))
    await mutation
  })

  test(`${scan.name}: a mutation arriving first is corrected by SSE without a later stale overwrite`, async () => {
    const f = fixture()
    const { result, reads } = deferredResponses(f, '/api/tasks')
    const mutation = operation(f.client, scan.useOperation).execute(scan.input)
    await vi.advanceTimersByTimeAsync(0)
    result.resolve(Response.json(queuedScan))
    await vi.advanceTimersByTimeAsync(0)
    expect(f.client.getQueryData<Task[]>(taskKeys.all)?.[0]).toEqual(queuedScan)
    expect(reads).toHaveLength(1)
    await f.events.send('tasks', [finishedScan])
    await mutation
    expect(reads[0]!.signal?.aborted).toBe(true)
    reads[0]!.resolve(Response.json([queuedScan]))
    await vi.advanceTimersByTimeAsync(0)
    expect(f.client.getQueryData(taskKeys.all)).toEqual([finishedScan])
    expect(reads).toHaveLength(1)
  })

  test(`${scan.name}: a response from the old account is not inserted after a source switch`, async () => {
    const f = fixture()
    f.client.setQueryData(taskKeys.all, [])
    const { result, reads } = deferredResponses(f, '/api/tasks')
    const mutation = operation(f.client, scan.useOperation).execute(scan.input)
    await vi.advanceTimersByTimeAsync(0)
    f.client.setQueryData(panKeys.account, {
      connected: true,
      account: { id: 'acc2' },
      directory: { id: 'dir2' }
    })
    result.resolve(Response.json(queuedScan))
    await vi.advanceTimersByTimeAsync(0)
    expect(f.client.getQueryData(taskKeys.all)).toEqual([])
    reads[0]!.resolve(Response.json([]))
    await mutation
  })
}

test('offline submission and an SSE burst share a current read and one follow-up', async () => {
  const f = fixture()
  await f.events.send('changes', { library: 0, offline: 0, monitor: 0 })
  await vi.advanceTimersByTimeAsync(0)
  observeActivity(f, [])
  const { result, reads } = deferredResponses(f, '/api/offline/tasks')
  const submitted = offlineSubmission({ status: 'running', phase: 'downloading', progress: 0 })
  const final = offlineSubmission()
  const mutation = operation(f.client, useAddFixture).execute('fixture-hash')
  await vi.advanceTimersByTimeAsync(0)
  result.resolve(Response.json(submitted))
  await mutation
  await vi.advanceTimersByTimeAsync(0)
  expect(reads).toHaveLength(1)
  expect(f.client.getQueryData<OfflineActivity>(offlineKeys.activity)?.tasks).toEqual([submitted])
  for (const revision of [1, 2, 3]) {
    await f.events.send('changes', { library: 0, offline: revision, monitor: 0 })
  }
  await vi.advanceTimersByTimeAsync(0)
  expect(reads[0]!.signal?.aborted).toBe(false)
  expect(reads).toHaveLength(1)
  reads[0]!.resolve(Response.json({ source, tasks: [submitted] }))
  await vi.advanceTimersByTimeAsync(0)
  expect(reads).toHaveLength(2)
  reads[1]!.resolve(Response.json({ source, tasks: [final] }))
  await vi.advanceTimersByTimeAsync(0)
  expect(f.client.getQueryData<OfflineActivity>(offlineKeys.activity)?.tasks).toEqual([final])
})

test('a submission response arriving during an SSE refresh waits for one correction without aborting it', async () => {
  const f = fixture()
  await f.events.send('changes', { library: 0, offline: 0, monitor: 0 })
  await vi.advanceTimersByTimeAsync(0)
  observeActivity(f, [])
  const { result, reads } = deferredResponses(f, '/api/offline/tasks')
  const mutation = operation(f.client, useAddFixture).execute('fixture-hash')
  await vi.advanceTimersByTimeAsync(0)
  await f.events.send('changes', { library: 0, offline: 1, monitor: 0 })
  await vi.advanceTimersByTimeAsync(0)
  result.resolve(
    Response.json(offlineSubmission({ status: 'running', phase: 'downloading', progress: 0 }))
  )
  await mutation
  await vi.advanceTimersByTimeAsync(0)
  expect(reads).toHaveLength(1)
  expect(reads[0]!.signal?.aborted).toBe(false)
  const current = { source, tasks: [offlineSubmission()] }
  reads[0]!.resolve(Response.json(current))
  await vi.advanceTimersByTimeAsync(0)
  expect(reads).toHaveLength(2)
  expect(f.client.getQueryData(offlineKeys.activity)).toEqual(current)
  reads[1]!.resolve(Response.json(current))
  await vi.advanceTimersByTimeAsync(0)
  expect(f.client.getQueryData(offlineKeys.activity)).toEqual(current)
})

test('events from a disposed connection do not cancel the new connection fallback', async () => {
  const f = fixture()
  const previous = f.events
  f.reconnect()
  const response = Promise.withResolvers<Response>()
  f.fetch.mockImplementation(() => response.promise)
  f.events.fail()
  await vi.advanceTimersByTimeAsync(0)
  const signal = f.fetch.mock.calls[0]![1]?.signal
  await previous.send('tasks', [])
  expect(signal?.aborted).toBe(false)
  response.resolve(Response.json([done]))
  await vi.advanceTimersByTimeAsync(0)
  expect(f.client.getQueryData(taskKeys.all)).toEqual([done])
})

for (const { action, useOperation } of [
  { action: 'add', useOperation: useAddFixture },
  { action: 'cancel', useOperation: useCancelFixture },
  { action: 'next', useOperation: useNextFixture }
]) {
  test(`${action}: an SSE-triggered read cannot be rolled back by a late operation response`, async () => {
    const f = fixture()
    await f.events.send('changes', { library: 0, offline: 0, monitor: 0 })
    await vi.advanceTimersByTimeAsync(0)
    const original = offlineSubmission({ status: 'running', phase: 'downloading', progress: 0 })
    observeActivity(f, action === 'add' ? [] : [original])
    const { result, reads } = deferredResponses(f, '/api/offline/tasks')
    const mutation = operation(f.client, useOperation).execute(
      action === 'add' ? 'fixture-hash' : 1
    )
    await vi.advanceTimersByTimeAsync(0)
    await f.events.send('changes', { library: 0, offline: 1, monitor: 0 })
    await vi.advanceTimersByTimeAsync(0)
    const latest =
      action === 'next'
        ? { ...original, hash: 'third-candidate', attempt_count: 3 }
        : offlineSubmission({ status: action === 'cancel' ? 'cancelled' : 'done' })
    reads[0]!.resolve(Response.json({ source, tasks: [latest] }))
    await vi.advanceTimersByTimeAsync(0)
    result.resolve(Response.json(original))
    await vi.advanceTimersByTimeAsync(0)
    expect(f.client.getQueryData<OfflineActivity>(offlineKeys.activity)?.tasks).toEqual([latest])
    expect(reads).toHaveLength(2)
    reads[1]!.resolve(Response.json({ source, tasks: [latest] }))
    await mutation
    await vi.advanceTimersByTimeAsync(0)
  })

  test(`${action}: an old-source response cannot overwrite activity after switching accounts`, async () => {
    const f = fixture()
    const original = offlineSubmission({ status: 'running', phase: 'downloading', progress: 0 })
    observeActivity(f, [original])
    const { result, reads } = deferredResponses(f, '/api/offline/tasks')
    const mutation = operation(f.client, useOperation).execute(
      action === 'add' ? 'fixture-hash' : 1
    )
    await vi.advanceTimersByTimeAsync(0)
    const nextSource = { account_id: 'acc2', directory: { id: 'dir2', name: 'New', path: '/New' } }
    const current = { source: nextSource, tasks: [] }
    f.client.setQueryData(panKeys.account, {
      connected: true,
      account: { id: 'acc2' },
      directory: nextSource.directory
    })
    f.client.setQueryData(offlineKeys.activity, current)
    result.resolve(Response.json(original))
    await vi.advanceTimersByTimeAsync(0)
    expect(f.client.getQueryData(offlineKeys.activity)).toEqual(current)
    expect(reads).toHaveLength(1)
    reads[0]!.resolve(Response.json(current))
    await mutation
    await vi.advanceTimersByTimeAsync(0)
  })

  if (action !== 'add') {
    test(`${action}: failures still reconcile a possibly changed server task`, async () => {
      const f = fixture()
      const original = offlineSubmission({ status: 'running', phase: 'downloading', progress: 0 })
      observeActivity(f, [original])
      const { result, reads } = deferredResponses(f, '/api/offline/tasks')
      const mutation = operation(f.client, useOperation).execute(1)
      const rejected = expect(mutation).rejects.toThrow('temporary failure')
      await vi.advanceTimersByTimeAsync(0)
      result.resolve(Response.json({ error: 'temporary failure' }, { status: 503 }))
      await vi.advanceTimersByTimeAsync(0)
      expect(reads).toHaveLength(1)
      const current = offlineSubmission({ status: 'cancelled', phase: 'available' })
      reads[0]!.resolve(Response.json({ source, tasks: [current] }))
      await rejected
      expect(f.client.getQueryData<OfflineActivity>(offlineKeys.activity)?.tasks).toEqual([current])
    })
  }
}
