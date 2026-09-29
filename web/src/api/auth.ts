import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPost } from '@/api/client'

export type AccessGateConfig = {
  enabled: boolean
  authenticated: boolean
}

export type LoginResponse = {
  success: boolean
  expires_at?: number
}

export const authKeys = {
  config: ['auth', 'config'] as const
}

export function useAccessGateConfig(enabled = true) {
  return useQuery({
    queryKey: authKeys.config,
    queryFn: ({ signal }) => apiGet<AccessGateConfig>('/api/auth/config', undefined, signal),
    enabled,
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

export function useAccessGateLogout() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<{ success: boolean }>('/api/auth/logout'),
    onSuccess: () => {
      queryClient.setQueryData<AccessGateConfig>(authKeys.config, prev => ({
        enabled: prev?.enabled ?? true,
        authenticated: false
      }))
    }
  })
}
