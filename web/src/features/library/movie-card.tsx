import { useState } from 'react'

import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { Button } from '@/components/ui/button'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { useUIStore } from '@/stores/ui'
import { LibraryMovieHoverDetails } from './movie-hover-details'
import { LibraryMovieStatus } from './movie-status'
import { useDesktopHover } from './use-desktop-hover'

export function LibraryMovieCard({ movie }: { movie: LibraryMovie }) {
  const canHover = useDesktopHover()
  return (
    <LibraryMovieCardContent
      key={canHover ? 'desktop' : 'touch'}
      movie={movie}
      canHover={canHover}
    />
  )
}

function LibraryMovieCardContent({ movie, canHover }: { movie: LibraryMovie; canHover: boolean }) {
  const openPlayer = useUIStore(state => state.openPlayer)
  const [open, setOpen] = useState(false)

  return (
    <HoverCard
      open={canHover && open}
      onOpenChange={value => setOpen(canHover && value)}
      openDelay={400}
      closeDelay={180}
    >
      <HoverCardTrigger asChild>
        <Button
          variant="ghost"
          className="block h-auto min-w-0 rounded-2xl p-0 text-left whitespace-normal hover:bg-transparent hover:text-current dark:hover:bg-transparent"
          aria-label={`播放 ${movie.code}${movie.title ? `：${movie.title}` : ''}`}
          onClick={() => {
            setOpen(false)
            openPlayer(movie.id)
          }}
        >
          <MovieCard movie={movie} titleTooltip={!canHover}>
            <LibraryMovieStatus movie={movie} />
          </MovieCard>
        </Button>
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
