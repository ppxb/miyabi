import assert from 'node:assert/strict'
import { test, onTestFinished, vi } from 'vitest'

import { ApiError, apiPost, clearLegacyAuthToken, notifyUnauthorized } from '@/api/client'

test('requests use cookies without reading or sending a stored JWT', async () => {
  const originalStorage = globalThis.localStorage
  vi.stubGlobal('localStorage', {
    getItem() {
      throw new Error('must not read credentials')
    }
  })
  onTestFinished(() => {
    globalThis.localStorage = originalStorage
  })
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (_path, init) => {
    assert.ok(init)
    assert.equal(init.credentials, 'same-origin')
    assert.equal(new Headers(init.headers).has('Authorization'), false)
    assert.equal(new Headers(init.headers).get('Content-Type'), 'application/json')
    return new Response('{}')
  })
  await apiPost('/api/test', { value: 1 })
})

test('upgrade removes only the legacy JWT and tolerates blocked storage', () => {
  const originalStorage = globalThis.localStorage
  onTestFinished(() => {
    globalThis.localStorage = originalStorage
  })
  const storage = new Map([
    ['miyabi_jwt_token', 'old-token'],
    ['miyabi-theme', 'dark']
  ])
  vi.stubGlobal('localStorage', { removeItem: (key: string) => storage.delete(key) })
  clearLegacyAuthToken()
  assert.equal(storage.has('miyabi_jwt_token'), false)
  assert.equal(storage.get('miyabi-theme'), 'dark')
  vi.stubGlobal('localStorage', {
    removeItem() {
      throw new Error('blocked')
    }
  })
  assert.doesNotThrow(clearLegacyAuthToken)
})

for (const scenario of [
  {
    name: 'JSON errors retain the server message and HTTP status',
    body: JSON.stringify({ error: '115 登录已失效' }),
    status: 401,
    statusText: 'Unauthorized',
    message: '115 登录已失效'
  },
  {
    name: 'HTML gateway errors retain their HTTP status',
    body: '<html>Bad Gateway</html>',
    status: 502,
    statusText: 'Bad Gateway',
    message: 'Bad Gateway'
  },
  {
    name: 'empty authorization errors retain their HTTP status',
    body: null,
    status: 401,
    statusText: 'Unauthorized',
    message: 'Unauthorized'
  },
  ...['{', 'null', '{}', '{"error":42}', '{"error":"  "}'].map(body => ({
    name: `invalid error payload falls back to the HTTP status: ${body}`,
    body,
    status: 503,
    statusText: '',
    message: '请求失败（HTTP 503）'
  }))
]) {
  test(scenario.name, async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(
      async () =>
        new Response(scenario.body, {
          status: scenario.status,
          statusText: scenario.statusText
        })
    )

    await assert.rejects(apiPost('/api/test'), error => {
      assert.ok(error instanceof ApiError)
      assert.equal(error.status, scenario.status)
      assert.equal(error.message, scenario.message)
      return true
    })
  })
}

for (const error of [
  new TypeError('Failed to fetch'),
  new DOMException('The request was aborted', 'AbortError')
]) {
  test(`fetch failures preserve the original ${error.name}`, async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      throw error
    })

    await assert.rejects(apiPost('/api/test'), actual => actual === error)
  })
}

test('cancellation while reading an error body remains cancellation', async () => {
  const error = new DOMException('The request was aborted', 'AbortError')
  const body = new ReadableStream({
    start(controller) {
      controller.error(error)
    }
  })
  vi.spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(body, { status: 401 }))

  await assert.rejects(apiPost('/api/test'), actual => actual === error)
})

test('401 with code UNAUTHORIZED dispatches miyabi:unauthorized', async () => {
  const events: string[] = []
  const storage = new Map([['miyabi_jwt_token', 'test-token']])
  const originalWindow = globalThis.window
  const originalLocalStorage = globalThis.localStorage

  vi.stubGlobal('localStorage', {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, val: string) => storage.set(key, val),
    removeItem: (key: string) => storage.delete(key)
  })
  vi.stubGlobal('window', {
    dispatchEvent: (event: Event) => {
      events.push(event.type)
      return true
    }
  })

  onTestFinished(() => {
    globalThis.window = originalWindow
    globalThis.localStorage = originalLocalStorage
  })

  vi.spyOn(globalThis, 'fetch').mockImplementation(
    async () =>
      new Response(JSON.stringify({ error: '认证令牌已过期', code: 'UNAUTHORIZED' }), {
        status: 401,
        statusText: 'Unauthorized'
      })
  )

  await assert.rejects(apiPost('/api/test'), error => {
    assert.ok(error instanceof ApiError)
    assert.equal(error.status, 401)
    assert.equal(error.code, 'UNAUTHORIZED')
    return true
  })

  assert.equal(storage.get('miyabi_jwt_token'), 'test-token')
  assert.deepEqual(events, ['miyabi:unauthorized'])
})

test('401 without code UNAUTHORIZED preserves token and does not dispatch miyabi:unauthorized', async () => {
  const events: string[] = []
  const storage = new Map([['miyabi_jwt_token', 'test-token']])
  const originalWindow = globalThis.window
  const originalLocalStorage = globalThis.localStorage

  vi.stubGlobal('localStorage', {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, val: string) => storage.set(key, val),
    removeItem: (key: string) => storage.delete(key)
  })
  vi.stubGlobal('window', {
    dispatchEvent: (event: Event) => {
      events.push(event.type)
      return true
    }
  })

  onTestFinished(() => {
    globalThis.window = originalWindow
    globalThis.localStorage = originalLocalStorage
  })

  vi.spyOn(globalThis, 'fetch').mockImplementation(
    async () =>
      new Response(JSON.stringify({ error: '115 登录已失效' }), {
        status: 401,
        statusText: 'Unauthorized'
      })
  )

  await assert.rejects(apiPost('/api/test'), error => {
    assert.ok(error instanceof ApiError)
    assert.equal(error.status, 401)
    assert.equal(error.message, '115 登录已失效')
    return true
  })

  assert.equal(storage.get('miyabi_jwt_token'), 'test-token')
  assert.deepEqual(events, [])
})

test('notifyUnauthorized dispatches miyabi:unauthorized', () => {
  const events: string[] = []
  const storage = new Map([['miyabi_jwt_token', 'test-token']])
  const originalWindow = globalThis.window
  const originalLocalStorage = globalThis.localStorage

  vi.stubGlobal('localStorage', {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, val: string) => storage.set(key, val),
    removeItem: (key: string) => storage.delete(key)
  })
  vi.stubGlobal('window', {
    dispatchEvent: (event: Event) => {
      events.push(event.type)
      return true
    }
  })

  try {
    notifyUnauthorized()
    assert.equal(storage.get('miyabi_jwt_token'), 'test-token')
    assert.deepEqual(events, ['miyabi:unauthorized'])
  } finally {
    globalThis.window = originalWindow
    globalThis.localStorage = originalLocalStorage
  }
})
