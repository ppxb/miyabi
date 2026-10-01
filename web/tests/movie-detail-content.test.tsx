import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, expect, test, vi } from 'vitest'

import { useRecordMovieView } from '@/api/browse-history'
import { imageURL } from '@/api/client'
import { useDiscoverMagnets, useDiscoverMovie, useResolveDiscoverMovie } from '@/api/discover'
import { useLibraryMovie, type LibraryMovieDetail } from '@/api/library'
import { MovieDetailContent } from '@/features/movie-detail/content'

vi.mock('@/api/discover', () => ({
  useDiscoverMovie: vi.fn(),
  useDiscoverMagnets: vi.fn(),
  useResolveDiscoverMovie: vi.fn()
}))
vi.mock('@/api/browse-history', () => ({ useRecordMovieView: vi.fn() }))
vi.mock('@/api/library', () => ({ useLibraryMovie: vi.fn() }))

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(useDiscoverMovie).mockReturnValue({ isPending: true } as ReturnType<
    typeof useDiscoverMovie
  >)
})

test('saved library metadata renders without a JavDB identity or lookup', () => {
  vi.mocked(useLibraryMovie, { partial: true }).mockReturnValue({
    data: {
      library_id: 42,
      id: '',
      code: 'ABP-123',
      title: 'Saved title',
      origin_title: '',
      cover: '',
      thumbnail: '',
      release_date: '2026-01-01',
      duration: 120,
      rating: 4.2,
      rating_source: 'fanza',
      rating_max: 5,
      actors: [
        {
          provider: 'fanza',
          id: 'actor',
          name: 'Saved actor',
          name_zht: '',
          gender: 'female',
          avatar: ''
        }
      ],
      tags: [{ provider: 'fanza', id: 'genre', name: 'Saved tag', name_zht: '', category_id: '' }],
      preview_images: [],
      preview_video: '',
      magnets_count: 0,
      has_subtitle: false,
      has_preview: false,
      actor_movies: [],
      related_movies: [],
      scrape_status: 'done',
      zone: 'censored'
    } satisfies LibraryMovieDetail,
    isPending: false
  })
  const html = renderToStaticMarkup(<MovieDetailContent movie={{ libraryId: 42 }} />)
  expect(useLibraryMovie).toHaveBeenCalledWith(42)
  expect(useDiscoverMovie).not.toHaveBeenCalled()
  expect(useResolveDiscoverMovie).not.toHaveBeenCalled()
  expect(useDiscoverMagnets).not.toHaveBeenCalled()
  expect(useRecordMovieView).not.toHaveBeenCalled()
  expect(html).toContain('Saved title')
  expect(html).toContain('Saved actor')
  expect(html).toContain('FANZA 评分')
  expect(html).not.toContain('JavDB 评分')
  expect(html).not.toContain('href=')
})

test('library preview URLs keep their own provider route and revision', () => {
  const url = '/api/library/movies/42/previews/0?v=123456'
  expect(imageURL(url)).toBe(url)
  expect(imageURL('https://image.example/preview.jpg')).toContain('/api/image?')
  expect(imageURL('/api/library/movies/42/previews/0/../../other')).toContain('/api/image?')
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
