import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPost, apiPut } from '@/api/client'
import { authKeys } from '@/api/auth'

export type SetupStatus = {
  needs_setup: boolean
  completed: boolean
  sources: string[]
  local_directory: string
  account_configured: boolean
}

export type SetupSource = 'local' | '115'

export const setupKeys = {
  status: ['setup', 'status'] as const
}

export function useSetupStatus() {
  return useQuery({
    queryKey: setupKeys.status,
    queryFn: ({ signal }) => apiGet<SetupStatus>('/api/setup/status', undefined, signal),
    staleTime: Infinity,
    retry: 1
  })
}

export function useSetupAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { username: string; password: string }) =>
      apiPost<{ success: boolean }>('/api/setup/account', input),
    onSuccess: () => {
      queryClient.setQueryData<SetupStatus>(setupKeys.status, status =>
        status ? { ...status, account_configured: true } : status
      )
      void queryClient.invalidateQueries({ queryKey: authKeys.config })
    }
  })
}

export function useSetLocalDirectory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (path: string) => apiPut<{ path: string }>('/api/setup/local-directory', { path }),
    onSuccess: data => {
      queryClient.setQueryData<SetupStatus>(setupKeys.status, status =>
        status ? { ...status, local_directory: data.path } : status
      )
    }
  })
}

export function useCompleteSetup() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (sources: SetupSource[]) =>
      apiPost<{ completed: boolean }>('/api/setup/complete', { sources }),
    onSuccess: (_data, sources) => {
      queryClient.setQueryData<SetupStatus>(setupKeys.status, status =>
        status ? { ...status, needs_setup: false, completed: true, sources } : status
      )
      void queryClient.invalidateQueries({ queryKey: setupKeys.status })
    }
  })
}
