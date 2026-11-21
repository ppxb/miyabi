import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient
} from '@tanstack/react-query'

import { apiDelete, apiGet, apiPost, apiPut } from '@/api/client'
import { resetMovieStates } from '@/api/movie-states'

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

export type PanLoginState = 'waiting' | 'scanned' | 'authorized' | 'expired' | 'canceled'

type PanLoginStatus = {
  state: PanLoginState
  account_id?: string
}

export const panKeys = {
  account: ['pan', 'account'] as const,
  accounts: ['pan', 'accounts'] as const,
  accountStatus: (accountID: string) => ['pan', 'accounts', accountID] as const,
  accountFiles: (accountID: string, directoryID: string, page: number) =>
    ['pan', 'accounts', accountID, 'files', directoryID, page] as const,
  login: (id: string) => ['pan', 'login', id] as const,
  files: (accountID: string, directoryID: string, page: number) =>
    ['pan', 'files', accountID, directoryID, page] as const
}

export function invalidatePanSource(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['library'] })
  void queryClient.invalidateQueries({ queryKey: ['offline'] })
  void resetMovieStates(queryClient)
}

export function useBeginPanLogin() {
  return useMutation({
    mutationFn: () => apiPost<PanLoginSession>('/api/pan/login'),
    gcTime: 0
  })
}

const loginPollInterval = 1500

export function panLoginOptions(id: string) {
  return queryOptions({
    queryKey: panKeys.login(id),
    queryFn: ({ signal }) =>
      apiGet<PanLoginStatus>(`/api/pan/login/${encodeURIComponent(id)}`, undefined, signal),
    enabled: id !== '',
    // Four retries plus the first attempt preserve the five-failure budget.
    retry: 4,
    retryDelay: loginPollInterval,
    staleTime: 0,
    gcTime: 0,
    refetchOnReconnect: false,
    refetchInterval: query => {
      if (query.state.status === 'error') return false
      const state = query.state.data?.state
      return !state || state === 'waiting' || state === 'scanned' ? loginPollInterval : false
    }
  })
}

export function usePanLoginStatus(id: string) {
  return useQuery(panLoginOptions(id))
}

export function usePanAccounts() {
  return useQuery({
    queryKey: panKeys.accounts,
    queryFn: ({ signal }) => apiGet<PanAccountStatus[]>('/api/pan/accounts', undefined, signal)
  })
}

export function usePanAccountStatus(accountID: string) {
  return useQuery({
    queryKey: panKeys.accountStatus(accountID),
    queryFn: ({ signal }) =>
      apiGet<PanAccountStatus>(
        `/api/pan/accounts/${encodeURIComponent(accountID)}`,
        undefined,
        signal
      ),
    enabled: accountID !== ''
  })
}

export function usePanAccountFiles(accountID: string, directoryID: string, page: number) {
  return useQuery({
    queryKey: panKeys.accountFiles(accountID, directoryID, page),
    queryFn: ({ signal }) =>
      apiGet<PanFilePage>(
        `/api/pan/accounts/${encodeURIComponent(accountID)}/files`,
        {
          directory_id: directoryID,
          page
        },
        signal
      )
  })
}

export function useSelectPanAccountDirectory(accountID: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiPut<PanDirectory>(`/api/pan/accounts/${encodeURIComponent(accountID)}/directory`, { id }),
    onSuccess: () => {
      invalidatePanSource(queryClient)
      void queryClient.invalidateQueries({ queryKey: panKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: panKeys.accountStatus(accountID) })
    }
  })
}

export function useClearPanAccountDirectory(accountID: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      apiDelete<null>(`/api/pan/accounts/${encodeURIComponent(accountID)}/directory`),
    onSuccess: () => {
      invalidatePanSource(queryClient)
      void queryClient.invalidateQueries({ queryKey: panKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: panKeys.accountStatus(accountID) })
    }
  })
}

export function useDisconnectPanAccount(accountID: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiDelete<null>(`/api/pan/accounts/${encodeURIComponent(accountID)}`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: panKeys.accounts })
      void queryClient.removeQueries({ queryKey: panKeys.accountStatus(accountID) })
      invalidatePanSource(queryClient)
    }
  })
}
