import { useTopRatedMovies } from '@/api/library'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { MovieShelf } from '@/features/library/movie-shelf'

export function TopRatedShelf({ onPlay }: { onPlay: (movieID: number) => void }) {
  const movies = useTopRatedMovies(12)
  const items = movies.data ?? []

  return (
    <MovieShelf title="高分影片" itemCount={items.length}>
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
