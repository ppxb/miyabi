import {
  Children,
  isValidElement,
  type ComponentProps,
  type MouseEvent,
  type ReactElement,
  type ReactNode
} from 'react'
import { expect, test, vi } from 'vitest'

import { type SubscriptionItem, useRemoveSubscription } from '@/api/subscriptions'
import { MovieCard } from '@/components/movie'
import { Button } from '@/components/ui/button'
import { MovieDetailTrigger } from '@/features/movie-detail/detail-trigger'
import { SubscriptionCard } from '@/features/subscriptions/subscription-card'

vi.mock('@/api/subscriptions', async importOriginal => ({
  ...(await importOriginal<typeof import('@/api/subscriptions')>()),
  useRemoveSubscription: vi.fn()
}))

const item: SubscriptionItem = {
  id: 1,
  kind: 'movie',
  target_id: 'movie-1',
  code: 'TEST-001',
  title: 'Test movie',
  cover: 'https://example.com/cover.jpg',
  auto_download: false,
  status: 'waiting',
  checks: 0,
  created_at: '',
  updated_at: ''
}

function findPath(
  node: ReactNode,
  type: ReactElement['type']
): ReactElement<{ children?: ReactNode }>[] | undefined {
  for (const child of Children.toArray(node)) {
    if (!isValidElement<{ children?: ReactNode }>(child)) continue
    if (child.type === type) return [child]
    const path = findPath(child.props.children, type)
    if (path) return [child, ...path]
  }
}

function cardState(selecting: boolean, disabled = false) {
  const remove = vi.fn()
  vi.mocked(useRemoveSubscription).mockReturnValue({
    isPending: false,
    mutate: remove
  } as unknown as ReturnType<typeof useRemoveSubscription>)
  const onSelect = vi.fn()
  const tree = SubscriptionCard({ item, selecting, disabled, selected: false, onSelect })
  const path = findPath(tree, MovieCard)
  expect(path).toBeDefined()
  const trigger = path!.find(node => node.type === MovieDetailTrigger) as ReactElement<
    ComponentProps<typeof MovieDetailTrigger>
  >
  expect(trigger).toBeDefined()
  const card = path!.at(-1) as ReactElement<ComponentProps<typeof MovieCard>>
  const action = findPath(card.props.coverOverlay, Button)?.at(-1) as
    | ReactElement<ComponentProps<typeof Button>>
    | undefined
  const ancestry = path!.map(({ type, key }) => ({ type, key }))
  return { ancestry, trigger, card, action, onSelect, remove }
}

test('selection preserves the cover ancestry and hides the cancel action', () => {
  const browsing = cardState(false)
  for (const selecting of [true, false]) {
    const current = cardState(selecting)
    // Changing an ancestor's type or key remounts the cover and resets its loaded state.
    expect(current.ancestry).toEqual(browsing.ancestry)
    expect(current.card.props.movie.cover).toBe(browsing.card.props.movie.cover)
    expect(current.action === undefined).toBe(selecting)
  }
})

test('selection consumes card clicks while browsing allows the detail dialog', () => {
  const selecting = cardState(true)
  const preventDefault = vi.fn()
  selecting.trigger.props.onClick?.({ preventDefault } as unknown as MouseEvent<HTMLDivElement>)
  expect(preventDefault).toHaveBeenCalledOnce()
  expect(selecting.onSelect).toHaveBeenCalledOnce()

  const browsing = cardState(false)
  preventDefault.mockClear()
  browsing.trigger.props.onClick?.({ preventDefault } as unknown as MouseEvent<HTMLDivElement>)
  expect(preventDefault).not.toHaveBeenCalled()
  expect(browsing.onSelect).not.toHaveBeenCalled()
})

test('selection disables unavailable cards without disabling normal detail browsing', () => {
  expect(cardState(true, true).trigger.props.disabled).toBe(true)
  expect(cardState(false, true).trigger.props.disabled).toBe(false)
})

test('canceling a subscription stays independent of the card click', () => {
  const { action, remove, onSelect } = cardState(false)
  expect(action).toBeDefined()
  const preventDefault = vi.fn()
  const stopPropagation = vi.fn()
  action!.props.onClick?.({
    preventDefault,
    stopPropagation
  } as never)
  expect(remove).toHaveBeenCalledWith(item)
  expect(preventDefault).toHaveBeenCalledOnce()
  expect(stopPropagation).toHaveBeenCalledOnce()
  expect(onSelect).not.toHaveBeenCalled()
})
