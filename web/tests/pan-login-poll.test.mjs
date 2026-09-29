import assert from 'node:assert/strict'
import { test } from 'vitest'

import {
  PAN_LOGIN_MAX_FAILURES,
  PAN_LOGIN_POLL_MS,
  panLoginPollDelay,
  panLoginShouldRetry
} from '@/lib/pan-login'

for (const scenario of [
  {
    name: 'a waiting login keeps polling',
    input: { failed: false, state: 'waiting' },
    want: PAN_LOGIN_POLL_MS
  },
  {
    name: 'a scanned login keeps polling until the phone confirms',
    input: { failed: false, state: 'scanned' },
    want: PAN_LOGIN_POLL_MS
  },
  {
    name: 'the first answer has not arrived yet',
    input: { failed: false, state: undefined },
    want: PAN_LOGIN_POLL_MS
  },
  {
    name: 'a completed login stops polling',
    input: { failed: false, state: 'authorized' },
    want: false
  },
  {
    name: 'an expired login stops polling so the dialog can refresh it',
    input: { failed: false, state: 'expired' },
    want: false
  },
  {
    name: 'a login the user canceled on their phone stops polling',
    input: { failed: false, state: 'canceled' },
    want: false
  },
  {
    name: 'a poll that spent its failure budget stops polling for the dialog',
    input: { failed: true, state: 'waiting' },
    want: false
  }
]) {
  test(scenario.name, () => {
    assert.equal(panLoginPollDelay(scenario.input), scenario.want)
  })
}

// react-query treats a missing `window` as a server and never polls on an
// interval there, so it has to see a browser before it loads.
globalThis.window = {}
const { QueryClient, QueryObserver } = await import('@tanstack/react-query')

// Mirrors usePanLoginStatus. Only the delays are shortened so a test finishes.
function observePanLogin(queryFn) {
  const client = new QueryClient()
  const observer = new QueryObserver(client, {
    queryKey: ['pan', 'login', 'fixture'],
    queryFn,
    retry: panLoginShouldRetry,
    retryDelay: 1,
    staleTime: 0,
    gcTime: 0,
    refetchInterval: query =>
      panLoginPollDelay({
        failed: query.state.status === 'error',
        state: query.state.data?.state
      }) && 1
  })
  const seen = []
  const stop = observer.subscribe(result => seen.push(result))
  return { observer, seen, stop }
}

async function until(condition) {
  const deadline = Date.now() + 5000
  while (!condition()) {
    if (Date.now() > deadline) throw new Error('condition was not met in time')
    await sleep(2)
  }
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms))
}

test('an unreachable backend spends the whole failure budget before the dialog hears of it', async () => {
  let calls = 0
  const { observer, seen, stop } = observePanLogin(async () => {
    calls++
    throw new Error(`connection refused ${calls}`)
  })
  try {
    await until(() => observer.getCurrentResult().isError)
    assert.equal(calls, PAN_LOGIN_MAX_FAILURES)
    assert.equal(observer.getCurrentResult().failureCount, PAN_LOGIN_MAX_FAILURES)
    const retries = seen.filter(result => !result.isError)
    assert.ok(
      retries.some(result => result.failureCount > 0),
      'retries never showed as such'
    )
    await sleep(25)
    assert.equal(calls, PAN_LOGIN_MAX_FAILURES, 'polling went on after the budget was spent')
  } finally {
    stop()
  }
})

test('a burst of failures inside the budget keeps the last answer and keeps polling', async () => {
  let calls = 0
  const { observer, seen, stop } = observePanLogin(async () => {
    calls++
    if (calls === 2 || calls === 3) throw new Error(`connection reset ${calls}`)
    return { state: 'scanned' }
  })
  try {
    await until(() => calls >= 5)
    assert.ok(
      seen.every(result => !result.isError),
      'a burst inside the budget was reported as an error'
    )
    const failing = seen.filter(result => result.failureCount > 0)
    assert.equal(Math.max(...failing.map(result => result.failureCount)), 2)
    assert.ok(
      failing.every(result => result.data?.state === 'scanned'),
      'the scanned answer was dropped while retrying'
    )
    assert.equal(observer.getCurrentResult().failureCount, 0)
  } finally {
    stop()
  }
})

test('a finished login is not polled again', async () => {
  let calls = 0
  const { observer, stop } = observePanLogin(async () => {
    calls++
    return { state: 'authorized' }
  })
  try {
    await until(() => observer.getCurrentResult().isSuccess)
    await sleep(25)
    assert.equal(calls, 1)
  } finally {
    stop()
  }
})
