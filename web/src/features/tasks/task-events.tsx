import { useQueryClient } from '@tanstack/react-query'
import { createContext, useContext, useEffect, useState, type PropsWithChildren } from 'react'

import { invalidateMovieStates } from '@/api/discover'
import { libraryKeys } from '@/api/library'
import { offlineKeys } from '@/api/offline'
import { isTaskActive, taskKeys, type ScanTask } from '@/api/tasks'

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
    let refreshTimer: ReturnType<typeof setTimeout> | undefined

    function refreshLibrary(cancelRefetch: boolean) {
      void queryClient.invalidateQueries({ queryKey: libraryKeys.all }, { cancelRefetch })
      void invalidateMovieStates(queryClient, cancelRefetch)
      void queryClient.invalidateQueries({ queryKey: offlineKeys.all }, { cancelRefetch })
    }

    events.addEventListener('tasks', async (event: MessageEvent<string>) => {
      const snapshot = JSON.parse(event.data) as ScanTask[]
      // An older GET response must not overwrite a newer pushed snapshot.
      await queryClient.cancelQueries({ queryKey: taskKeys.all, exact: true })
      if (events.readyState === EventSource.CLOSED) return
      const previous = queryClient.getQueryData<ScanTask[]>(taskKeys.all)
      queryClient.setQueryData(taskKeys.all, snapshot)
      setConnection('connected')

      const changed = snapshot.some(task => {
        const old = previous?.find(item => item.id === task.id)
        return old?.updated_at !== task.updated_at
      })
      if (!changed) return
      const finished = snapshot.some(
        task =>
          !isTaskActive(task) && previous?.find(item => item.id === task.id)?.status !== task.status
      )
      if (finished) {
        clearTimeout(refreshTimer)
        refreshTimer = undefined
        refreshLibrary(true)
        return
      }
      if (refreshTimer !== undefined) return

      // Coalesce page updates without canceling an in-flight library request.
      refreshTimer = setTimeout(() => {
        refreshTimer = undefined
        refreshLibrary(false)
      }, 1000)
    })
    events.onerror = () => setConnection('reconnecting')

    return () => {
      events.close()
      clearTimeout(refreshTimer)
    }
  }, [queryClient])

  return <TaskConnectionContext value={connection}>{children}</TaskConnectionContext>
}
