import assert from 'node:assert/strict'
import { test, vi } from 'vitest'
import { toast } from 'sonner'
import { isOfflineTaskActive } from '@/api/offline'
import { offlineSubmission } from './fixtures'
import { notifyOfflineTask } from '@/features/tasks/task-toast'
import { downloadStatus } from '@/features/tasks/download-status'
import { diffTaskNotifications, type DiffContext } from '@/features/tasks/task-notification-diff'

vi.mock('sonner', () => ({
  toast: { info: vi.fn(), error: vi.fn(), warning: vi.fn(), success: vi.fn() }
}))

test('user cancellation is a neutral terminal notification without a cancel action', () => {
  vi.clearAllMocks()
  const task = offlineSubmission({ status: 'cancelled', phase: 'available', can_cancel: false })
  notifyOfflineTask(task)
  assert.equal(vi.mocked(toast.error).mock.calls.length, 0)
  const [, options] = vi.mocked(toast.info).mock.calls[0]!
  assert.equal(options?.description, '已取消下载')
  assert.equal(options?.duration, 8000)
})

test('switching candidates updates the existing toast even when the integer progress is unchanged', () => {
  const task = offlineSubmission({
    task_id: 13,
    status: 'running',
    phase: 'downloading',
    progress: 0,
    can_cancel: true
  })
  const context: DiffContext = {
    tasks: [],
    activity: { tasks: [task] },
    waiting: false,
    isTasksError: false,
    isActivityError: false,
    previous: new Map(),
    dismissed: new Set(),
    announced: new Set(),
    initialized: true
  }
  const first = diffTaskNotifications(context)
  const second = diffTaskNotifications({
    ...context,
    previous: first.nextEntries,
    activity: {
      tasks: [{ ...task, hash: 'replacement', attempt_count: 2, download_state: 'submitting' }]
    }
  })
  assert.equal(second.actions.length, 1)
  assert.equal(second.actions[0]?.id, 'offline:13')
  assert.deepEqual(second.dismissIDs, [])
})

test('exhausted candidates remain an active download with a compact status', () => {
  vi.clearAllMocks()
  const task = offlineSubmission({
    status: 'running',
    phase: 'downloading',
    download_state: 'exhausted',
    can_cancel: true
  })
  assert.equal(downloadStatus(task), '停止换源，继续等待')
  assert.equal(isOfflineTaskActive(task), true)
  notifyOfflineTask(task)
  assert.equal(vi.mocked(toast.info).mock.calls[0]?.[1]?.duration, Infinity)
  assert.equal(vi.mocked(toast.error).mock.calls.length, 0)
})
