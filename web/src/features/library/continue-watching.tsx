import { memo } from 'react'

import { useHistory } from '@/api/history'
import { historyToCard } from '@/features/library/history-card'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { MovieShelf } from '@/features/library/movie-shelf'
import { MovieShelfItem } from '@/features/library/movie-shelf-section'

export const ContinueWatching = memo(function ContinueWatching({
  onPlay
}: {
  onPlay: (movieID: number) => void
}) {
  const history = useHistory(1)
  const items = (history.data?.items ?? [])
    .filter(item => item.duration > 0 && item.position > 0 && item.position / item.duration < 0.95)
    .slice(0, 12)

  return (
    <MovieShelf title="正在观看" itemCount={items.length} href="/history">
      {items.map(item => {
        const { movie, progress } = historyToCard(item)
        return (
          <MovieShelfItem key={item.movie_id}>
            <LibraryMovieCard
              movie={movie}
              progress={progress}
              onPlay={() => onPlay(item.movie_id)}
            />
          </MovieShelfItem>
        )
      })}
    </MovieShelf>
  )
})
