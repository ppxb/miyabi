import assert from 'node:assert/strict'
import { test } from 'node:test'
import { QueryClient, QueryObserver } from '@tanstack/react-query'

import {
  subscriptionKeys,
  SUBSCRIPTION_PAGE_SIZE
} from '../src/api/subscriptions.ts'

function createTestClient(t) {
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false
      }
    }
  })
  t.after(() => client.clear())
  return client
}

function observe(t, client, options) {
  const observer = new QueryObserver(client, options)
  const unsubscribe = observer.subscribe(() => {})
  t.after(unsubscribe)
  return observer
}

function settled(observer) {
  if (observer.getCurrentResult().fetchStatus === 'idle') {
    return Promise.resolve(observer.getCurrentResult())
  }
  return new Promise(resolve => {
    const unsubscribe = observer.subscribe(result => {
      if (result.fetchStatus !== 'idle') return
      unsubscribe()
      resolve(result)
    })
  })
}

test('subscription targets indexes more than 100 items with O(1) map lookup', async t => {
  const client = createTestClient(t)

  // Generate 150 subscription targets (beyond old 100 limit)
  const mockTargets = Array.from({ length: 150 }, (_, i) => ({
    id: i + 1,
    kind: 'movie',
    target_id: `movie-${i + 1}`,
    status: i % 2 === 0 ? 'waiting' : 'added'
  }))

  const observer = observe(t, client, {
    queryKey: subscriptionKeys.targets('movie'),
    queryFn: () => mockTargets,
    staleTime: Infinity,
    select: targets => {
      const map = new Map()
      for (const item of targets) {
        map.set(item.target_id, item)
      }
      return { list: targets, map }
    }
  })

  const result = await settled(observer)
  assert.equal(result.data.list.length, 150)
  assert.equal(result.data.map.size, 150)

  // Check 1st item and 101st item and 150th item
  assert.equal(result.data.map.get('movie-1')?.status, 'waiting')
  assert.equal(result.data.map.get('movie-101')?.status, 'waiting')
  assert.equal(result.data.map.get('movie-150')?.status, 'added')
  assert.equal(result.data.map.get('movie-nonexistent'), undefined)
})

test('subscriptionKeys.all invalidates all target and paginated list queries', async t => {
  const client = createTestClient(t)

  let targetsCalls = 0
  let listCalls = 0

  const targetsObserver = observe(t, client, {
    queryKey: subscriptionKeys.targets('movie'),
    queryFn: () => {
      targetsCalls++
      return [{ id: 1, kind: 'movie', target_id: 'm-1', status: 'waiting' }]
    }
  })

  const listObserver = observe(t, client, {
    queryKey: subscriptionKeys.list('movie', 1, SUBSCRIPTION_PAGE_SIZE),
    queryFn: () => {
      listCalls++
      return [{ id: 1, kind: 'movie', target_id: 'm-1', code: 'M-1', title: 'Movie 1' }]
    }
  })

  await settled(targetsObserver)
  await settled(listObserver)
  assert.equal(targetsCalls, 1)
  assert.equal(listCalls, 1)

  // Invalidate all subscriptions
  await client.invalidateQueries({ queryKey: subscriptionKeys.all })

  assert.equal(targetsCalls, 2)
  assert.equal(listCalls, 2)
})
