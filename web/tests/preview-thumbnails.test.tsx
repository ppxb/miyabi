import { renderToStaticMarkup } from 'react-dom/server'
import { expect, test, vi } from 'vitest'

import { imageURL } from '@/api/client'
import { MoviePreviews } from '@/features/movie-detail/previews'

vi.mock('@/stores/settings', () => ({
  useSettingsStore: (select: (state: { nsfwMode: boolean }) => unknown) =>
    select({ nsfwMode: false })
}))

test('preview grids use thumbnails without advertising the full image as a retina source', () => {
  const original = '/api/library/movies/42/previews/0?v=123'
  const thumbnail = '/api/library/movies/42/previews/0/thumbnail?v=123'
  expect(imageURL(original)).toBe(original)
  expect(imageURL(thumbnail)).toBe(thumbnail)
  const html = renderToStaticMarkup(<MoviePreviews images={[{ original, thumbnail }]} />)
  expect(html).toContain(`src="${thumbnail}"`)
  expect(html).not.toContain('srcSet=')
  expect(html).not.toContain(`src="${original}"`)
})
