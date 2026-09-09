import { useMatch } from '@tanstack/react-router'
import { PlayIcon } from 'lucide-react'

import { useDiscoverMovie } from '@/api/discover'
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
  const openPlayer = useUIStore(state => state.openPlayer)

  const libraryID = movie.data?.library_id
  if (!movieID || movie.data?.state !== 'in_library' || libraryID === undefined) return null

  return (
    <>
      <Separator orientation="vertical" className="data-vertical:h-6 data-vertical:self-center" />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-11 cursor-pointer sm:size-9"
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
