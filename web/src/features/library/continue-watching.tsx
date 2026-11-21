import { useHistory } from '@/api/history'
import type { LibraryMovie } from '@/api/library'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { MovieShelf } from '@/features/library/movie-shelf'

export function ContinueWatching({ onPlay }: { onPlay: (movieID: number) => void }) {
  const history = useHistory(1)
  const items = (history.data?.items ?? [])
    .filter(item => item.duration > 0 && item.position > 0 && item.position / item.duration < 0.95)
    .slice(0, 12)

  return (
    <MovieShelf title="正在观看" itemCount={items.length} href="/history">
      {items.map(item => {
        const percent = (item.position / item.duration) * 100
        const movie: LibraryMovie = {
          id: item.movie_id,
          code: item.code,
          title: item.title,
          cover: item.cover,
          poster: item.poster,
          duration: item.duration,
          rating: item.rating ?? 0,
          actors: item.actors ?? [],
          source: item.source,
          has_subtitle: item.has_subtitle,
          tags: [],
          scrape_status: 'done'
        }
        return (
          <div
            key={item.movie_id}
            className="w-[72vw] shrink-0 snap-start sm:w-[calc((100%-1rem)/2)] lg:w-[calc((100%-3rem)/3)] xl:w-[calc((100%-4.5rem)/4)]"
          >
            <LibraryMovieCard
              movie={movie}
              progress={percent}
              onPlay={() => onPlay(item.movie_id)}
            />
          </div>
        )
      })}
    </MovieShelf>
  )
}
