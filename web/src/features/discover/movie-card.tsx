import type { DiscoverMovie } from '@/api/discover'
import { MovieCard, MovieCoverCaption, MovieRatingPill } from '@/components/movie'
import { MovieStateBadge } from '@/components/movie/movie-badges'
import { MovieDetailTrigger } from '@/features/movie-detail/detail-trigger'

/** The fields the explore card actually renders (no subscription affordance). */
export type DiscoverMovieCardData = Pick<
  DiscoverMovie,
  'id' | 'code' | 'title' | 'cover' | 'rating' | 'release_date'
>

export function DiscoverMovieCard({ movie }: { movie: DiscoverMovieCardData }) {
  const title = movie.title || movie.code
  return (
    <MovieDetailTrigger
      movie={{ id: movie.id, code: movie.code }}
      className="block h-full min-w-0 rounded-2xl outline-ring"
    >
      <MovieCard
        movie={movie}
        hideContent
        state={<MovieStateBadge movie={movie} />}
        coverOverlay={
          <>
            <MovieRatingPill rating={movie.rating} />
            <MovieCoverCaption title={title} subtitle={movie.release_date} />
          </>
        }
      />
    </MovieDetailTrigger>
  )
}
