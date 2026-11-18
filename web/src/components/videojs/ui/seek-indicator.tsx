'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { SeekIndicator as SeekIndicatorPrimitive } from '@videojs/react'
import { ChevronIcon as ChevronIconPrimitive } from '@videojs/react/icons'

import { cn, resolveClassName } from '@/lib/resolve-class-name'

export type SeekIndicatorProps = Omit<SeekIndicatorPrimitive.RootProps, 'children'>

export function SeekIndicator({ className, ...props }: SeekIndicatorProps = {}) {
  return (
    <SeekIndicatorPrimitive.Root
      className={state =>
        cn(
          'group/seek-status col-start-2 row-start-1 grid place-content-center gap-1 p-4 text-center',
          'media-2xl:p-6',
          'data-[direction=backward]:col-start-1 data-[direction=backward]:justify-self-start',
          'data-[direction=forward]:col-start-3 data-[direction=forward]:justify-self-end',
          resolveClassName(className, state)
        )
      }
      {...props}
    >
      <ChevronIconPrimitive
        className={cn(
          'hidden size-media-icon-lg group-data-direction/seek-status:block',
          'group-data-[direction=backward]/seek-status:scale-[-1_1]',
          'transition-[translate,opacity] duration-media-slow ease-in-out',
          'group-media-transitioning/seek-status:opacity-0',
          'group-data-[direction=forward]/seek-status:group-data-starting-style/seek-status:-translate-x-media-hidden-seek-offset',
          'group-data-[direction=backward]/seek-status:group-data-starting-style/seek-status:translate-x-media-hidden-seek-offset'
        )}
      />
      <SeekIndicatorPrimitive.Value className={'tabular-nums'} />
    </SeekIndicatorPrimitive.Root>
  )
}
