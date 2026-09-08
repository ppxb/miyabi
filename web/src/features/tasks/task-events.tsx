import { useQueryClient } from '@tanstack/react-query'
import { createContext, useContext, useEffect, useState, type PropsWithChildren } from 'react'

import { invalidateMovieStates } from '@/api/discover'
import { libraryKeys } from '@/api/library'
import { offlineKeys } from '@/api/offline'
import { taskKeys, type ScanTask, type TaskRevisions } from '@/api/tasks'

type ConnectionState = 'connecting' | 'connected' | 'reconnecting'
const TaskConnectionContext = createContext<ConnectionState>('connecting')

export function useTaskConnection() {
  return useContext(TaskConnectionContext)
}

export function TaskEventsProvider({ children }: PropsWithChildren) {
  const queryClient = useQueryClient()
  const [connection, setConnection] = useState<ConnectionState>('connecting')

  useEffect(() => {
    const events = new EventSource('/api/tasks/events')
    let revisions: TaskRevisions | undefined
    let refreshTimer: ReturnType<typeof setTimeout> | undefined
    let libraryChanged = false
    let offlineChanged = false

    function refreshData() {
      refreshTimer = undefined
      if (libraryChanged) void queryClient.invalidateQueries({ queryKey: libraryKeys.all })
      if (libraryChanged || offlineChanged) {
        void invalidateMovieStates(queryClient)
        void queryClient.invalidateQueries({ queryKey: offlineKeys.all })
      }
      libraryChanged = false
      offlineChanged = false
    }

    events.addEventListener('tasks', async (event: MessageEvent<string>) => {
      const snapshot = JSON.parse(event.data) as ScanTask[]
      await queryClient.cancelQueries({ queryKey: taskKeys.all, exact: true })
      if (events.readyState === EventSource.CLOSED) return
      queryClient.setQueryData(taskKeys.all, snapshot)
      setConnection('connected')
    })

    events.addEventListener('changes', (event: MessageEvent<string>) => {
      const next = JSON.parse(event.data) as TaskRevisions
      const reconnecting = revisions === undefined
      libraryChanged ||= revisions?.library !== next.library
      offlineChanged ||= revisions?.offline !== next.offline
      revisions = next
      if (!libraryChanged && !offlineChanged) return
      if (reconnecting) {
        clearTimeout(refreshTimer)
        refreshData()
      } else if (refreshTimer === undefined) {
        refreshTimer = setTimeout(refreshData, 1000)
      }
    })

    events.onerror = () => {
      revisions = undefined
      setConnection('reconnecting')
    }
    return () => {
      events.close()
      clearTimeout(refreshTimer)
    }
  }, [queryClient])

  return <TaskConnectionContext value={connection}>{children}</TaskConnectionContext>
}
