import { StarIcon } from 'lucide-react'

import type { LibraryMovie } from '@/api/library'
import { MovieCover } from '@/components/movie/movie-cover'
import { MovieMetadata } from '@/features/movie-detail/metadata'
import { libraryMovieMetadata } from './movie-metadata'

// The library page includes its locally scraped metadata and cached artwork.
// Opening a hover card never requests JavDB movie details.
export function LibraryMovieHoverDetails({ movie }: { movie: LibraryMovie }) {
  const metadata = libraryMovieMetadata(movie)
  const source = movie.fanart || movie.cover || movie.poster || ''
  const hasMetadata = Boolean(
    movie.release_date ||
    movie.duration ||
    movie.rating ||
    metadata.maker ||
    metadata.series ||
    metadata.director ||
    metadata.actors.length ||
    metadata.tags.length
  )

  return (
    <>
      <div className="aspect-3/2 w-full overflow-hidden bg-muted">
        <MovieCover source={source} loading="eager" />
      </div>
      <div className="space-y-4 p-4">
        <div className="space-y-2.5">
          <h3 className="text-sm leading-6 font-semibold break-words">
            {movie.title || movie.code}
          </h3>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground tabular-nums">
            {movie.release_date ? <span>{movie.release_date}</span> : null}
            {movie.duration > 0 ? <span>{movie.duration} 分钟</span> : null}
            {movie.rating > 0 ? (
              <span
                className="inline-flex items-center gap-1"
                aria-label={`评分 ${movie.rating.toFixed(1)}`}
              >
                <StarIcon className="size-3.5" />
                {movie.rating.toFixed(1)}
              </span>
            ) : null}
          </div>
        </div>

        <MovieMetadata movie={metadata} />
        {!hasMetadata ? (
          <p className="text-xs leading-5 text-muted-foreground">暂无刮削详情</p>
        ) : null}
      </div>
    </>
  )
}
