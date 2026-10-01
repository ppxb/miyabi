import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { MovieDetailTrigger } from '@/features/movie-detail/detail-trigger'
import { LibraryMovieStatus } from './movie-status'

export function LibraryMovieCard({ movie }: { movie: LibraryMovie }) {
  return (
    <MovieDetailTrigger
      movie={{ libraryId: movie.id }}
      className="block min-w-0 rounded-2xl outline-ring"
    >
      <MovieCard movie={movie} titleTooltip={false}>
        <LibraryMovieStatus movie={movie} />
      </MovieCard>
    </MovieDetailTrigger>
  )
}
