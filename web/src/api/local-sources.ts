import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet, apiPatch, apiPost } from '@/api/client'
import { taskKeys, type ScanTask, type Task } from '@/api/tasks'

// A local media directory that Miyabi scans alongside the 115 library.
export type LocalSource = {
  id: string
  name: string
  path: string
  enabled: boolean
}

export type LocalSourceUpdate = {
  name?: string
  enabled?: boolean
}

export const localSourceKeys = {
  all: ['settings', 'local-sources'] as const
}

export function useLocalSources() {
  return useQuery({
    queryKey: localSourceKeys.all,
    queryFn: ({ signal }) =>
      apiGet<LocalSource[]>('/api/settings/local-sources', undefined, signal),
    staleTime: 15_000,
    refetchOnMount: 'always'
  })
}

export function useAddLocalSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { name: string; path: string }) =>
      apiPost<LocalSource>('/api/settings/local-sources', input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: localSourceKeys.all })
  })
}

export function useUpdateLocalSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...update }: LocalSourceUpdate & { id: string }) =>
      apiPatch<LocalSource>(`/api/settings/local-sources/${encodeURIComponent(id)}`, update),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: localSourceKeys.all })
  })
}

export function useRemoveLocalSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiDelete<null>(`/api/settings/local-sources/${encodeURIComponent(id)}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: localSourceKeys.all })
  })
}

// The queued scan joins the task list so it shows up on the tasks page too.
export function useScanLocalSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiPost<ScanTask>('/api/library/local-scan', { source_id: id }),
    onSuccess: task => {
      queryClient.setQueryData<Task[]>(taskKeys.all, tasks => [
        task,
        ...(tasks ?? []).filter(item => item.id !== task.id)
      ])
      return queryClient.invalidateQueries({ queryKey: taskKeys.all })
    }
  })
}
