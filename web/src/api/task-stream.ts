import { createContext, useContext } from 'react'

export type ConnectionState = 'connecting' | 'connected' | 'disconnected'
export type TaskConnection = { status: ConnectionState; reconnect: () => void }

export const TaskConnectionContext = createContext<TaskConnection | null>(null)

export function useTaskConnection() {
  const connection = useContext(TaskConnectionContext)
  if (connection === null) throw new Error('TaskEventsProvider is missing')
  return connection
}

// The task SSE stream pushes authoritative snapshots, so mutations only need
// an HTTP task refresh while the stream is unavailable. Data hooks stay usable
// without the provider (tests, embedded views) and then keep refreshing.
export function useTaskStreamConnected() {
  return useContext(TaskConnectionContext)?.status === 'connected'
}
