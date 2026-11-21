import { useQueryClient, type QueryKey } from '@tanstack/react-query'
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

import { notifyUnauthorized } from '@/api/client'
import { movieStateKeys } from '@/api/movie-states'
import { cancelQueryRefresh, refreshQueries } from '@/api/query-refresh'
import { libraryKeys } from '@/api/library'
import { offlineKeys } from '@/api/offline'
import { subscriptionKeys } from '@/api/subscriptions'
import { taskKeys, type Task, type TaskRevisions } from '@/api/tasks'

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
  // Kept across reconnects so a reconnect only refetches the groups that
  // actually changed. The server instance detects a restart (revisions reset).
  const revisionsRef = useRef<TaskRevisions | undefined>(undefined)
  const reconnect = useCallback(() => {
    setConnection('connecting')
    setAttempt(value => value + 1)
    // Prefer the new stream's snapshot; failure or timeout may request HTTP again.
    snapshotRequested.current = false
  }, [])

  useEffect(() => {
    const events = new EventSource('/api/tasks/events')
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let connectionTimer: ReturnType<typeof setTimeout> | undefined
    let probeTimer: ReturnType<typeof setTimeout> | undefined
    let probeController: AbortController | undefined
    let disposed = false

    function markDisconnected() {
      setConnection('disconnected')
      // Reconcile cached "running" tasks once per outage, including a missed completion event.
      if (!snapshotRequested.current) {
        snapshotRequested.current = true
        void refreshQueries(queryClient, taskKeys.all)
      }
    }

    async function restartConnection() {
      if (disposed || probeController || retryTimer) return
      events.close()
      clearTimeout(connectionTimer)
      clearTimeout(retryTimer)
      markDisconnected()

      const controller = new AbortController()
      probeController = controller
      // Keep the deadline active while reading a possible 401 response body.
      probeTimer = setTimeout(() => controller.abort(), 5000)
      try {
        const response = await fetch('/api/tasks/events', {
          signal: controller.signal
        })
        if (disposed) return
        controller.signal.throwIfAborted()
        if (response.status === 401) {
          const payload = (await response.json().catch(() => null)) as { code?: string } | null
          if (disposed) return
          controller.signal.throwIfAborted()
          if (!payload?.code || payload.code === 'UNAUTHORIZED') {
            notifyUnauthorized()
            return
          }
        }
      } catch {
        // Network error, abort, or offline falls through to reconnect retry
      } finally {
        clearTimeout(probeTimer)
        controller.abort()
        probeTimer = undefined
        probeController = undefined
      }

      if (!disposed) {
        retryTimer = setTimeout(() => setAttempt(value => value + 1), 3000)
      }
    }

    function waitForActivity(timeout = 45_000) {
      if (disposed || probeController || retryTimer) return
      clearTimeout(connectionTimer)
      connectionTimer = setTimeout(() => void restartConnection(), timeout)
    }

    events.addEventListener('tasks', async (event: MessageEvent<string>) => {
      if (disposed) return
      const snapshot = JSON.parse(event.data) as Task[]
      await cancelQueryRefresh(queryClient, taskKeys.all)
      if (disposed || events.readyState !== EventSource.OPEN) return
      queryClient.setQueryData(taskKeys.all, snapshot)
      snapshotRequested.current = false
      waitForActivity()
      setConnection('connected')
    })

    events.addEventListener('changes', (event: MessageEvent<string>) => {
      if (disposed) return
      const next = JSON.parse(event.data) as TaskRevisions
      waitForActivity()
      // Revisions reset to zero when the server restarts. A changed instance
      // means the stored counters are stale, so re-baseline (refetch all).
      const stored = revisionsRef.current
      const prev = stored?.instance === next.instance ? stored : undefined
      const keys: QueryKey[] = []
      const libraryChanged = prev?.library !== next.library
      if (libraryChanged) keys.push(libraryKeys.all)
      if (libraryChanged || prev?.offline !== next.offline) {
        keys.push(movieStateKeys.all, offlineKeys.all)
      }
      if (prev?.monitor !== next.monitor) keys.push(subscriptionKeys.all)
      revisionsRef.current = next
      void refreshQueries(queryClient, ...keys)
    })

    // The server sends a heartbeat every 15 seconds, even when no task changes.
    events.addEventListener('ping', () => waitForActivity())
    events.onerror = () => {
      if (disposed) return
      // Keep revisionsRef across reconnects: the next changes event is compared
      // against it, and re-baselined only when the server instance changed.
      // Native EventSource retries transport interruptions, but not a terminal CLOSED state.
      if (events.readyState === EventSource.CLOSED) void restartConnection()
      else {
        markDisconnected()
        waitForActivity(15_000)
      }
    }
    waitForActivity(15_000)

    return () => {
      disposed = true
      events.close()
      probeController?.abort()
      clearTimeout(probeTimer)
      clearTimeout(retryTimer)
      clearTimeout(connectionTimer)
    }
  }, [queryClient, attempt])

  const value = useMemo(() => ({ status: connection, reconnect }), [connection, reconnect])
  return <TaskConnectionContext value={value}>{children}</TaskConnectionContext>
}
