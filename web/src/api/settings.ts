import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPut } from '@/api/client'

type Preferences = { nsfw_mode: boolean }

const preferencesKey = ['settings', 'preferences'] as const

export function usePreferences() {
  return useQuery({
    queryKey: preferencesKey,
    queryFn: ({ signal }) => apiGet<Preferences>('/api/settings/preferences', undefined, signal),
    retry: false
  })
}

export function useSavePreferences() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (preferences: Preferences) =>
      apiPut<Preferences>('/api/settings/preferences', preferences),
    onSuccess: preferences => queryClient.setQueryData(preferencesKey, preferences)
  })
}
