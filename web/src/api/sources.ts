import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPut } from '@/api/client'

export type SourceKind = 'local' | '115' | 'pikpak'

export type StorageSource = {
  id: string
  kind: SourceKind
  name: string
  enabled: boolean
  config?: Record<string, string>
}

export const sourcesKey = ['settings', 'sources'] as const

export function useSources() {
  return useQuery({
    queryKey: sourcesKey,
    queryFn: ({ signal }) => apiGet<StorageSource[]>('/api/settings/sources', undefined, signal)
  })
}

export function useSaveSources() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (sources: StorageSource[]) =>
      apiPut<StorageSource[]>('/api/settings/sources', sources),
    onSuccess: next => queryClient.setQueryData(sourcesKey, next),
    onError: () => queryClient.invalidateQueries({ queryKey: sourcesKey })
  })
}
