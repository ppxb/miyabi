import assert from 'node:assert/strict'
import { setImmediate } from 'node:timers/promises'
import { test, onTestFinished, vi } from 'vitest'
import { QueryObserver, type QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { ApiError, apiPost, describeApiError, isPanUnauthorized } from '@/api/client'
import { createAppQueryClient } from '@/api/query-client'
import { panKeys } from '@/api/pan'
import { libraryKeys } from '@/api/library'

function clientFixture() {
  const client = createAppQueryClient()
  onTestFinished(() => client.clear())
  client.setQueryData(panKeys.account, { connected: true })
  client.setQueryData(libraryKeys.movies(1), [])
  return client
}

function failMutation(client: QueryClient, error: Error, meta?: { errorTitle?: string }) {
  return client
    .getMutationCache()
    .build(client, {
      mutationFn: async () => {
        throw error
      },
      meta
    })
    .execute(undefined)
}

test('API errors preserve business codes and distinguish app auth from 115 auth', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(
      JSON.stringify({
        error: 'pan authorization required',
        code: 'PAN_UNAUTHORIZED'
      }),
      { status: 401 }
    )
  )
  const dispatch = vi.fn()
  vi.stubGlobal('window', { dispatchEvent: dispatch })
  await assert.rejects(apiPost('/api/test'), error => {
    assert.ok(error instanceof ApiError)
    assert.equal(error.code, 'PAN_UNAUTHORIZED')
    assert.equal(isPanUnauthorized(error), true)
    assert.equal(describeApiError(error), '115 登录已失效，请前往设置重新登录。')
    return true
  })
  assert.equal(dispatch.mock.calls.length, 0)
  assert.equal(isPanUnauthorized(new ApiError('Session expired', 401, 'UNAUTHORIZED')), false)
  assert.equal(
    describeApiError(new ApiError('Session expired', 401, 'UNAUTHORIZED')),
    'Session expired'
  )
  assert.equal(isPanUnauthorized(new ApiError('Unknown 401', 401)), false)
})

test('only account and source business errors invalidate related cached state', async () => {
  for (const [code, status, refreshAccount, refreshLibrary] of [
    ['PAN_UNAUTHORIZED', 401, true, true],
    ['PAN_DIRECTORY_REQUIRED', 400, true, true],
    ['PAN_SOURCE_CHANGED', 409, true, true],
    ['UNAUTHORIZED', 401, false, false],
    [undefined, 400, false, false],
    [undefined, 401, false, false],
    [undefined, 500, false, false]
  ] as const) {
    const client = clientFixture()
    await assert.rejects(failMutation(client, new ApiError('failure', status, code)))
    assert.equal(client.getQueryState(panKeys.account)?.isInvalidated, refreshAccount)
    assert.equal(client.getQueryState(libraryKeys.movies(1))?.isInvalidated, refreshLibrary)
  }
})

test('a shared failing query refreshes the account once and account failure cannot loop', async () => {
  const client = clientFixture()
  let accountRequests = 0
  const observer = new QueryObserver(client, {
    queryKey: panKeys.account,
    queryFn: async () => {
      accountRequests++
      throw new ApiError('expired', 401, 'PAN_UNAUTHORIZED')
    }
  })
  const unsubscribe = observer.subscribe(() => {})
  onTestFinished(unsubscribe)
  const files = {
    queryKey: panKeys.files('account', 'root', 1),
    queryFn: async () => {
      throw new ApiError('expired', 401, 'PAN_UNAUTHORIZED')
    }
  }
  await Promise.all([
    assert.rejects(client.fetchQuery(files)),
    assert.rejects(client.fetchQuery(files))
  ])
  await setImmediate()
  assert.equal(accountRequests, 1)
  assert.equal(observer.getCurrentResult().isError, true)
  assert.equal(observer.getCurrentResult().fetchStatus, 'idle')
})

test('concurrent operation failures reuse an in-flight account refresh', async () => {
  const client = clientFixture()
  const pending = Promise.withResolvers<{ connected: boolean }>()
  let requests = 0
  const observer = new QueryObserver(client, {
    queryKey: panKeys.account,
    queryFn: () => {
      requests++
      return pending.promise
    }
  })
  onTestFinished(observer.subscribe(() => {}))
  await Promise.all(
    Array.from({ length: 3 }, () =>
      assert.rejects(failMutation(client, new ApiError('expired', 401, 'PAN_UNAUTHORIZED')))
    )
  )
  assert.equal(requests, 1)
  pending.resolve({ connected: false })
  await setImmediate()
  assert.deepEqual(observer.getCurrentResult().data, { connected: false })
})

test('only opted-in mutations toast; inline failures, queries, cancellation and app auth stay quiet', async () => {
  const client = clientFixture()
  const notify = vi.spyOn(toast, 'error').mockReturnValue(1)
  const error = new ApiError('failure', 500)
  await assert.rejects(failMutation(client, error))
  await assert.rejects(
    client.fetchQuery({
      queryKey: ['failure'],
      queryFn: async () => {
        throw error
      }
    })
  )
  await assert.rejects(
    failMutation(client, new ApiError('expired', 401, 'UNAUTHORIZED'), {
      errorTitle: 'Operation failed'
    })
  )
  await assert.rejects(
    failMutation(client, new DOMException('canceled', 'AbortError'), {
      errorTitle: 'Operation failed'
    })
  )
  assert.equal(notify.mock.calls.length, 0)
  await assert.rejects(failMutation(client, error, { errorTitle: 'Operation failed' }))
  assert.equal(notify.mock.calls.length, 1)
  assert.deepEqual(notify.mock.calls[0], ['Operation failed', { description: 'failure' }])
})
