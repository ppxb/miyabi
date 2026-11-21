import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPut } from '@/api/client'

export type NetworkConfig = {
  enabled: boolean
  url: string
}

export const networkKeys = {
  config: ['settings', 'network'] as const
}

export function useNetworkConfig() {
  return useQuery({
    queryKey: networkKeys.config,
    queryFn: ({ signal }) => apiGet<NetworkConfig>('/api/settings/network', undefined, signal),
    staleTime: 15_000
  })
}

export function useUpdateNetworkConfig() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (config: NetworkConfig) => apiPut<NetworkConfig>('/api/settings/network', config),
    onMutate: async next => {
      await queryClient.cancelQueries({ queryKey: networkKeys.config })
      queryClient.setQueryData<NetworkConfig>(networkKeys.config, old => ({
        enabled: next.enabled,
        url: next.url || old?.url || ''
      }))
    },
    onSuccess: next => {
      queryClient.setQueryData(networkKeys.config, next)
    },
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: networkKeys.config })
    }
  })
}
