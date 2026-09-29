import assert from 'node:assert/strict'
import { test, vi } from 'vitest'

import { BrowseHistoryStore } from '@/api/browse-history-store'

test('BrowseHistoryStore tracks viewed IDs and deduplicates entries', async () => {
  const synced: string[] = []
  const store = new BrowseHistoryStore({
    syncViewed: async ids => {
      synced.push(...ids)
    },
    storageKey: 'test:browse_history'
  })

  assert.equal(store.isViewed('test-id-1'), false)
  assert.equal(store.isViewed(undefined), false)

  store.recordView('test-id-1')
  store.recordView(' test-id-1 ')
  store.recordView('   ')

  assert.equal(store.isViewed('test-id-1'), true)
  assert.equal(store.isViewed('test-id-2'), false)
  assert.equal(store.getPendingCount(), 1)

  await store.flush()
  assert.equal(store.getPendingCount(), 0)
  assert.deepEqual(synced, ['test-id-1'])
})

test('BrowseHistoryStore keeps pending entries when sync fails', async () => {
  let fail = true
  const store = new BrowseHistoryStore({
    syncViewed: async () => {
      if (fail) throw new Error('offline')
    },
    storageKey: 'test:browse_retry'
  })
  store.recordView('retry-id')
  await store.flush()
  assert.equal(store.getPendingCount(), 1)
  fail = false
  await store.flush()
  assert.equal(store.getPendingCount(), 0)
})

test('BrowseHistoryStore batch triggers flush at 50 items', async () => {
  const syncedBatches: string[][] = []
  const store = new BrowseHistoryStore({
    syncViewed: async ids => {
      syncedBatches.push([...ids])
    },
    storageKey: 'test:browse_batch'
  })

  for (let i = 0; i < 49; i++) store.recordView(`item-${i}`)
  assert.equal(syncedBatches.length, 0)
  assert.equal(store.getPendingCount(), 49)

  store.recordView('item-49')
  await new Promise(resolve => setTimeout(resolve, 10))
  assert.equal(syncedBatches.length, 1)
  assert.equal(syncedBatches[0]!.length, 50)
  assert.equal(store.getPendingCount(), 0)
})

test('BrowseHistoryStore flushKeepalive clears pending and persists state', () => {
  const originalWindow = globalThis.window
  const originalNavDesc = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
  const originalLocalStorage = globalThis.localStorage

  const storage = new Map<string, string>()
  const beacons: { url: string | URL; blob: Blob }[] = []

  const mockStorage = {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, val: string) => storage.set(key, val),
    removeItem: (key: string) => storage.delete(key)
  }
  vi.stubGlobal('localStorage', mockStorage)
  vi.stubGlobal('window', {
    addEventListener: () => {},
    removeEventListener: () => {},
    localStorage: mockStorage
  })
  Object.defineProperty(globalThis, 'navigator', {
    value: {
      sendBeacon: (url: string | URL, blob: Blob) => {
        beacons.push({ url, blob })
        return true
      }
    },
    configurable: true
  })

  try {
    const store = new BrowseHistoryStore({ storageKey: 'test:keepalive_clear' })
    store.recordView('movie-1')
    assert.equal(store.getPendingCount(), 1)

    store.flushKeepalive()

    assert.equal(beacons.length, 1)
    assert.equal(beacons[0]!.url, '/api/discover/viewed')
    assert.equal(store.getPendingCount(), 0)

    const saved = JSON.parse(storage.get('test:keepalive_clear')!)
    assert.deepEqual(saved.pending, [])
    assert.deepEqual(saved.ids, ['movie-1'])
  } finally {
    globalThis.window = originalWindow
    if (originalNavDesc) {
      Object.defineProperty(globalThis, 'navigator', originalNavDesc)
    }
    globalThis.localStorage = originalLocalStorage
  }
})

test('BrowseHistoryStore flushKeepalive does not send duplicate beacon while flush is in flight', async () => {
  const originalWindow = globalThis.window
  const originalNavDesc = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
  const originalLocalStorage = globalThis.localStorage

  const storage = new Map<string, string>()
  const beacons: { url: string | URL; blob: Blob }[] = []
  const { promise: syncPromise, resolve: resolveSync } = Promise.withResolvers<void>()

  const mockStorage = {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, val: string) => storage.set(key, val),
    removeItem: (key: string) => storage.delete(key)
  }
  vi.stubGlobal('localStorage', mockStorage)
  vi.stubGlobal('window', {
    addEventListener: () => {},
    removeEventListener: () => {},
    localStorage: mockStorage
  })
  Object.defineProperty(globalThis, 'navigator', {
    value: {
      sendBeacon: (url: string | URL, blob: Blob) => {
        beacons.push({ url, blob })
        return true
      }
    },
    configurable: true
  })

  try {
    const store = new BrowseHistoryStore({
      storageKey: 'test:keepalive_inflight',
      syncViewed: async () => {
        await syncPromise
      }
    })
    store.recordView('movie-2')
    assert.equal(store.getPendingCount(), 1)

    const flushPromise = store.flush()
    store.flushKeepalive()

    assert.equal(beacons.length, 0)
    assert.equal(store.getPendingCount(), 1)

    resolveSync()
    await flushPromise

    assert.equal(store.getPendingCount(), 0)
    assert.equal(beacons.length, 0)
  } finally {
    globalThis.window = originalWindow
    if (originalNavDesc) {
      Object.defineProperty(globalThis, 'navigator', originalNavDesc)
    }
    globalThis.localStorage = originalLocalStorage
  }
})

test('keepalive fallback preserves failed batches and acknowledges only successful IDs', async () => {
  const originalWindow = globalThis.window
  const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
  const originalFetch = globalThis.fetch
  const saved = new Map<string, string>()
  vi.stubGlobal('window', {
    addEventListener: () => {},
    localStorage: {
      getItem: (key: string) => saved.get(key),
      setItem: (key: string, value: string) => saved.set(key, value)
    }
  })
  Object.defineProperty(globalThis, 'navigator', {
    value: { sendBeacon: () => false },
    configurable: true
  })
  try {
    const store = new BrowseHistoryStore({ storageKey: 'test:fallback' })
    store.recordView('first')
    for (const fail of [
      async () => new Response(null, { status: 500 }),
      async () => {
        throw new Error('offline')
      }
    ]) {
      globalThis.fetch = fail
      await store.flushKeepalive()
      assert.equal(store.getPendingCount(), 1)
      assert.deepEqual(JSON.parse(saved.get('test:fallback')!).pending, ['first'])
    }
    const { promise: response, resolve: finish } = Promise.withResolvers<Response>()
    let requests = 0
    globalThis.fetch = () => {
      requests++
      return response
    }
    const pending = store.flushKeepalive()
    await store.flushKeepalive()
    assert.equal(requests, 1)
    store.recordView('second')
    finish(new Response())
    await pending
    assert.deepEqual(JSON.parse(saved.get('test:fallback')!).pending, ['second'])
  } finally {
    globalThis.window = originalWindow
    globalThis.fetch = originalFetch
    if (originalNavigator) Object.defineProperty(globalThis, 'navigator', originalNavigator)
    else Reflect.deleteProperty(globalThis, 'navigator')
  }
})
