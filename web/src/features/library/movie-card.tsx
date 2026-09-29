import { Link } from '@tanstack/react-router'
import { useState } from 'react'

import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { LibraryMovieHoverDetails } from './movie-hover-details'
import { LibraryMovieStatus } from './movie-status'
import { useDesktopHover } from './use-desktop-hover'

export function LibraryMovieCard({ movie }: { movie: LibraryMovie }) {
  const canHover = useDesktopHover()
  const [open, setOpen] = useState(false)
  // Reset before committing children so restoring desktop mode cannot reopen
  // old hover content. The card and its image keep their component identity.
  if (!canHover && open) setOpen(false)

  const card = (
    <MovieCard movie={movie} titleTooltip={!canHover}>
      <LibraryMovieStatus movie={movie} />
    </MovieCard>
  )

  return (
    <HoverCard
      open={canHover && open}
      onOpenChange={value => setOpen(canHover && value)}
      openDelay={400}
      closeDelay={180}
    >
      <HoverCardTrigger asChild>
        {movie.javdb_id ? (
          <Link
            to="/discover/$movieId"
            search={previous => ({ main: previous.main || undefined })}
            params={{ movieId: movie.javdb_id }}
            className="block min-w-0 rounded-2xl outline-ring"
            onClick={() => setOpen(false)}
          >
            {card}
          </Link>
        ) : (
          <div className="min-w-0">{card}</div>
        )}
      </HoverCardTrigger>
      {canHover ? (
        <HoverCardContent
          side="right"
          align="start"
          sideOffset={12}
          collisionPadding={16}
          className="max-h-[min(42rem,var(--radix-hover-card-content-available-height))] w-96 max-w-[calc(100vw-2rem)] overflow-y-auto overscroll-contain p-0"
          role="region"
          aria-label={`${movie.code} 影片详情`}
          onClick={event => {
            if (event.target instanceof Element && event.target.closest('a')) setOpen(false)
          }}
        >
          <LibraryMovieHoverDetails movie={movie} />
        </HoverCardContent>
      ) : null}
    </HoverCard>
  )
}
