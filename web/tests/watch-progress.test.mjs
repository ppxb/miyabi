import assert from 'node:assert/strict'
import { test } from 'node:test'
import { setImmediate } from 'node:timers/promises'

import { createWatchProgressWriter } from '../src/features/player/watch-progress.ts'
import {
  formatWatchTime,
  watchProgressPercent,
  watchResumePosition
} from '../src/lib/watch-progress.ts'

test('progress and resume use a finite duration and the saved video file', () => {
  assert.equal(watchProgressPercent(150, 600), 25)
  assert.equal(watchProgressPercent(800, 600), 100)
  assert.equal(watchProgressPercent(-10, 600), 0)
  for (const value of [0, NaN, Infinity]) assert.equal(watchProgressPercent(100, value), 0)
  const session = { id: 1, session_id: 'session', file_id: 'video', position: 150, duration: 600 }
  assert.equal(watchResumePosition(session, 'video'), 150)
  assert.equal(watchResumePosition(session, 'different-video'), 0)
  assert.equal(watchResumePosition({ ...session, position: 600 }, 'video'), 0)
  assert.equal(watchResumePosition(undefined, 'video'), 0)
  assert.equal(formatWatchTime(3661.8), '1:01:01')
})

test('frequent playback events save at most once per interval and flush the latest position', async () => {
  let now = 0
  const requests = []
  const writer = createWatchProgressWriter({
    sessionID: 'session',
    fileID: 'video',
    now: () => now,
    write: async (progress, keepalive) => requests.push({ ...progress, keepalive })
  })
  for (let i = 1; i < 100; i++) {
    now = i * 100
    writer.update(i, 600)
  }
  assert.equal(requests.length, 0)
  now = 10_000
  writer.update(100, 600)
  await setImmediate()
  assert.deepEqual(
    requests.map(row => [row.position, row.version, row.keepalive]),
    [[100, 1, false]]
  )
  now = 10_500
  writer.update(105, 600)
  const closing = writer.flush(true)
  assert.equal(requests.length, 2, 'pagehide must initiate its keepalive request synchronously')
  await closing
  assert.deepEqual(
    requests.map(row => [row.position, row.version, row.keepalive]),
    [
      [100, 1, false],
      [105, 2, true]
    ]
  )
  await writer.flush(true)
  assert.equal(requests.length, 2)
})

test('closing sends the final snapshot even while an older request is pending', async () => {
  let now = 0
  const requests = []
  const writer = createWatchProgressWriter({
    sessionID: 'session',
    fileID: 'video',
    now: () => now,
    write: (progress, keepalive) =>
      new Promise(resolve => requests.push({ progress, keepalive, resolve }))
  })
  now = 10_000
  writer.update(100, 600)
  await setImmediate()
  now = 20_000
  writer.update(200, 600)
  await setImmediate()
  assert.equal(requests.length, 1)
  const final = writer.flush(true)
  await setImmediate()
  assert.equal(requests.length, 2)
  assert.equal(requests[1].progress.position, 200)
  assert.equal(requests[1].progress.version, 2)
  assert.equal(requests[1].keepalive, true)
  requests[1].resolve()
  await final
  requests[0].resolve()
  await setImmediate()
  await writer.flush(true)
  assert.equal(requests.length, 2, 'late success must not roll back the saved snapshot')
})

test('failed writes can retry without accepting invalid or unbounded media values', async () => {
  let attempts = 0
  let failures = 0
  const requests = []
  const writer = createWatchProgressWriter({
    sessionID: 'session',
    fileID: 'video',
    write: async progress => {
      requests.push(progress)
      if (++attempts === 1) throw new Error('offline')
    },
    onError: () => failures++
  })
  writer.update(NaN, 600)
  writer.update(1, Infinity)
  writer.update(1, 0)
  await writer.flush(true)
  assert.equal(attempts, 0)
  writer.update(700, 600)
  await writer.flush(true)
  assert.equal(failures, 1)
  await writer.flush(true)
  assert.equal(attempts, 2)
  assert.deepEqual(
    requests.map(row => [row.position, row.duration, row.version]),
    [
      [600, 600, 1],
      [600, 600, 2]
    ]
  )
})
