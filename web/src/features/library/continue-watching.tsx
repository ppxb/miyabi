import { useHistory } from '@/api/history'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { MovieShelf } from '@/features/library/movie-shelf'
import { MovieShelfItem } from '@/features/library/movie-shelf-section'

export function ContinueWatching({ onPlay }: { onPlay: (movieID: number) => void }) {
  const history = useHistory(1)
  const items = (history.data?.items ?? [])
    .filter(item => item.duration > 0 && item.position > 0 && item.position / item.duration < 0.95)
    .slice(0, 12)

  return (
    <MovieShelf title="正在观看" itemCount={items.length} href="/history">
      {items.map(item => {
        const percent = (item.position / item.duration) * 100
        return (
          <MovieShelfItem key={item.movie_id}>
            <LibraryMovieCard
              movie={{
                id: item.movie_id,
                code: item.code,
                title: item.title,
                cover: item.cover,
                rating: item.rating ?? 0,
                actors: item.actors ?? [],
                source: item.source,
                has_subtitle: item.has_subtitle
              }}
              progress={percent}
              onPlay={() => onPlay(item.movie_id)}
            />
          </MovieShelfItem>
        )
      })}
    </MovieShelf>
  )
}
