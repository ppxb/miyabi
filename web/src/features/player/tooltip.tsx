import { useState, type ComponentProps, type ReactNode } from 'react'
import { useMediaPlayer, useMediaState, useSliderState } from '@vidstack/react'
import {
  useDefaultLayoutWord,
  type DefaultLayoutIconProps,
  type DefaultLayoutWord
} from '@vidstack/react/player/layouts/default'
import type { LucideIcon } from 'lucide-react'

import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function PlayerTooltipContent(props: ComponentProps<typeof TooltipContent>) {
  const player = useMediaPlayer()

  return (
    <TooltipContent
      container={player?.el}
      collisionBoundary={player?.el}
      collisionPadding={8}
      side="top"
      align="center"
      {...props}
    />
  )
}

export function PlayerSliderTooltip({
  children,
  orientation = 'horizontal'
}: {
  children: ReactNode
  orientation?: 'horizontal' | 'vertical'
}) {
  const active = useSliderState('active')
  const controlsVisible = useMediaState('controlsVisible')
  const vertical = orientation === 'vertical'

  return (
    <Tooltip open={active && controlsVisible} disableHoverableContent>
      <TooltipTrigger asChild>
        <span
          className="pointer-events-none absolute size-(--media-slider-thumb-size)"
          style={
            vertical
              ? {
                  left: '50%',
                  bottom: 'var(--slider-pointer)',
                  transform: 'translate(-50%, 50%)'
                }
              : {
                  left: 'var(--slider-pointer)',
                  top: '50%',
                  transform: 'translate(-50%, -50%)'
                }
          }
        />
      </TooltipTrigger>
      <PlayerTooltipContent
        side={vertical ? 'right' : 'top'}
        updatePositionStrategy="always"
        className="pointer-events-none"
      >
        {children}
      </PlayerTooltipContent>
    </Tooltip>
  )
}

// Icon slots let the shared Tooltip cover the whole button without replacing Vidstack's controls.
export function withPlayerTooltip(Icon: LucideIcon, word: DefaultLayoutWord) {
  return function PlayerControlIcon(props: DefaultLayoutIconProps) {
    const controlsVisible = useMediaState('controlsVisible')
    const label = useDefaultLayoutWord(word)
    const [open, setOpen] = useState(false)

    return (
      <Tooltip open={controlsVisible && open} onOpenChange={setOpen} delayDuration={0}>
        <TooltipTrigger asChild>
          <span className="absolute inset-0 inline-flex items-center justify-center">
            <Icon {...props} />
          </span>
        </TooltipTrigger>
        <PlayerTooltipContent>{label}</PlayerTooltipContent>
      </Tooltip>
    )
  }
}
