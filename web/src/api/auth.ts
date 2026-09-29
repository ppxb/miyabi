import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPost } from '@/api/client'

export type AccessGateConfig = {
  enabled: boolean
  authenticated: boolean
}

export type LoginResponse = {
  success: boolean
}

export const authKeys = {
  config: ['auth', 'config'] as const
}

export function useAccessGateConfig() {
  return useQuery({
    queryKey: authKeys.config,
    queryFn: ({ signal }) => apiGet<AccessGateConfig>('/api/auth/config', undefined, signal),
    staleTime: 60_000
  })
}

export function useAccessGateLogin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (password: string) => apiPost<LoginResponse>('/api/auth/login', { password }),
    onSuccess: () => {
      queryClient.setQueryData<AccessGateConfig>(authKeys.config, {
        enabled: true,
        authenticated: true
      })
    }
  })
}
