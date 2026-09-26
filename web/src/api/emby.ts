import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPost, apiPut } from '@/api/client'

export type EmbyConfig = {
  enabled: boolean
  server_url: string
  api_key: string
  media_path: string
  local_dir?: string
  sync_actors?: boolean
  public_url?: string
}

export type EmbyServerInfo = {
  server_name: string
  version: string
  id: string
}

export const embyKeys = {
  config: ['settings', 'emby'] as const
}

export function useEmbyConfig() {
  return useQuery({
    queryKey: embyKeys.config,
    queryFn: ({ signal }) => apiGet<EmbyConfig>('/api/settings/emby', undefined, signal),
    staleTime: 15_000,
    refetchOnMount: 'always'
  })
}

export function useUpdateEmbyConfig() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (config: EmbyConfig) => apiPut<EmbyConfig>('/api/settings/emby', config),
    onSuccess: next => {
      queryClient.setQueryData(embyKeys.config, next)
    },
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: embyKeys.config })
    }
  })
}

export function useTestEmbyConfig() {
  return useMutation({
    mutationFn: (config?: Partial<EmbyConfig>) =>
      apiPost<EmbyServerInfo>('/api/settings/emby/test', config)
  })
}
