import { useQueryClient } from '@tanstack/react-query'
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type PropsWithChildren
} from 'react'

import { invalidateMovieStates } from '@/api/movie-state-cache'
import { libraryKeys } from '@/api/library'
import { offlineKeys } from '@/api/offline'
import { taskKeys, type ScanTask, type TaskRevisions } from '@/api/tasks'

type ConnectionState = 'connecting' | 'connected' | 'disconnected'
type TaskConnection = { status: ConnectionState; reconnect: () => void }
const TaskConnectionContext = createContext<TaskConnection | null>(null)

export function useTaskConnection() {
  const connection = useContext(TaskConnectionContext)
  if (connection === null) throw new Error('TaskEventsProvider is missing')
  return connection
}

export function TaskEventsProvider({ children }: PropsWithChildren) {
  const queryClient = useQueryClient()
  const [connection, setConnection] = useState<ConnectionState>('connecting')
  const [attempt, setAttempt] = useState(0)
  const snapshotRequested = useRef(false)
  const reconnect = useCallback(() => {
    setConnection('connecting')
    setAttempt(value => value + 1)
    snapshotRequested.current = true
    void queryClient.invalidateQueries({ queryKey: taskKeys.all, exact: true })
  }, [queryClient])

  useEffect(() => {
    const events = new EventSource('/api/tasks/events')
    let revisions: TaskRevisions | undefined
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let connectionTimer: ReturnType<typeof setTimeout> | undefined
    let libraryChanged = false
    let offlineChanged = false
    let refreshing = false
    let disposed = false

    function markDisconnected() {
      setConnection('disconnected')
      // Reconcile cached "running" tasks once per outage, including a missed completion event.
      if (!snapshotRequested.current) {
        snapshotRequested.current = true
        void queryClient.invalidateQueries({ queryKey: taskKeys.all, exact: true })
      }
    }

    function restartConnection() {
      events.close()
      clearTimeout(connectionTimer)
      clearTimeout(retryTimer)
      markDisconnected()
      retryTimer = setTimeout(() => setAttempt(value => value + 1), 3000)
    }

    function waitForActivity(timeout = 45_000) {
      clearTimeout(connectionTimer)
      connectionTimer = setTimeout(restartConnection, timeout)
    }

    async function refreshData() {
      if (refreshing) return
      refreshing = true
      try {
        // Refresh immediately, then reconcile once more if changes arrive
        // during the request. Bursts do not cancel each other's responses.
        while (!disposed && (libraryChanged || offlineChanged)) {
          const refreshLibrary = libraryChanged
          libraryChanged = false
          offlineChanged = false
          await Promise.all([
            invalidateMovieStates(queryClient),
            queryClient.invalidateQueries({ queryKey: offlineKeys.all }),
            refreshLibrary
              ? queryClient.invalidateQueries({ queryKey: libraryKeys.all })
              : Promise.resolve()
          ])
        }
      } finally {
        refreshing = false
      }
    }

    events.addEventListener('tasks', async (event: MessageEvent<string>) => {
      const snapshot = JSON.parse(event.data) as ScanTask[]
      await queryClient.cancelQueries({ queryKey: taskKeys.all, exact: true })
      if (events.readyState !== EventSource.OPEN) return
      queryClient.setQueryData(taskKeys.all, snapshot)
      snapshotRequested.current = false
      waitForActivity()
      setConnection('connected')
    })

    events.addEventListener('changes', (event: MessageEvent<string>) => {
      const next = JSON.parse(event.data) as TaskRevisions
      waitForActivity()
      libraryChanged ||= revisions?.library !== next.library
      offlineChanged ||= revisions?.offline !== next.offline
      revisions = next
      void refreshData()
    })

    // The server sends a heartbeat every 15 seconds, even when no task changes.
    events.addEventListener('ping', () => waitForActivity())
    events.onerror = () => {
      revisions = undefined
      // Native EventSource retries transport interruptions, but not a terminal CLOSED state.
      if (events.readyState === EventSource.CLOSED) restartConnection()
      else {
        markDisconnected()
        waitForActivity(15_000)
      }
    }
    waitForActivity(15_000)

    return () => {
      disposed = true
      events.close()
      clearTimeout(retryTimer)
      clearTimeout(connectionTimer)
    }
  }, [queryClient, attempt])

  const value = useMemo(() => ({ status: connection, reconnect }), [connection, reconnect])
  return <TaskConnectionContext value={value}>{children}</TaskConnectionContext>
}
