import type { WatchHistoryItem } from '@/api/history'
import type { LibraryMovieCardData } from './movie-card'

export type HistoryCard = {
  movie: LibraryMovieCardData
  progress: number
}

/**
 * Maps a watch-history item to the library card's props. Progress is rounded to
 * a whole percent; the card clamps out-of-range values for display.
 */
export function historyToCard(item: WatchHistoryItem): HistoryCard {
  const progress = item.duration > 0 ? Math.round((item.position / item.duration) * 100) : 0
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
