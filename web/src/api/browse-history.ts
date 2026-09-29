import {
  mutationOptions,
  queryOptions,
  useMutation,
  useMutationState,
  useQuery,
  useQueryClient,
  type QueryClient
} from '@tanstack/react-query'
import { useEffect } from 'react'

import { ApiError, apiGet, apiPost } from '@/api/client'

const viewedPath = '/api/discover/viewed'
const maxViewedMovies = 5000
export const browseHistoryKeys = {
  viewed: ['browse-history', 'viewed'] as const,
  record: (id: string) => ['browse-history', 'record', id] as const
}

function retryHistory(failures: number, error: Error) {
  return failures < 2 && !(error instanceof ApiError && error.status < 500)
}

export const viewedMoviesOptions = queryOptions({
  queryKey: browseHistoryKeys.viewed,
  queryFn: async ({ signal, client }) => {
    const before = new Set(client.getQueryData<string[]>(browseHistoryKeys.viewed))
    const ids = await apiGet<string[]>(viewedPath, undefined, signal)
    // Preserve confirmed writes made while this GET was in flight, including
    // the first load. Everything else follows the server's ordering/eviction.
    const added = (client.getQueryData<string[]>(browseHistoryKeys.viewed) ?? []).filter(
      id => !before.has(id)
    )
    return [...new Set([...added, ...ids])].slice(0, maxViewedMovies)
  },
  staleTime: 60_000,
  retry: retryHistory,
  refetchOnWindowFocus: true
})

export function recordMovieViewOptions(client: QueryClient, id: string) {
  return mutationOptions({
    mutationKey: browseHistoryKeys.record(id),
    mutationFn: () => apiPost<null>(viewedPath, { ids: [id] }),
    retry: retryHistory,
    onSuccess: () => {
      const updatedAt = client.getQueryState(browseHistoryKeys.viewed)?.dataUpdatedAt ?? 0
      client.setQueryData<string[]>(
        browseHistoryKeys.viewed,
        ids => [id, ...(ids ?? []).filter(value => value !== id)].slice(0, maxViewedMovies),
        { updatedAt }
      )
      // Keep the last server-read age: local writes must not postpone normal
      // stale-on-mount/focus synchronization, or trigger a GET per view.
    }
  })
}

export function useIsMovieViewed(id?: string): boolean {
  const cleanID = id?.trim() ?? ''
  const viewed = useQuery({ ...viewedMoviesOptions, select: ids => ids.includes(cleanID) })
  // Pending mutations provide the optimistic badge without changing server data.
  // A failed write disappears naturally, without rolling back another movie's view.
  const pending = useMutationState({
    filters: { mutationKey: browseHistoryKeys.record(cleanID), exact: true, status: 'pending' },
    select: () => true
  })
  return !!cleanID && (viewed.data === true || pending.length > 0)
}

export function useRecordMovieView(id: string): void {
  const client = useQueryClient()
  const cleanID = id.trim()
  const { mutate } = useMutation(recordMovieViewOptions(client, cleanID))
  useEffect(() => {
    if (!cleanID || client.getQueryData<string[]>(browseHistoryKeys.viewed)?.includes(cleanID))
      return
    if (client.isMutating({ mutationKey: browseHistoryKeys.record(cleanID), exact: true })) return
    mutate()
  }, [client, cleanID, mutate])
}
