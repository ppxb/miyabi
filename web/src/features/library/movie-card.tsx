import { PlayIcon } from 'lucide-react'

import { MovieCard, MovieCoverCaption, MovieRatingPill } from '@/components/movie'
import { Badge } from '@/components/ui/badge'
import { ContextMenu, ContextMenuTrigger } from '@/components/ui/context-menu'
import { LibraryMovieActions } from './movie-actions'

/** The fields the library card actually renders; kept narrow so callers pass
 * only what the card needs (e.g. a history item) without fabricating a movie. */
export type LibraryMovieCardData = {
  id: number
  code: string
  title: string
  cover?: string
  rating: number
  actors: Array<{ name: string }>
  source?: string
  has_subtitle?: boolean
  has_uncensored?: boolean
  has_cracked?: boolean
  finished?: boolean
}

export function LibraryMovieCard({
  movie,
  onPlay,
  progress
}: {
  movie: LibraryMovieCardData
  onPlay?: () => void
  progress?: number
}) {
  const title = movie.title || movie.code
  const actors = movie.actors
    .map(actor => actor.name)
    .filter(Boolean)
    .join(' / ')

  return (
    <ContextMenu>
      <ContextMenuTrigger className="block h-full min-w-0">
        <div
          role="button"
          tabIndex={0}
          className="group relative block h-full min-w-0 cursor-pointer rounded-2xl outline-ring"
          onClick={() => onPlay?.()}
          onKeyDown={event => {
            if (event.key !== 'Enter' && event.key !== ' ') return
            event.preventDefault()
            if (!event.repeat) onPlay?.()
          }}
        >
          <MovieCard
            movie={movie}
            hideContent
            state={
              movie.source === '115' ||
              movie.has_subtitle ||
              movie.has_uncensored ||
              movie.has_cracked ||
              movie.finished ? (
                <>
                  {movie.source === '115' ? <Badge variant="frosted">115</Badge> : null}
                  {movie.has_subtitle ? <Badge variant="frosted">中文</Badge> : null}
                  {movie.has_uncensored || movie.has_cracked ? (
                    <Badge variant="frosted">无码</Badge>
                  ) : null}
                  {movie.finished ? <Badge variant="success">已看完</Badge> : null}
                </>
              ) : undefined
            }
            coverOverlay={
              <>
                <MovieRatingPill rating={movie.rating} />
                <MovieCoverCaption title={title} subtitle={actors || 'N/A'}>
                  {progress !== undefined ? (
                    <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-white/25">
                      <div
                        className="h-full bg-success"
                        style={{ width: `${Math.max(0, Math.min(100, progress))}%` }}
                      />
                    </div>
                  ) : null}
                </MovieCoverCaption>
              </>
            }
          />
          <span className="pointer-events-none absolute inset-0 grid place-items-center opacity-0 transition-opacity group-hover:opacity-100">
            <span className="grid size-12 place-items-center rounded-full bg-black/55 text-white backdrop-blur-sm">
              <PlayIcon className="size-6 fill-current" />
            </span>
          </span>
        </div>
      </ContextMenuTrigger>
      <LibraryMovieActions movie={movie} />
    </ContextMenu>
  )
}
