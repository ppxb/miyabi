import {
  Children,
  isValidElement,
  type ComponentProps,
  type KeyboardEvent,
  type MouseEvent,
  type ReactElement,
  type ReactNode
} from 'react'
import { expect, test, vi } from 'vitest'

import { type SubscriptionItem, useRemoveSubscription } from '@/api/subscriptions'
import { MovieCard } from '@/components/movie'
import { Button } from '@/components/ui/button'
import { MovieDetailLink } from '@/features/movie-detail/detail-link'
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

function findAction(node: ReactNode): ReactElement<ComponentProps<typeof Button>> | undefined {
  if (!isValidElement<{ children?: ReactNode }>(node)) return undefined
  if (node.type === Button) return node as ReactElement<ComponentProps<typeof Button>>
  return Children.toArray(node.props.children).map(findAction).find(Boolean)
}

function cardState(selecting: boolean, disabled = false) {
  const remove = vi.fn()
  vi.mocked(useRemoveSubscription).mockReturnValue({
    isPending: false,
    mutate: remove
  } as unknown as ReturnType<typeof useRemoveSubscription>)
  const onSelect = vi.fn()
  const tree = SubscriptionCard({ item, selecting, disabled, selected: false, onSelect })
  const link = Children.toArray(tree.props.children)[0] as ReactElement<
    ComponentProps<typeof MovieDetailLink>
  >
  const card = link.props.children as ReactElement<ComponentProps<typeof MovieCard>>
  const action = findAction(card.props.coverOverlay)
  return { tree, link, card, action, onSelect, remove }
}

test('entering and leaving selection preserves card ancestors without a changing footer', () => {
  const browsing = cardState(false)
  for (const selecting of [true, false]) {
    const current = cardState(selecting)
    // Changing an ancestor's type or key remounts the cover and resets its loaded state.
    for (const node of ['tree', 'link', 'card'] as const) {
      expect(current[node].type).toBe(browsing[node].type)
      expect(current[node].key).toBe(browsing[node].key)
    }
    expect(current.card.props.movie.cover).toBe(browsing.card.props.movie.cover)
    expect(current.card.props.children).toBeUndefined()
    expect(current.action === undefined).toBe(selecting)
  }
})

test('selection intercepts card clicks and Space, while browsing leaves detail navigation available', () => {
  const selecting = cardState(true)
  const preventDefault = vi.fn()
  selecting.link.props.onClick?.({ preventDefault } as unknown as MouseEvent<HTMLAnchorElement>)
  expect(preventDefault).toHaveBeenCalledOnce()
  expect(selecting.onSelect).toHaveBeenCalledOnce()

  selecting.link.props.onKeyDown?.({
    key: ' ',
    repeat: false,
    preventDefault
  } as unknown as KeyboardEvent<HTMLAnchorElement>)
  expect(selecting.onSelect).toHaveBeenCalledTimes(2)
  selecting.link.props.onKeyDown?.({
    key: ' ',
    repeat: true,
    preventDefault
  } as unknown as KeyboardEvent<HTMLAnchorElement>)
  expect(selecting.onSelect).toHaveBeenCalledTimes(2)
  expect(selecting.link.props['aria-haspopup']).toBe(false)

  const browsing = cardState(false)
  preventDefault.mockClear()
  browsing.link.props.onClick?.({ preventDefault } as unknown as MouseEvent<HTMLAnchorElement>)
  expect(preventDefault).not.toHaveBeenCalled()
  expect(browsing.onSelect).not.toHaveBeenCalled()
})

test('disabled selection cannot toggle or navigate, including middle clicks', () => {
  const { link, onSelect } = cardState(true, true)
  const preventDefault = vi.fn()
  link.props.onClick?.({ preventDefault } as unknown as MouseEvent<HTMLAnchorElement>)
  link.props.onKeyDown?.({
    key: ' ',
    preventDefault
  } as unknown as KeyboardEvent<HTMLAnchorElement>)
  link.props.onAuxClick?.({ button: 1, preventDefault } as unknown as MouseEvent<HTMLAnchorElement>)
  expect(onSelect).not.toHaveBeenCalled()
  expect(preventDefault).toHaveBeenCalledTimes(3)
  expect(link.props['aria-disabled']).toBe(true)
  expect(link.props.tabIndex).toBe(-1)
})

test('canceling a subscription stays independent of the card click', () => {
  const { action, remove, onSelect } = cardState(false)
  expect(action).toBeDefined()
  expect(action!.props.size).toBe('icon-sm')
  expect(action!.props['aria-label']).toBe(`取消订阅 ${item.code}`)
  const preventDefault = vi.fn()
  const stopPropagation = vi.fn()
  action!.props.onClick?.({
    preventDefault,
    stopPropagation
  } as unknown as MouseEvent<HTMLButtonElement>)
  expect(remove).toHaveBeenCalledWith(item)
  expect(preventDefault).toHaveBeenCalledOnce()
  expect(stopPropagation).toHaveBeenCalledOnce()
  expect(onSelect).not.toHaveBeenCalled()
})
