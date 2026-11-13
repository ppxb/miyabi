import {
  mutationOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient
} from '@tanstack/react-query'

import { apiGet, apiPost } from '@/api/client'

export type AccessGateConfig = {
  enabled: boolean
  authenticated: boolean
  username_required?: boolean
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

export function accessGateLoginOptions(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: (input: string | { username: string; password: string }) =>
      apiPost<LoginResponse>(
        '/api/auth/login',
        typeof input === 'string' ? { password: input } : input
      ),
    onSuccess: async (_data, input) => {
      await queryClient.cancelQueries({ queryKey: authKeys.config })
      queryClient.setQueryData<AccessGateConfig>(authKeys.config, {
        enabled: true,
        authenticated: true,
        username_required: typeof input !== 'string' && input.username !== ''
      })
    }
  })
}

export function useAccessGateLogin() {
  return useMutation(accessGateLoginOptions(useQueryClient()))
}
