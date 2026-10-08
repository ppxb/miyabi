import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPost, apiPut } from '@/api/client'

export type NetworkConfig = {
  enabled: boolean
  url: string
}

export type NetworkProbeResult = {
  available: boolean
  latency_ms?: number
  error?: string
}

export type NetworkTestResponse = {
  javdb: NetworkProbeResult
  javbus: NetworkProbeResult
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
      const previous = queryClient.getQueryData<NetworkConfig>(networkKeys.config)
      queryClient.setQueryData<NetworkConfig>(networkKeys.config, old => ({
        enabled: next.enabled,
        url: next.url || old?.url || ''
      }))
      return { previous }
    },
    onSuccess: next => {
      queryClient.setQueryData(networkKeys.config, next)
    },
    onError: (_err, _next, context) => {
      if (context?.previous) {
        queryClient.setQueryData(networkKeys.config, context.previous)
      }
      void queryClient.invalidateQueries({ queryKey: networkKeys.config })
    }
  })
}

export function useTestNetwork() {
  return useMutation({
    mutationFn: (candidate: NetworkConfig) =>
      apiPost<NetworkTestResponse>('/api/settings/network/test', candidate)
  })
}
