import assert from 'node:assert/strict'
import { isValidElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { toast } from 'sonner'
import { test, vi } from 'vitest'

import type { ScanTask } from '@/api/tasks'
import { taskProgressState, type TaskStage } from '@/features/tasks/task-progress-state'
import { notifyOfflineTask, notifyScanTask } from '@/features/tasks/task-toast'
import { offlineSubmission } from './fixtures'

vi.mock('sonner', () => ({ toast: { info: vi.fn() } }))

test('scan and download workflows advance through their own stages', () => {
  assert.equal(taskProgressState('scanning').value, 0)
  assert.equal(taskProgressState('scraping').value, 50)
  assert.equal(taskProgressState('artwork', false, 50).value, 75)
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
  // Simulate an unknown stage received from a newer backend.
  for (const phase of ['future-stage', 'downloading']) {
    assert.deepEqual(taskProgressState(phase as TaskStage), { label: '等待进度同步', value: null })
  }
})

test('invalid numeric progress cannot escape its current stage or produce NaN', () => {
  for (const value of [-1, NaN, Infinity]) {
    assert.equal(taskProgressState('artwork', false, value).value, 50)
  }
  assert.equal(taskProgressState('downloading', true, 120).value, 20)
})

test('metadata and artwork share batch progress in scan and offline notifications', () => {
  const scan: ScanTask = {
    id: 1,
    type: 'scan',
    status: 'running',
    progress: 0,
    created_at: '2026-10-01T09:00:00Z',
    updated_at: '2026-10-01T09:00:00Z',
    source: { account_id: 'account', directory: { id: 'root', name: 'Movies', path: '/Movies' } },
    scan: {
      stage: 'scraping',
      current_path: '/Movies',
      directories_discovered: 1,
      directories_scanned: 1,
      files_scanned: 4,
      video_files: 4,
      matched_files: 4,
      unmatched_files: 0,
      movies: 4,
      removed_files: 0,
      removed_movies: 0,
      metadata_total: 4,
      metadata_completed: 0
    }
  }
  const offline = offlineSubmission({ phase: 'downloaded', processing: true, scan_task_id: 1 })
  const sequence = [
    ['scraping', 0],
    ['artwork', 0],
    ['scraping', 1],
    ['artwork', 1],
    ['scraping', 2],
    ['artwork', 3]
  ] as const
  for (const isOffline of [false, true]) {
    let previous = 0
    for (const [stage, completed] of sequence) {
      scan.scan.stage = stage
      scan.scan.metadata_completed = completed
      scan.progress = (completed / 4) * 100
      if (isOffline) notifyOfflineTask(offline, { scan })
      else notifyScanTask(scan)

      const options = vi.mocked(toast.info).mock.lastCall?.[1]
      assert.ok(isValidElement(options?.description))
      const html = renderToStaticMarkup(options.description)
      const value = Number(html.match(/aria-valuenow="([^"]+)"/)?.[1])
      assert.ok(Number.isFinite(value) && value >= previous, `${stage}: ${previous} -> ${value}`)
      assert.equal(value, isOffline ? 60 + completed * 10 : 50 + completed * 12.5)
      assert.ok(html.includes(`${completed} / 4 部`))
      previous = value
    }
  }
})
