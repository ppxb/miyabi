import assert from 'node:assert/strict'
import { test } from 'node:test'

import { taskProgressState } from '../src/features/tasks/task-progress-state.ts'

test('scan and download workflows advance through their own stages', () => {
  assert.equal(taskProgressState('scanning').value, 0)
  assert.equal(taskProgressState('scraping').value, 50)
  assert.equal(taskProgressState('artwork', false, 50).value, 87.5)
  assert.equal(taskProgressState('downloading', true, 50).value, 10)
  assert.equal(taskProgressState('scanning', true).value, 20)
  assert.equal(taskProgressState('done').value, 100)
  assert.equal(taskProgressState('done', true).value, 100)
})

test('waiting phases describe what the workflow is waiting for', () => {
  assert.deepEqual(taskProgressState('queued'), { label: '等待扫描', value: 0 })
  assert.deepEqual(taskProgressState('locating', true), {
    label: '已下载，等待 115 返回文件信息',
    value: 20
  })
})

test('an unknown or unavailable phase stays indeterminate instead of crashing', () => {
  for (const phase of ['future-stage', 'downloading']) {
    assert.deepEqual(taskProgressState(phase), { label: '等待进度同步', value: null })
  }
})

test('invalid numeric progress cannot escape its current stage or produce NaN', () => {
  for (const value of [-1, NaN, Infinity]) {
    assert.equal(taskProgressState('artwork', false, value).value, 75)
  }
  assert.equal(taskProgressState('downloading', true, 120).value, 20)
})
