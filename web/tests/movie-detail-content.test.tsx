import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, expect, test, vi } from 'vitest'

import { useRecordMovieView } from '@/api/browse-history'
import { imageURL } from '@/api/client'
import { useDiscoverMagnets, useDiscoverMovie } from '@/api/discover'
import { useLibraryMovie, type LibraryMovieDetail } from '@/api/library'
import { MovieDetailContent } from '@/features/movie-detail/content'

vi.mock('@/api/discover', () => ({
  useDiscoverMovie: vi.fn(),
  useDiscoverMagnets: vi.fn()
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
  expect(useDiscoverMagnets).not.toHaveBeenCalled()
  expect(useRecordMovieView).not.toHaveBeenCalled()
  expect(html).toContain('Saved title')
  expect(html).toContain('Saved actor')
  expect(html).toContain('评分')
  expect(html).not.toContain('FANZA 评分')
  expect(html).not.toContain('JavDB 评分')
  expect(html).not.toContain('href=')
})

test('library preview URLs keep their own provider route and revision', () => {
  const url = '/api/library/movies/42/previews/0?v=123456'
  expect(imageURL(url)).toBe(url)
  expect(imageURL('https://image.example/preview.jpg')).toContain('/api/image?')
  expect(imageURL('/api/library/movies/42/previews/0/../../other')).toContain('/api/image?')
})

test('known identities open details by id', () => {
  renderToStaticMarkup(<MovieDetailContent movie={{ id: 'catalogue-movie' }} />)
  expect(useDiscoverMovie).toHaveBeenCalledWith('catalogue-movie', undefined)
})

test('code targets open detail directly by code without a lookup step', () => {
  renderToStaticMarkup(<MovieDetailContent movie={{ code: 'ABP-123' }} />)
  expect(useDiscoverMovie).toHaveBeenCalledWith('ABP-123', 'ABP-123')
  expect(useDiscoverMagnets).toHaveBeenCalledWith('ABP-123', 'ABP-123')
  expect(useRecordMovieView).toHaveBeenCalledWith('ABP-123')
})
