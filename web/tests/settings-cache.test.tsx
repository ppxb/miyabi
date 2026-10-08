import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryObserver, useQuery } from '@tanstack/react-query'
import { beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { useDataInfo } from '@/api/data'
import { useEmbyConfig } from '@/api/emby'
import { useNetworkConfig } from '@/api/network'
import { useSubscriptionSettings } from '@/api/subscription-settings'

// Feed each hook's actual options into QueryObserver to test remounts without a browser.
vi.mock('@tanstack/react-query', async importOriginal => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useQuery: vi.fn()
}))

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  vi.stubGlobal('window', { location: { origin: 'http://localhost' } })
})

test.each([
  { name: 'network', useConfig: useNetworkConfig, path: '/api/settings/network' },
  { name: 'Emby', useConfig: useEmbyConfig, path: '/api/settings/emby' },
  { name: 'subscriptions', useConfig: useSubscriptionSettings, path: '/api/settings/subscription' },
  { name: 'cache statistics', useConfig: useDataInfo, path: '/api/settings/system' }
])(
  '$name reuses fresh data but refreshes expired or invalidated data',
  async ({ useConfig, path }) => {
    function Config() {
      useConfig()
      return null
    }
    renderToStaticMarkup(<Config />)
    const options = vi.mocked(useQuery).mock.calls.at(-1)![0]
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(async url => {
      expect(url).toBe(path)
      return Response.json({})
    })
    vi.stubGlobal('fetch', fetch)
    let unsubscribe = () => {}
    function mount() {
      const observer = new QueryObserver(client, options)
      unsubscribe = observer.subscribe(() => {})
    }
    onTestFinished(() => {
      unsubscribe()
      client.clear()
    })

    mount()
    await vi.advanceTimersByTimeAsync(0)
    expect(fetch).toHaveBeenCalledTimes(1)
    unsubscribe()

    await vi.advanceTimersByTimeAsync(14_999)
    mount()
    await vi.advanceTimersByTimeAsync(0)
    expect(fetch).toHaveBeenCalledTimes(1)
    unsubscribe()

    await vi.advanceTimersByTimeAsync(1)
    mount()
    await vi.advanceTimersByTimeAsync(0)
    expect(fetch).toHaveBeenCalledTimes(2)

    // Mutation failures must still reconcile immediately, even while the cache is fresh.
    await client.invalidateQueries({ queryKey: options.queryKey })
    expect(fetch).toHaveBeenCalledTimes(3)
  }
)
