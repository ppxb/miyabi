import { StarIcon } from 'lucide-react'

import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { Badge } from '@/components/ui/badge'
import { ContextMenu, ContextMenuTrigger } from '@/components/ui/context-menu'
import { LibraryMovieActions } from './movie-actions'

export function LibraryMovieCard({
  movie,
  busy = false,
  onPlay
}: {
  movie: LibraryMovie
  busy?: boolean
  onPlay?: () => void
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
          className="block h-full min-w-0 cursor-pointer rounded-2xl outline-ring"
          onClick={() => onPlay?.()}
          onKeyDown={event => {
            if (event.key !== 'Enter' && event.key !== ' ') return
            event.preventDefault()
            if (!event.repeat) onPlay?.()
          }}
        >
          <MovieCard
            movie={movie}
            titleTooltip={false}
            hideContent
            state={movie.source === '115' ? <Badge variant="frosted">115</Badge> : undefined}
            coverOverlay={
              <>
                {movie.rating > 0 ? (
                  <div className="pointer-events-none absolute top-2 right-2 flex items-center gap-1 rounded-full bg-black/55 px-2 py-0.5 text-xs font-semibold text-white backdrop-blur-sm">
                    <StarIcon className="size-3.5 fill-yellow-400 text-yellow-400" />
                    <span className="tabular-nums">{movie.rating.toFixed(1)}</span>
                  </div>
                ) : null}
                <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent p-3 pt-8">
                  <h3 className="line-clamp-2 text-sm leading-5 font-semibold text-white">
                    {title}
                  </h3>
                  <p className="mt-0.5 line-clamp-1 text-xs text-white/75">{actors || 'N/A'}</p>
                </div>
              </>
            }
          />
        </div>
      </ContextMenuTrigger>
      <LibraryMovieActions movie={movie} disabled={busy} />
    </ContextMenu>
  )
}
