import {
  mutationOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient
} from '@tanstack/react-query'

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
    staleTime: 15_000
  })
}

export function updateEmbyConfigOptions(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: (config: EmbyConfig) => apiPut<EmbyConfig>('/api/settings/emby', config),
    onMutate: async next => {
      await queryClient.cancelQueries({ queryKey: embyKeys.config })
      const previous = queryClient.getQueryData<EmbyConfig>(embyKeys.config)
      queryClient.setQueryData<EmbyConfig>(embyKeys.config, {
        ...previous,
        ...next,
        local_dir: next.local_dir || previous?.local_dir
      })
      return { previous }
    },
    onSuccess: next => {
      queryClient.setQueryData(embyKeys.config, next)
    },
    onError: (_error, _next, context) => {
      if (context?.previous) queryClient.setQueryData(embyKeys.config, context.previous)
      // The backend can save settings before reporting a scan enqueue failure.
      void queryClient.invalidateQueries({ queryKey: embyKeys.config })
    }
  })
}

export function useUpdateEmbyConfig() {
  return useMutation(updateEmbyConfigOptions(useQueryClient()))
}

export function useTestEmbyConfig() {
  return useMutation({
    mutationFn: (config?: Partial<EmbyConfig>) =>
      apiPost<EmbyServerInfo>('/api/settings/emby/test', config)
  })
}
