import { hashKey, type QueryClient, type QueryKey } from '@tanstack/react-query'

type Refresh = { started: boolean; cancelled: boolean; promise: Promise<void> }
const refreshes = new WeakMap<QueryClient, Map<string, Refresh>>()

// Share queued invalidations across mutations and SSE. A change during a read
// queues one following read, so it cannot be lost by merely joining an older one.
export function refreshQueries(client: QueryClient, ...keys: QueryKey[]): Promise<void> {
  let pending = refreshes.get(client)
  if (!pending) {
    pending = new Map()
    refreshes.set(client, pending)
  }
  const queue = pending
  return Promise.all(
    keys.map(queryKey => {
      const key = hashKey(queryKey)
      const previous = queue.get(key)
      if (previous && !previous.started && !previous.cancelled) return previous.promise

      const refresh: Refresh = {
        started: false,
        cancelled: false,
        promise: (previous?.promise.catch(() => {}) ?? Promise.resolve()).then(async () => {
          try {
            if (refresh.cancelled) return
            // Discard reads started before this change, including initial reads
            // without cached data. Subsequent scheduled reads wait for this one.
            await client.cancelQueries({ queryKey })
            if (refresh.cancelled) return
            refresh.started = true
            await client.invalidateQueries({ queryKey }, { cancelRefetch: false })
          } finally {
            if (queue.get(key) === refresh) queue.delete(key)
          }
        })
      }
      queue.set(key, refresh)
      return refresh.promise
    })
  ).then(() => {})
}

// An authoritative SSE snapshot supersedes both the current task read and its
// queued follow-up. Refresh requests arriving after this call remain eligible.
export function cancelQueryRefresh(client: QueryClient, queryKey: QueryKey) {
  const pending = refreshes.get(client)?.get(hashKey(queryKey))
  if (pending) pending.cancelled = true
  return client.cancelQueries({ queryKey })
}

// Apply an optimistic cache update from a mutation response only when the cache
// still matches the snapshot captured before the mutation. A concurrent read or
// SSE refresh (fetchStatus !== 'idle' or newer data) supersedes the response.
export function updateIfCurrent<T>(
  client: QueryClient,
  queryKey: QueryKey,
  previous: T | undefined,
  update: (current: T | undefined) => T | undefined
): void {
  const state = client.getQueryState(queryKey)
  if ((state?.fetchStatus ?? 'idle') !== 'idle' || state?.data !== previous) return
  client.setQueryData<T>(queryKey, update)
}
