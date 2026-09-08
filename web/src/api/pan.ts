import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'

import { ApiError, apiDelete, apiGet, apiPost, apiPut } from '@/api/client'
import { invalidateMovieStates } from '@/api/discover'

export type PanDirectory = {
  id: string
  name: string
  path: string
}

type PanSpaceAmount = {
  bytes: number
  formatted: string
}

export type PanAccount = {
  id: string
  name: string
  avatar: string
  level: string
  space: {
    total: PanSpaceAmount
    used: PanSpaceAmount
    remaining: PanSpaceAmount
  }
}

export type PanAccountStatus = {
  connected: boolean
  account?: PanAccount
  directory?: PanDirectory
}

type PanFilePage = {
  files: { id: string; name: string; is_directory: boolean }[]
  path: { id: string; name: string }[]
  total: number
  has_more: boolean
}

export type PanLoginSession = {
  id: string
  qr_code: string
}

type PanLoginStatus = {
  state: 'waiting' | 'scanned' | 'authorized' | 'expired' | 'canceled'
}

export const panKeys = {
  account: ['pan', 'account'] as const,
  login: (id: string) => ['pan', 'login', id] as const,
  fileLists: ['pan', 'files'] as const,
  files: (accountID: string, directoryID: string, page: number) =>
    ['pan', 'files', accountID, directoryID, page] as const
}

export function invalidatePanSource(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['library'] })
  void queryClient.invalidateQueries({ queryKey: ['offline'] })
  void invalidateMovieStates(queryClient)
}

export function usePanAccount(enabled = true) {
  return useQuery({
    queryKey: panKeys.account,
    queryFn: ({ signal }) => apiGet<PanAccountStatus>('/api/pan/account', undefined, signal),
    enabled,
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useBeginPanLogin() {
  return useMutation({
    mutationFn: () => apiPost<PanLoginSession>('/api/pan/login'),
    gcTime: 0
  })
}

export function usePanLoginStatus(id: string) {
  return useQuery({
    queryKey: panKeys.login(id),
    queryFn: ({ signal }) =>
      apiGet<PanLoginStatus>(`/api/pan/login/${encodeURIComponent(id)}`, undefined, signal),
    enabled: id !== '',
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    refetchInterval: query => {
      if (query.state.status === 'error') return false
      const state = query.state.data?.state
      return !state || state === 'waiting' || state === 'scanned' ? 1500 : false
    }
  })
}

export function useDisconnectPan() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiDelete<PanAccountStatus>('/api/pan/account'),
    onSuccess: async status => {
      queryClient.setQueryData(panKeys.account, status)
      invalidatePanSource(queryClient)
      await queryClient.cancelQueries({ queryKey: panKeys.fileLists })
      queryClient.removeQueries({ queryKey: panKeys.fileLists })
    }
  })
}

export function usePanFiles(accountID: string, directoryID: string, page: number) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: panKeys.files(accountID, directoryID, page),
    queryFn: ({ signal }) =>
      apiGet<PanFilePage>('/api/pan/files', { directory_id: directoryID, page }, signal),
    retry: false,
    refetchOnWindowFocus: false
  })
  useEffect(() => {
    if (query.error instanceof ApiError && query.error.status === 401) {
      void queryClient.invalidateQueries({ queryKey: panKeys.account })
    }
  }, [query.error, queryClient])
  return query
}

export function useSelectPanDirectory(accountID: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiPut<PanDirectory>('/api/pan/directory', { id }),
    onSuccess: directory => {
      invalidatePanSource(queryClient)
      queryClient.setQueryData<PanAccountStatus>(panKeys.account, status =>
        status?.account?.id === accountID ? { ...status, directory } : status
      )
    },
    onError: error => {
      if (error instanceof ApiError && error.status === 401) {
        void queryClient.invalidateQueries({ queryKey: panKeys.account })
      }
    }
  })
}

export function useClearPanDirectory(accountID: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiDelete<null>('/api/pan/directory'),
    onSuccess: () => {
      invalidatePanSource(queryClient)
      queryClient.setQueryData<PanAccountStatus>(panKeys.account, status =>
        status?.account?.id === accountID ? { ...status, directory: undefined } : status
      )
    }
  })
}
