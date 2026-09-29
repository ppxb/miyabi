import { MutationCache, QueryCache, QueryClient, type Query } from '@tanstack/react-query'
import { toast } from 'sonner'

import { ApiError, describeApiError, isPanUnauthorized } from './client'
import { libraryKeys } from './library'
import { panKeys } from './pan'

interface MutationMeta extends Record<string, unknown> {
  errorTitle?: string
}

declare module '@tanstack/react-query' {
  interface Register {
    mutationMeta: MutationMeta
  }
}

export function createAppQueryClient() {
  function refreshPanState(error: Error, failedQuery?: Query<unknown, unknown>) {
    const sourceChanged =
      error instanceof ApiError &&
      (error.code === 'PAN_DIRECTORY_REQUIRED' || error.code === 'PAN_SOURCE_CHANGED')
    if (!isPanUnauthorized(error) && !sourceChanged) return

    // Account-query failures must not trigger a refetch of that same query.
    void client.invalidateQueries(
      {
        queryKey: panKeys.account,
        exact: true,
        predicate: query => query !== failedQuery
      },
      { cancelRefetch: false }
    )
    if (!failedQuery) {
      void client.invalidateQueries({ queryKey: libraryKeys.all }, { cancelRefetch: false })
    }
  }

  const client = new QueryClient({
    queryCache: new QueryCache({ onError: refreshPanState }),
    mutationCache: new MutationCache({
      onError: (error, _variables, _context, mutation) => {
        refreshPanState(error)
        // The auth gate handles application-session expiry; local errors stay inline.
        if (error instanceof ApiError && error.code === 'UNAUTHORIZED') return
        if (error.name === 'AbortError') return
        if (mutation.meta?.errorTitle) {
          toast.error(mutation.meta.errorTitle, { description: describeApiError(error) })
        }
      }
    }),
    defaultOptions: {
      queries: { staleTime: 30_000, retry: false, refetchOnWindowFocus: false }
    }
  })
  return client
}
