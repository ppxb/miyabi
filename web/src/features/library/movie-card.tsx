import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { Badge } from '@/components/ui/badge'
import { ContextMenu, ContextMenuTrigger } from '@/components/ui/context-menu'
import { MovieDetailTrigger } from '@/features/movie-detail/detail-trigger'
import { LibraryMovieActions } from './movie-actions'

export function LibraryMovieCard({ movie, busy = false }: { movie: LibraryMovie; busy?: boolean }) {
  const title = movie.title || movie.code
  const actors = movie.actors
    .map(actor => actor.name)
    .filter(Boolean)
    .join(' / ')

  return (
    <ContextMenu>
      <ContextMenuTrigger className="block h-full min-w-0">
        <MovieDetailTrigger
          movie={{ libraryId: movie.id }}
          className="block h-full min-w-0 rounded-2xl outline-ring"
        >
          <MovieCard
            movie={movie}
            titleTooltip={false}
            hideContent
            state={movie.source === '115' ? <Badge variant="frosted">115</Badge> : undefined}
            coverOverlay={
              <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent p-3 pt-8">
                <h3 className="line-clamp-2 text-sm leading-5 font-semibold text-white">{title}</h3>
                <p className="mt-0.5 line-clamp-1 text-xs text-white/75">{actors || 'N/A'}</p>
              </div>
            }
          />
        </MovieDetailTrigger>
      </ContextMenuTrigger>
      <LibraryMovieActions movie={movie} disabled={busy} />
    </ContextMenu>
  )
}
