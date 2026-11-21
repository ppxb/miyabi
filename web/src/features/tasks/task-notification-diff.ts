import {
  isOfflineTaskActive,
  type OfflineActivity,
  type OfflineSubmission
} from '@/api/offline'
import { isScanTask, isTaskActive, type BatchTask, type ScanTask, type Task } from '@/api/tasks'

function isBatchTask(task: Task): task is BatchTask {
  return task.type === 'subscription_batch'
}

export type NotificationEntry = {
  active: boolean
  inLibrary?: boolean
  retryable?: boolean
  fingerprint: string
}

export type DiffContext = {
  tasks: Task[]
  activity: OfflineActivity
  waiting: boolean
  isTasksError: boolean
  isActivityError: boolean
  previous: Map<string, NotificationEntry>
  dismissed: Set<string>
  announced: Set<string>
  initialized: boolean
}

export type NotificationAction =
  | { type: 'notify_scan'; id: string; task: ScanTask; waiting: boolean }
  | { type: 'notify_batch'; id: string; task: BatchTask; waiting: boolean }
  | {
      type: 'notify_offline'
      id: string
      task: OfflineSubmission
      scan?: ScanTask
      waiting: boolean
    }

export type DiffResult = {
  actions: NotificationAction[]
  dismissIDs: string[]
  nextEntries: Map<string, NotificationEntry>
}

export const scanToastID = (id: number) => `scan:${id}`

export const offlineToastID = (id: number) => `offline:${id}`

export const batchToastID = (id: number) => `batch:${id}`

export function scanFingerprint(task: ScanTask): string {
  return `${task.status}|${task.progress}|${task.error ?? ''}|${task.scan.stage}|${task.scan.movies}|${task.scan.metadata_total}|${task.scan.metadata_completed}|${task.scan.metadata_retrying ?? 0}|${task.scan.metadata_failed ?? 0}|${task.retry_at ?? ''}|${task.updated_at}|${!!task.can_retry}|${!!task.paused}`
}

export function batchFingerprint(task: BatchTask): string {
  return `${task.status}|${task.progress}|${task.error ?? ''}|${task.batch.processed}|${task.batch.failed}|${task.updated_at}|${!!task.can_retry}`
}

/** Whether a changed entry should raise a toast (active or terminal transition). */
export function diffTaskNotifications(ctx: DiffContext): DiffResult {
  const actions: NotificationAction[] = []
  const dismissIDs: string[] = []
  const nextEntries = new Map<string, NotificationEntry>()

  const shouldNotify = (id: string, active: boolean, old?: NotificationEntry, extra = false) => {
    if (active) return !ctx.dismissed.has(id)
    return !!old?.active || extra || ctx.announced.has(id) || (!old && ctx.initialized)
  }

  const waitingForScan = ctx.waiting || ctx.isTasksError
  const waitingForOffline = ctx.waiting || ctx.isActivityError

  const scans = ctx.tasks.filter(isScanTask)
  const batches = ctx.tasks.filter(isBatchTask)

  for (const task of scans) {
    if (task.offline_task_id) {
      continue
    }
    const id = scanToastID(task.id)
    const active = isTaskActive(task)
    const fp = `${scanFingerprint(task)}|${waitingForScan}`
    const entry: NotificationEntry = { active, fingerprint: fp }
    nextEntries.set(id, entry)

    const old = ctx.previous.get(id)
    if (old?.fingerprint !== fp && shouldNotify(id, active, old)) {
      // A retry may finish between polls; refresh its visible terminal result too.
      actions.push({ type: 'notify_scan', id, task, waiting: waitingForScan })
    }
  }

  for (const task of batches) {
    const id = batchToastID(task.id)
    const active = isTaskActive(task)
    const fp = `${batchFingerprint(task)}|${waitingForScan}`
    const entry: NotificationEntry = { active, fingerprint: fp }
    nextEntries.set(id, entry)

    const old = ctx.previous.get(id)
    if (old?.fingerprint !== fp && shouldNotify(id, active, old)) {
      actions.push({ type: 'notify_batch', id, task, waiting: waitingForScan })
    }
  }

  const scansByID = new Map(scans.map(task => [task.id, task]))
  for (const task of ctx.activity.tasks) {
    const id = offlineToastID(task.task_id)
    const scan = task.scan_task_id ? scansByID.get(task.scan_task_id) : undefined
    const active = isOfflineTaskActive(task)
    const scanPart = active && scan ? scanFingerprint(scan) : ''
    const retryable = !!scan?.can_retry
    const fp = `${task.status}|${task.phase}|${task.processing}|${task.library_id ?? ''}|${task.progress}|${task.error ?? ''}|${task.hash}|${task.download_state ?? ''}|${task.attempt_count ?? ''}|${task.can_cancel ?? ''}|${task.retry_at ?? ''}|${scanPart}|${scan?.updated_at ?? ''}|${retryable}|${waitingForOffline}`
    const entry: NotificationEntry = {
      active,
      inLibrary: task.phase === 'in_library',
      retryable,
      fingerprint: fp
    }
    nextEntries.set(id, entry)

    const old = ctx.previous.get(id)
    if (
      old?.fingerprint !== fp &&
      shouldNotify(id, active, old, !!(old && retryable && !old.retryable))
    ) {
      actions.push({ type: 'notify_offline', id, task, scan, waiting: waitingForOffline })
    }

    if (!active && task.phase !== 'in_library' && old?.inLibrary) {
      dismissIDs.push(id)
    }
  }

  for (const id of ctx.previous.keys()) {
    if (!nextEntries.has(id)) {
      dismissIDs.push(id)
    }
  }

  return { actions, dismissIDs, nextEntries }
}
