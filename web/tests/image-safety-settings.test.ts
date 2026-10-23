import assert from 'node:assert/strict'
import { test, vi } from 'vitest'

import type { ImageSafety } from '@/stores/settings'

const storageKey = 'miyabi-settings'

function stubStorage(persisted?: { state: unknown; version: number }) {
  const values = new Map<string, string>()
  if (persisted) values.set(storageKey, JSON.stringify(persisted))
  const storage: Storage = {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: key => values.get(key) ?? null,
    key: index => [...values.keys()][index] ?? null,
    removeItem: key => {
      values.delete(key)
    },
    setItem: (key, value) => {
      values.set(key, value)
    }
  }
  vi.stubGlobal('localStorage', storage)
  vi.stubGlobal('window', { localStorage: storage })
  return storage
}

async function loadStore() {
  vi.resetModules()
  const { useSettingsStore } = await import('@/stores/settings')
  await useSettingsStore.persist.rehydrate()
  assert.equal(useSettingsStore.persist.hasHydrated(), true)
  return useSettingsStore
}

test('missing storage defaults to blur and exposes the image safety setter', async () => {
  stubStorage()
  const store = await loadStore()
  assert.equal(store.getState().imageSafety, 'blur')
  assert.equal('nsfwMode' in store.getState(), false)
  store.getState().setImageSafety('show')
  assert.equal(store.getState().imageSafety, 'show')
})

for (const [nsfwMode, imageSafety] of [
  [true, 'hide'],
  [false, 'show']
] as const) {
  test(`version 0 nsfwMode ${nsfwMode} migrates to ${imageSafety}`, async () => {
    const storage = stubStorage({ state: { nsfwMode }, version: 0 })
    const store = await loadStore()
    assert.equal(store.getState().imageSafety, imageSafety)
    assert.equal('nsfwMode' in store.getState(), false)
    assert.deepEqual(JSON.parse(storage.getItem(storageKey)!), {
      state: { imageSafety },
      version: 1
    })
  })
}

for (const state of [{}, { nsfwMode: null }, { nsfwMode: 'false' }, { imageSafety: 'invalid' }]) {
  test(`version 0 defaults to blur for ${JSON.stringify(state)}`, async () => {
    stubStorage({ state, version: 0 })
    const store = await loadStore()
    assert.equal(store.getState().imageSafety, 'blur')
  })
}

const modes: ImageSafety[] = ['show', 'blur', 'hide']

for (const imageSafety of modes) {
  test(`migration preserves valid imageSafety ${imageSafety} over legacy flags`, async () => {
    stubStorage({ state: { imageSafety, nsfwMode: imageSafety !== 'hide' }, version: 0 })
    const store = await loadStore()
    assert.equal(store.getState().imageSafety, imageSafety)
  })

  test(`version 1 ${imageSafety} rehydrates and round-trips through storage`, async () => {
    const storage = stubStorage({ state: { imageSafety }, version: 1 })
    const store = await loadStore()
    assert.equal(store.getState().imageSafety, imageSafety)
    const nextMode = modes[(modes.indexOf(imageSafety) + 1) % modes.length]!
    store.getState().setImageSafety(nextMode)
    assert.deepEqual(JSON.parse(storage.getItem(storageKey)!), {
      state: { imageSafety: nextMode },
      version: 1
    })
    const restored = await loadStore()
    assert.equal(restored.getState().imageSafety, nextMode)
  })
}

test('persist saves only imageSafety, excluding temporary reveal, legacy flags and actions', async () => {
  const storage = stubStorage()
  const store = await loadStore()
  const transientState = { imageSafety: 'hide' as const, revealed: true, nsfwMode: false }
  store.setState(transientState)
  assert.deepEqual(JSON.parse(storage.getItem(storageKey)!), {
    state: { imageSafety: 'hide' },
    version: 1
  })
  const restored = await loadStore()
  assert.equal(restored.getState().imageSafety, 'hide')
  assert.equal('revealed' in restored.getState(), false)
  assert.equal('nsfwMode' in restored.getState(), false)
  assert.equal(typeof restored.getState().setImageSafety, 'function')
})
