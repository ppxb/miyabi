import { useEffect, useRef } from 'react'
import { toast } from 'sonner'

import { useOfflineActivity } from '@/api/offline'
import { useTasks } from '@/api/tasks'
import { sourceKey } from '@/lib/source'
import { useTaskConnection } from './task-events'
import { diffTaskNotifications, type NotificationEntry } from './task-notification-diff'
import { notifyOfflineTask, notifyScanTask } from './task-toast'

export function TaskNotifications() {
  const tasks = useTasks()
  const activity = useOfflineActivity()
  const connection = useTaskConnection()
  const previous = useRef(new Map<string, NotificationEntry>())
  const dismissed = useRef(new Set<string>())
  const scope = useRef<string | undefined>(undefined)
  const initialized = useRef(false)

  useEffect(() => {
    if (!tasks.data || !activity.data) return
    const source = activity.data.source
    const nextScope = sourceKey(source)
    if (scope.current !== nextScope) {
      for (const id of previous.current.keys()) toast.dismiss(id)
      previous.current.clear()
      dismissed.current.clear()
      initialized.current = false
      scope.current = nextScope
    }

    const announced = new Set(toast.getToasts().map(item => String(item.id)))
    const result = diffTaskNotifications({
      tasks: tasks.data,
      activity: activity.data,
      waiting: connection.status !== 'connected',
      isTasksError: tasks.isError,
      isActivityError: activity.isError,
      previous: previous.current,
      dismissed: dismissed.current,
      announced,
      initialized: initialized.current
    })

    for (const dismissID of result.dismissIDs) {
      toast.dismiss(dismissID)
      dismissed.current.delete(dismissID)
    }

    for (const action of result.actions) {
      const callbacks = {
        onDismiss: () => {
          dismissed.current.add(action.id)
        }
      }
      if (action.type === 'notify_scan') {
        notifyScanTask(action.task, { ...callbacks, waiting: action.waiting })
      } else if (action.type === 'notify_offline') {
        notifyOfflineTask(action.task, {
          ...callbacks,
          scan: action.scan,
          waiting: action.waiting
        })
      }
    }

    previous.current = result.nextEntries
    initialized.current = true
  }, [tasks.data, tasks.isError, activity.data, activity.isError, connection.status])

  return null
}
