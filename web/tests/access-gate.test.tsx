import assert from 'node:assert/strict'
import { setImmediate } from 'node:timers/promises'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderToStaticMarkup } from 'react-dom/server'
import { onTestFinished, test, vi } from 'vitest'

import { accessGateLoginOptions, authKeys, type AccessGateConfig } from '@/api/auth'
import { ApiError } from '@/api/client'
import { AccessGate } from '@/features/access-gate/access-gate'

const locked: AccessGateConfig = { enabled: true, authenticated: false }

function fixture() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  onTestFinished(() => client.clear())
  client.setQueryData(authKeys.config, locked)
  vi.stubGlobal('window', { location: { origin: 'http://localhost' } })

  const login = client.getMutationCache().build(client, accessGateLoginOptions(client))
  return { client, login }
}

function renderGate(client: QueryClient) {
  return renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <AccessGate>
        <p>Protected content</p>
      </AccessGate>
    </QueryClientProvider>
  )
}

test('login unlocks the gate despite an older config response, and a fresh config can lock it again', async () => {
  const { client, login } = fixture()
  assert.match(renderGate(client), /访问 Miyabi/)
  const oldResponse = Promise.withResolvers<Response>()
  let readSignal: AbortSignal | null | undefined
  const fetch = vi.spyOn(globalThis, 'fetch').mockImplementation(async (path, init) => {
    if (path === '/api/auth/config') {
      readSignal = init?.signal
      return oldResponse.promise
    }
    assert.equal(path, '/api/auth/login')
    assert.equal(init?.method, 'POST')
    assert.equal(init?.body, JSON.stringify({ password: 'password' }))
    return Response.json({ success: true })
  })

  const refresh = client.refetchQueries({ queryKey: authKeys.config })
  assert.ok(readSignal)
  assert.equal(readSignal.aborted, false)
  await login.execute('password')
  assert.equal(readSignal.aborted, true)
  assert.deepEqual(client.getQueryData(authKeys.config), { enabled: true, authenticated: true })
  assert.equal(renderGate(client), '<p>Protected content</p>')

  // Even if the transport still delivers the canceled response, it must not relock the gate.
  oldResponse.resolve(Response.json(locked))
  await refresh
  await setImmediate()
  assert.equal(renderGate(client), '<p>Protected content</p>')

  fetch.mockResolvedValue(Response.json(locked))
  await client.refetchQueries({ queryKey: authKeys.config })
  assert.match(renderGate(client), /访问 Miyabi/)
  assert.doesNotMatch(renderGate(client), /Protected content/)
})

test('a rejected login preserves the locked config and keeps protected content hidden', async () => {
  const { client, login } = fixture()
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    Response.json({ error: '密码错误', code: 'UNAUTHORIZED' }, { status: 401 })
  )
  await assert.rejects(login.execute('wrong'), error => {
    assert.ok(error instanceof ApiError)
    assert.equal(error.message, '密码错误')
    return true
  })
  assert.deepEqual(client.getQueryData(authKeys.config), locked)
  assert.match(renderGate(client), /访问 Miyabi/)
  assert.doesNotMatch(renderGate(client), /Protected content/)
})

test('a disabled gate allows access without an authenticated session', () => {
  const { client } = fixture()
  client.setQueryData(authKeys.config, { enabled: false, authenticated: false })
  assert.equal(renderGate(client), '<p>Protected content</p>')
})
