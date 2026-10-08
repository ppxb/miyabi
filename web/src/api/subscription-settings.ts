import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPut } from '@/api/client'

export type PreferenceLevel = 'preferred' | 'required' | 'any'
export type UncensoredFilter = 'preferred' | 'required' | 'exclude' | 'any'

export type MagnetPreferences = {
  subtitle: PreferenceLevel
  hd: PreferenceLevel
  uncensored: UncensoredFilter
}

export type SubscriptionConfig = {
  movie_auto_download: boolean
  actor_auto_download: boolean
  check_time: string
  preferences: MagnetPreferences
  download: DownloadConfig
}

export type DownloadConfig = {
  auto_switch: boolean
  zero_progress_minutes: number
  stalled_minutes: number
  completion_grace_minutes: number
  max_attempts: number
}

export const subscriptionSettingsKey = ['settings', 'subscription'] as const

export function useSubscriptionSettings() {
  return useQuery({
    queryKey: subscriptionSettingsKey,
    queryFn: ({ signal }) =>
      apiGet<SubscriptionConfig>('/api/settings/subscription', undefined, signal),
    staleTime: 15_000,
    refetchOnMount: 'always'
  })
}

export function useUpdateSubscriptionSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (config: SubscriptionConfig) =>
      apiPut<SubscriptionConfig>('/api/settings/subscription', config),
    onSuccess: next => queryClient.setQueryData(subscriptionSettingsKey, next),
    onError: () => queryClient.invalidateQueries({ queryKey: subscriptionSettingsKey })
  })
}
