import { beforeEach, expect, test, vi } from 'vitest'

import type { OfflineSubmission } from '@/api/offline'
import type { ScanTask } from '@/api/tasks'
import { notifyOfflineTask, notifyScanTask } from '@/features/tasks/task-toast'

const toastMocks = vi.hoisted(() => ({
  info: vi.fn(),
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn()
}))

vi.mock('sonner', () => ({ toast: toastMocks }))

beforeEach(() => {
  for (const mock of Object.values(toastMocks)) mock.mockClear()
})

const offline: OfflineSubmission = {
  task_id: 7,
  code: 'ABP-001',
  account_id: 'acc',
  directory_id: 'dir',
  hash: 'hash',
  status: 'running',
  phase: 'downloading',
  processing: false,
  progress: 40
}

const scan: ScanTask = {
  id: 9,
  type: 'scan',
  status: 'done',
  progress: 100,
  created_at: '',
  updated_at: '',
  source: { account_id: 'acc', directory: { id: 'dir', name: 'Movies', path: '/Movies' } },
  scan: {
    stage: 'done',
    current_path: '/Movies',
    directories_discovered: 1,
    directories_scanned: 1,
    files_scanned: 1,
    video_files: 1,
    matched_files: 1,
    unmatched_files: 0,
    movies: 1,
    removed_files: 0,
    removed_movies: 0,
    metadata_total: 1,
    metadata_completed: 1
  }
}

function optionsOf(mock: { mock: { calls: unknown[][] } }) {
  return mock.mock.calls[0]![1] as Record<string, unknown>
}

test('offline completion clears the active spinner and dismisses quickly', () => {
  notifyOfflineTask(offline)
  const active = optionsOf(toastMocks.info)
  expect(active.duration).toBe(Infinity)
  expect(active.icon).toBeDefined()

  notifyOfflineTask({ ...offline, status: 'done', phase: 'in_library', progress: 100 })
  const completed = optionsOf(toastMocks.success)
  expect(Object.hasOwn(completed, 'icon')).toBe(true)
  expect(completed.icon).toBeUndefined()
  expect(completed.duration).toBe(3000)
})

test('scan success clears the spinner and dismisses in three seconds', () => {
  notifyScanTask(scan)
  const options = optionsOf(toastMocks.success)
  expect(Object.hasOwn(options, 'icon')).toBe(true)
  expect(options.icon).toBeUndefined()
  expect(options.duration).toBe(3000)
})

test('failures keep the longer duration', () => {
  notifyScanTask({ ...scan, status: 'failed', error: 'boom' })
  expect(optionsOf(toastMocks.error).duration).toBe(8000)
})

test('cancelling an offline task dismisses quickly', () => {
  notifyOfflineTask({ ...offline, status: 'cancelled', phase: 'available' })
  const options = optionsOf(toastMocks.info)
  expect(options.icon).toBeUndefined()
  expect(options.duration).toBe(3000)
})
