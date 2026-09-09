import { useMutation, useQuery } from '@tanstack/react-query'

import { apiGet, apiPost } from '@/api/client'

export function useAccessGateConfig(enabled: boolean) {
  return useQuery({
    queryKey: ['auth', 'config'],
    queryFn: ({ signal }) => apiGet<{ enabled: boolean }>('/api/auth/config', undefined, signal),
    enabled,
    staleTime: Infinity,
    retry: false
  })
}

export function useAccessGateLogin() {
  return useMutation({
    mutationFn: (password: string) => apiPost<{ success: boolean }>('/api/auth/login', { password })
  })
}
