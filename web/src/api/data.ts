import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet } from '@/api/client'

export type DataInfo = {
  data_directory: string
  database_size_bytes: number
  cache: {
    size_bytes: number
    entry_count: number
    unused_size_bytes: number
    unused_entry_count: number
  }
}

const dataKeys = { system: ['settings', 'system'] as const }

export function useDataInfo() {
  return useQuery({
    queryKey: dataKeys.system,
    queryFn: ({ signal }) => apiGet<DataInfo>('/api/settings/system', undefined, signal),
    staleTime: 15_000,
    retry: false,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false
  })
}

export function useClearCache() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiDelete<DataInfo>('/api/settings/cache'),
    onSuccess: async info => {
      // An earlier statistics response must not restore the pre-cleanup counts.
      await queryClient.cancelQueries({ queryKey: dataKeys.system })
      queryClient.setQueryData(dataKeys.system, info)
    },
    onError: () => {
      // A filesystem error may occur after some unused files were removed.
      void queryClient.invalidateQueries({ queryKey: dataKeys.system })
    }
  })
}
