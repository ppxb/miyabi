import { useEffect, useRef } from 'react'
import { toast } from 'sonner'

import { isOfflineTaskActive, useOfflineActivity } from '@/api/offline'
import { isTaskActive, useTasks, type ScanTask } from '@/api/tasks'
import { useTaskConnection } from './task-events'
import { notifyOfflineTask, notifyScanTask, offlineToastID, scanToastID } from './task-toast'

type NotificationState = { version: string; active: boolean; playable?: boolean }

export function TaskNotifications() {
  const tasks = useTasks()
  const activity = useOfflineActivity()
  const connection = useTaskConnection()
  const previous = useRef(new Map<string, NotificationState>())
  const dismissed = useRef(new Set<string>())
  const scope = useRef<string | undefined>(undefined)
  const initialized = useRef(false)

  useEffect(() => {
    if (!tasks.data || !activity.data) return
    const source = activity.data.source
    const nextScope = source ? `${source.account_id}:${source.directory.id}` : ''
    if (scope.current !== nextScope) {
      for (const id of previous.current.keys()) toast.dismiss(id)
      previous.current.clear()
      dismissed.current.clear()
      initialized.current = false
      scope.current = nextScope
    }

    const current = new Map<string, NotificationState>()
    const announced = new Set(toast.getToasts().map(item => item.id))
    function update(id: string, state: NotificationState, notify: () => void) {
      current.set(id, state)
      const old = previous.current.get(id)
      if (old?.version === state.version) return
      // Restore active work on load, but only announce newly observed completions.
      if (
        state.active
          ? !dismissed.current.has(id)
          : old?.active || (!old && (initialized.current || announced.has(id)))
      ) {
        notify()
      }
    }
    function callbacks(id: string) {
      return {
        onDismiss: () => {
          dismissed.current.add(id)
        }
      }
    }

    const waiting = connection.status !== 'connected'
    for (const task of tasks.data) {
      if (
        !source ||
        task.offline_task_id ||
        task.source.account_id !== source.account_id ||
        task.source.directory.id !== source.directory.id
      )
        continue
      const id = scanToastID(task.id)
      update(
        id,
        {
          active: isTaskActive(task),
          version: JSON.stringify([scanVersion(task), waiting, tasks.isError])
        },
        () => notifyScanTask(task, { ...callbacks(id), waiting: waiting || tasks.isError })
      )
    }

    const scans = new Map(tasks.data.map(task => [task.id, task]))
    for (const task of activity.data.tasks) {
      const id = offlineToastID(task.task_id)
      const scan = task.scan_task_id ? scans.get(task.scan_task_id) : undefined
      const active = isOfflineTaskActive(task)
      update(
        id,
        {
          active,
          playable: task.phase === 'in_library',
          version: JSON.stringify([
            task.status,
            task.phase,
            task.library_id,
            task.progress,
            task.error,
            active && scan ? scanVersion(scan) : undefined,
            waiting,
            activity.isError
          ])
        },
        () =>
          notifyOfflineTask(task, {
            ...callbacks(id),
            scan,
            waiting: waiting || activity.isError
          })
      )
      // A later scan can remove a file; its old toast must no longer offer playback.
      if (!active && task.phase !== 'in_library' && previous.current.get(id)?.playable) {
        toast.dismiss(id)
      }
    }
    for (const id of previous.current.keys()) {
      if (!current.has(id)) {
        toast.dismiss(id)
        dismissed.current.delete(id)
      }
    }
    previous.current = current
    initialized.current = true
  }, [tasks.data, tasks.isError, activity.data, activity.isError, connection.status])

  return null
}

function scanVersion(task: ScanTask) {
  return [
    task.status,
    task.progress,
    task.error,
    task.scan.stage,
    task.scan.movies,
    task.scan.metadata_total,
    task.scan.metadata_completed
  ]
}
