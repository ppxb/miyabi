import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, expect, test, vi } from 'vitest'

import { useRecordMovieView } from '@/api/browse-history'
import { useDiscoverMagnets, useDiscoverMovie, useResolveDiscoverMovie } from '@/api/discover'
import { MovieDetailContent } from '@/features/movie-detail/content'

vi.mock('@/api/discover', () => ({
  useDiscoverMovie: vi.fn(),
  useDiscoverMagnets: vi.fn(),
  useResolveDiscoverMovie: vi.fn()
}))
vi.mock('@/api/browse-history', () => ({ useRecordMovieView: vi.fn() }))

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(useDiscoverMovie).mockReturnValue({ isPending: true } as ReturnType<
    typeof useDiscoverMovie
  >)
})

test('known JavDB identities open details without a code lookup', () => {
  renderToStaticMarkup(<MovieDetailContent movie={{ id: 'javdb-movie' }} />)
  expect(useResolveDiscoverMovie).not.toHaveBeenCalled()
  expect(useDiscoverMovie).toHaveBeenCalledWith('javdb-movie')
})

test('unresolved and failed code lookups never fetch magnets or record an invalid movie view', () => {
  for (const isPending of [true, false]) {
    vi.mocked(useResolveDiscoverMovie).mockReturnValue({
      isPending,
      isFetching: false,
      data: undefined,
      refetch: vi.fn()
    } as unknown as ReturnType<typeof useResolveDiscoverMovie>)
    const html = renderToStaticMarkup(<MovieDetailContent movie={{ code: 'ABP-123' }} />)
    expect(useResolveDiscoverMovie).toHaveBeenCalledWith('ABP-123')
    expect(useDiscoverMovie).not.toHaveBeenCalled()
    expect(useDiscoverMagnets).not.toHaveBeenCalled()
    expect(useRecordMovieView).not.toHaveBeenCalled()
    if (!isPending) {
      expect(html).toContain('未能从 JavDB 确认 ABP-123 的影片详情')
      expect(html).toContain('重试')
    }
  }
})

test('resolved codes load existing JavDB detail, magnets and history with the confirmed ID', () => {
  vi.mocked(useResolveDiscoverMovie).mockReturnValue({
    isPending: false,
    data: { id: 'confirmed-id' }
  } as ReturnType<typeof useResolveDiscoverMovie>)
  renderToStaticMarkup(<MovieDetailContent movie={{ code: 'ABP-123' }} />)
  expect(useDiscoverMovie).toHaveBeenCalledWith('confirmed-id')
  expect(useDiscoverMagnets).toHaveBeenCalledWith('confirmed-id')
  expect(useRecordMovieView).toHaveBeenCalledWith('confirmed-id')
})
