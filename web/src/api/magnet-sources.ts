import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPut } from './client'

export type MagnetSource = { id: string; enabled: boolean }
const magnetSourcesKey = ['settings', 'magnet-sources'] as const

export const magnetSourceNames: Record<string, string> = {
  sukebei: 'Sukebei',
  projectjav: 'ProjectJav'
}

export function useMagnetSources() {
  return useQuery({
    queryKey: magnetSourcesKey,
    queryFn: ({ signal }) =>
      apiGet<MagnetSource[]>('/api/settings/magnet-sources', undefined, signal)
  })
}

export function useUpdateMagnetSources() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (sources: MagnetSource[]) =>
      apiPut<MagnetSource[]>('/api/settings/magnet-sources', sources),
    onSuccess: sources => {
      client.setQueryData(magnetSourcesKey, sources)
    }
  })
}
