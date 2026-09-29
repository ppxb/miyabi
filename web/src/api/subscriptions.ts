import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { apiDelete, apiGet, apiPatch, apiPost } from './client'

export type SubscriptionKind = 'movie' | 'actor'

export type SubscriptionStatus = 'waiting' | 'added' | 'stale' | 'active' | 'paused'

export type SubscriptionItem = {
  id: number
  kind: SubscriptionKind
  target_id: string
  code: string
  title: string
  cover: string
  release_date?: string
  origin_id?: number
  auto_download: boolean
  status: SubscriptionStatus
  hash?: string
  task_id?: number
  next_check_at?: string
  last_checked_at?: string
  checks: number
  error?: string
  created_at: string
  updated_at: string
}

export const SUBSCRIPTION_PAGE_SIZE = 20

export type SubscriptionTarget = {
  id: number
  kind: SubscriptionKind
  target_id: string
  origin_id?: number
  status: SubscriptionStatus
}

export const subscriptionKeys = {
  all: ['subscriptions'] as const,
  targets: (kind?: SubscriptionKind) => ['subscriptions', 'targets', kind ?? 'all'] as const,
  list: (kind: SubscriptionKind, page = 1, limit = SUBSCRIPTION_PAGE_SIZE) =>
    ['subscriptions', 'list', kind, { page, limit }] as const,
  feed: (actorID: number | null, page = 1, limit = SUBSCRIPTION_PAGE_SIZE) =>
    ['subscriptions', 'feed', actorID ?? 'all', { page, limit }] as const
}

const listOptions = {
  staleTime: Infinity,
  refetchOnMount: 'always'
} as const

export function useSubscriptionTargets(kind?: SubscriptionKind) {
  return useQuery({
    queryKey: subscriptionKeys.targets(kind),
    queryFn: ({ signal }) =>
      apiGet<SubscriptionTarget[]>(
        '/api/subscriptions/targets',
        kind ? { kind } : undefined,
        signal
      ),
    staleTime: Infinity,
    select: targets => {
      const map = new Map<string, SubscriptionTarget>()
      for (const item of targets) {
        map.set(item.target_id, item)
      }
      return { list: targets, map }
    }
  })
}

export function useSubscriptions(
  kind: SubscriptionKind,
  page = 1,
  limit = SUBSCRIPTION_PAGE_SIZE,
  enabled = true
) {
  return useQuery({
    ...listOptions,
    queryKey: subscriptionKeys.list(kind, page, limit),
    queryFn: ({ signal }) =>
      apiGet<SubscriptionItem[]>('/api/subscriptions', { kind, page, limit }, signal),
    enabled
  })
}

export function useSubscription(kind: SubscriptionKind, targetID: string) {
  const targets = useSubscriptionTargets(kind)
  return {
    subscription: targets.data?.map.get(targetID),
    isPending: targets.isPending
  }
}

export function useActorFeed(
  actorID: number | null,
  page = 1,
  limit = SUBSCRIPTION_PAGE_SIZE,
  enabled = true
) {
  const url = actorID
    ? `/api/subscriptions/actors/${actorID}/feed`
    : '/api/subscriptions/actors/feed'
  return useQuery({
    ...listOptions,
    queryKey: subscriptionKeys.feed(actorID, page, limit),
    queryFn: ({ signal }) => apiGet<SubscriptionItem[]>(url, { page, limit }, signal),
    enabled
  })
}

export function useAddSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (payload: { kind: SubscriptionKind; target_id: string; title?: string }) =>
      apiPost<SubscriptionItem>('/api/subscriptions', payload),
    onSuccess: item => {
      void queryClient.invalidateQueries({ queryKey: subscriptionKeys.all })
      toast.success(item.kind === 'actor' ? `已订阅演员 ${item.title}` : item.code, {
        description:
          item.kind === 'actor'
            ? '每天检查一次新作，未发售作品会自动加入影片订阅。'
            : item.auto_download
              ? '出现符合偏好的磁力后会自动加入 115。'
              : '出现磁力后可在订阅页手动入库。'
      })
    },
    meta: { errorTitle: '订阅失败' }
  })
}

export function useUpdateSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      id,
      ...patch
    }: {
      id: number
      auto_download?: boolean
      status?: 'active' | 'paused'
    }) => apiPatch<SubscriptionItem>(`/api/subscriptions/${id}`, patch),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: subscriptionKeys.all })
    },
    meta: { errorTitle: '更新订阅失败' }
  })
}

export function useRemoveSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (item: { id: number; kind?: SubscriptionKind }) =>
      apiDelete<null>(`/api/subscriptions/${item.id}`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: subscriptionKeys.all })
    },
    meta: { errorTitle: '取消订阅失败' }
  })
}

export function useEnqueueSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => apiPost<SubscriptionItem>(`/api/subscriptions/${id}/enqueue`),
    onSuccess: item => {
      void queryClient.invalidateQueries({ queryKey: subscriptionKeys.all })
      if (item.status === 'added') {
        toast.success(item.code, { description: '已加入 115 离线下载。' })
      } else {
        toast.info(item.code, { description: '暂无符合偏好的磁力，出现后会自动加入 115。' })
      }
    },
    meta: { errorTitle: '入库失败' }
  })
}

export function useBatchEnqueueSubscriptions() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (payload: { ids?: number[]; all?: boolean }) =>
      apiPost<{ task_id: number }>('/api/subscriptions/enqueue', payload),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: subscriptionKeys.all })
  })
}
