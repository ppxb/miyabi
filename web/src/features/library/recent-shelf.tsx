import { useRecentMovies } from '@/api/library'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { MovieShelf } from '@/features/library/movie-shelf'

export function RecentlyAddedShelf({ onPlay }: { onPlay: (movieID: number) => void }) {
  const recent = useRecentMovies(12)
  const items = recent.data?.movies ?? []

  return (
    <MovieShelf title="最近加入" itemCount={items.length}>
      {items.map(movie => (
        <div
          key={movie.id}
          className="w-[72vw] shrink-0 snap-start sm:w-[calc((100%-1rem)/2)] lg:w-[calc((100%-3rem)/3)] xl:w-[calc((100%-4.5rem)/4)]"
        >
          <LibraryMovieCard movie={movie} onPlay={() => onPlay(movie.id)} />
        </div>
      ))}
    </MovieShelf>
  )
}
