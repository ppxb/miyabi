'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { Tooltip } from '@videojs/react'
import type { ReactElement, ReactNode } from 'react'

import { cn } from '@/lib/resolve-class-name'

export interface ButtonTooltipProps extends Omit<Tooltip.RootProps, 'children'> {
  children: ReactElement
  label?: ReactNode
}

export function ButtonTooltip({ children, label, ...props }: ButtonTooltipProps) {
  return (
    <Tooltip.Root {...props}>
      <Tooltip.Trigger render={children} />
      <Tooltip.Popup
        className={cn(
          'm-0 overflow-visible border-0 text-inherit',
          'media-transitioning:scale-media-hidden-popup media-transitioning:opacity-0 media-transitioning:blur-media-hidden-popup',
          'data-starting-style:transform-[translate(var(--media-popup-translate-x-distance,0),var(--media-popup-translate-y-distance,0))]',
          'data-ending-style:transform-none',
          'data-[side=bottom]:origin-top data-[side=left]:origin-right data-[side=right]:origin-left data-[side=top]:origin-bottom',
          'data-[side=top]:[--media-popup-translate-y-distance:var(--media-popup-translate-distance)]',
          'data-[side=bottom]:[--media-popup-translate-y-distance:calc(var(--media-popup-translate-distance)*-1)]',
          'data-[side=left]:[--media-popup-translate-x-distance:var(--media-popup-translate-distance)]',
          'data-[side=right]:[--media-popup-translate-x-distance:calc(var(--media-popup-translate-distance)*-1)]',
          'before:pointer-events-auto before:absolute',
          'data-[side=top]:before:inset-x-0 data-[side=top]:before:top-full',
          'data-[side=bottom]:before:inset-x-0 data-[side=bottom]:before:bottom-full',
          'data-[side=left]:before:inset-y-0 data-[side=left]:before:left-full',
          'data-[side=right]:before:inset-y-0 data-[side=right]:before:right-full',
          'data-[side=bottom]:before:h-(--media-popup-side-offset) data-[side=top]:before:h-(--media-popup-side-offset)',
          'data-[side=left]:before:w-(--media-popup-side-offset) data-[side=right]:before:w-(--media-popup-side-offset)',
          'transition-media-popup data-ending-style:duration-media-instant',
          'bg-media-popover text-media-popover-foreground surface-media after:surface-media-inset',
          'rounded-media-control py-1 text-media whitespace-nowrap [--media-popup-side-offset:var(--media-tooltip-side-offset)]',
          'data-open:flex data-open:items-center data-open:gap-1',
          'px-2.5'
        )}
      >
        {label ?? <Tooltip.Label />}
        {!label && (
          <Tooltip.Shortcut
            className={
              'min-w-[1.5em] rounded-[--spacing(1)] bg-media-muted p-[0.1em] text-center [font-family:inherit] text-media-sm leading-tight font-semibold'
            }
          />
        )}
      </Tooltip.Popup>
    </Tooltip.Root>
  )
}
