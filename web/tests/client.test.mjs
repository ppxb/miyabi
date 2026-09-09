import assert from 'node:assert/strict'
import { test } from 'node:test'

import { ApiError, apiPost } from '../src/api/client.ts'

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
  test(scenario.name, async t => {
    t.mock.method(
      globalThis,
      'fetch',
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
  test(`fetch failures preserve the original ${error.name}`, async t => {
    t.mock.method(globalThis, 'fetch', async () => {
      throw error
    })

    await assert.rejects(apiPost('/api/test'), actual => actual === error)
  })
}

test('cancellation while reading an error body remains cancellation', async t => {
  const error = new DOMException('The request was aborted', 'AbortError')
  const body = new ReadableStream({
    start(controller) {
      controller.error(error)
    }
  })
  t.mock.method(globalThis, 'fetch', async () => new Response(body, { status: 401 }))

  await assert.rejects(apiPost('/api/test'), actual => actual === error)
})
