import { useMatch } from '@tanstack/react-router'
import { PlayIcon } from 'lucide-react'

import { useDiscoverMovie } from '@/api/discover'
import { useMovieState } from '@/api/movie-states'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useUIStore } from '@/stores/ui'

export function MoviePlaybackNavAction() {
  const movieID = useMatch({
    from: '/discover_/$movieId',
    shouldThrow: false,
    select: match => match.params.movieId
  })
  const movie = useDiscoverMovie(movieID ?? '', movieID !== undefined)
  const state = useMovieState(movie.data)
  const openPlayer = useUIStore(state => state.openPlayer)

  const libraryID = state.library_id
  if (!movieID || state.state !== 'in_library' || libraryID === undefined) return null

  return (
    <>
      <Separator orientation="vertical" className="data-vertical:h-6 data-vertical:self-center" />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-11 sm:size-9"
            onClick={() => openPlayer(libraryID)}
          >
            <PlayIcon className="size-5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">播放</TooltipContent>
      </Tooltip>
    </>
  )
}
