import type { WatchHistoryItem } from '@/api/history'
import type { LibraryMovieCardData } from './movie-card'

export type HistoryCard = {
  movie: LibraryMovieCardData
  progress: number
}

/**
 * Maps a watch-history item to the library card's props. Progress is clamped to
 * 0-100 and rounded, the single shared convention for history and shelves (the
 * card also clamps defensively).
 */
export function historyToCard(item: WatchHistoryItem): HistoryCard {
  const progress =
    item.duration > 0
      ? Math.max(0, Math.min(100, Math.round((item.position / item.duration) * 100)))
      : 0
  return {
    movie: {
      id: item.movie_id,
      code: item.code,
      title: item.title,
      cover: item.cover,
      rating: item.rating ?? 0,
      actors: item.actors ?? [],
      source: item.source,
      has_subtitle: item.has_subtitle
    },
    progress
  }
}
