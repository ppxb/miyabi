import { PlayIcon, StarIcon } from 'lucide-react'

import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { Badge } from '@/components/ui/badge'
import { ContextMenu, ContextMenuTrigger } from '@/components/ui/context-menu'
import { LibraryMovieActions } from './movie-actions'

export function LibraryMovieCard({
  movie,
  onPlay,
  progress
}: {
  movie: LibraryMovie
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
              movie.source === '115' || movie.has_subtitle || movie.finished ? (
                <>
                  {movie.source === '115' ? <Badge variant="frosted">115</Badge> : null}
                  {movie.has_subtitle ? <Badge variant="frosted">中文</Badge> : null}
                  {movie.finished ? <Badge variant="success">已看完</Badge> : null}
                </>
              ) : undefined
            }
            coverOverlay={
              <>
                {movie.rating > 0 ? (
                  <div className="pointer-events-none absolute top-2 right-2 flex items-center gap-1 rounded-full bg-black/55 px-2 py-0.5 text-xs font-semibold text-white backdrop-blur-sm">
                    <StarIcon className="size-3.5 fill-yellow-400 text-yellow-400" />
                    <span className="tabular-nums">{movie.rating.toFixed(1)}</span>
                  </div>
                ) : null}
                <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent p-3 pt-8">
                  <h3 className="truncate text-sm leading-5 font-semibold text-white" title={title}>
                    {title}
                  </h3>
                  <p className="mt-0.5 line-clamp-1 text-xs text-white/75">{actors || 'N/A'}</p>
                  {progress !== undefined ? (
                    <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-white/25">
                      <div
                        className="h-full bg-success"
                        style={{ width: `${Math.max(0, Math.min(100, progress))}%` }}
                      />
                    </div>
                  ) : null}
                </div>
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
