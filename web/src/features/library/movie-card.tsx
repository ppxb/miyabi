import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { MovieDetailTrigger } from '@/features/movie-detail/detail-trigger'
import { LibraryMovieStatus } from './movie-status'

export function LibraryMovieCard({ movie }: { movie: LibraryMovie }) {
  const card = (
    <MovieCard movie={movie} titleTooltip={false}>
      <LibraryMovieStatus movie={movie} />
    </MovieCard>
  )

  return movie.javdb_id ? (
    <MovieDetailTrigger movieId={movie.javdb_id} className="block min-w-0 rounded-2xl outline-ring">
      {card}
    </MovieDetailTrigger>
  ) : (
    <div className="min-w-0">{card}</div>
  )
}
