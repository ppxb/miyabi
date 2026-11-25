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
  status: SubscriptionStatus
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

// Stable module-level select so the index map is only rebuilt when the data
// changes, not on every render.
function indexTargets(targets: SubscriptionTarget[]) {
  const map = new Map<string, SubscriptionTarget>()
  for (const item of targets) {
    map.set(item.target_id, item)
  }
  return { list: targets, map }
}

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
    select: indexTargets
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

// Following a library actor, by actor key, records their new works for the
// follow page.
export function useAddSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (actorKey: string) =>
      apiPost<SubscriptionItem>('/api/subscriptions', { kind: 'actor', target_id: actorKey }),
    // Awaiting the refetch keeps the follow button busy until it shows the new state.
    onSuccess: async item => {
      await queryClient.invalidateQueries({ queryKey: subscriptionKeys.all })
      toast.success(`已关注 ${item.title}`, {
        description: '每天检查一次新作，显示在「关注」页。'
      })
    },
    meta: { errorTitle: '关注失败' }
  })
}

export function useUpdateSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...patch }: { id: number; status?: 'active' | 'paused' }) =>
      apiPatch<SubscriptionItem>(`/api/subscriptions/${id}`, patch),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: subscriptionKeys.all })
    },
    meta: { errorTitle: '更新关注失败' }
  })
}

export function useRemoveSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (item: { id: number }) => apiDelete<null>(`/api/subscriptions/${item.id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: subscriptionKeys.all }),
    meta: { errorTitle: '取消关注失败' }
  })
}
