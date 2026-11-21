import { StarIcon } from 'lucide-react'

import type { DiscoverMovie } from '@/api/discover'
import { MovieCard } from '@/components/movie'
import { MovieStateBadge } from '@/components/movie/movie-badges'
import { MovieDetailTrigger } from '@/features/movie-detail/detail-trigger'
import { MovieSubscribeButton } from './movie-subscribe-button'

export function DiscoverMovieCard({
  movie,
  allowSubscribe = true
}: {
  movie: DiscoverMovie
  allowSubscribe?: boolean
}) {
  const subscribable = allowSubscribe && movie.release_status === 'upcoming' && movie.magnets_count === 0
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
            {subscribable ? (
              <div className="absolute top-2 right-2">
                <MovieSubscribeButton movie={movie} />
              </div>
            ) : movie.rating > 0 ? (
              <div className="pointer-events-none absolute top-2 right-2 flex items-center gap-1 rounded-full bg-black/55 px-2 py-0.5 text-xs font-semibold text-white backdrop-blur-sm">
                <StarIcon className="size-3.5 fill-yellow-400 text-yellow-400" />
                <span className="tabular-nums">{movie.rating.toFixed(1)}</span>
              </div>
            ) : null}
            <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent p-3 pt-8">
              <h3 className="truncate text-sm leading-5 font-semibold text-white" title={title}>
                {title}
              </h3>
              {movie.release_date ? (
                <p className="mt-0.5 text-[11px] leading-4 text-white/60">{movie.release_date}</p>
              ) : null}
            </div>
          </>
        }
      />
    </MovieDetailTrigger>
  )
}
