import { useState } from 'react'
import { useMediaPlayer, useMediaState } from '@vidstack/react'
import {
  useDefaultLayoutWord,
  type DefaultLayoutIconProps,
  type DefaultLayoutWord
} from '@vidstack/react/player/layouts/default'
import type { LucideIcon } from 'lucide-react'

import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

// Icon slots let the shared Tooltip cover the whole button without replacing Vidstack's controls.
export function withPlayerTooltip(Icon: LucideIcon, word: DefaultLayoutWord) {
  return function PlayerControlIcon(props: DefaultLayoutIconProps) {
    const player = useMediaPlayer()
    const controlsVisible = useMediaState('controlsVisible')
    const fullscreen = useMediaState('fullscreen')
    const label = useDefaultLayoutWord(word)
    const [open, setOpen] = useState(false)

    return (
      <Tooltip open={controlsVisible && open} onOpenChange={setOpen} delayDuration={0}>
        <TooltipTrigger asChild>
          <span className="absolute inset-0 inline-flex items-center justify-center">
            <Icon {...props} />
          </span>
        </TooltipTrigger>
        <TooltipContent container={fullscreen ? player?.el : undefined} side="top" align="center">
          {label}
        </TooltipContent>
      </Tooltip>
    )
  }
}
