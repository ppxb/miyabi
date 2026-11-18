'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { BufferingIndicator as BufferingIndicatorPrimitive } from '@videojs/react'
import { SpinnerIcon as SpinnerIconPrimitive } from '@videojs/react/icons'

import { cn, resolveClassName } from '@/lib/resolve-class-name'

export type BufferingIndicatorProps = Omit<BufferingIndicatorPrimitive.Props, 'children'>

export function BufferingIndicator({ className, ...props }: BufferingIndicatorProps = {}) {
  return (
    <BufferingIndicatorPrimitive
      className={state =>
        cn(
          'pointer-events-none absolute inset-0 hidden place-content-center text-media-controls-foreground',
          'before:absolute before:inset-0 before:bg-media-backdrop/35 before:backdrop-filter-media-indicator',
          'not-data-visible:[--media-spinner-animation:none] data-visible:grid',
          resolveClassName(className, state)
        )
      }
      {...props}
    >
      <SpinnerIconPrimitive className={'relative z-30 size-media-icon drop-shadow-media-icon'} />
    </BufferingIndicatorPrimitive>
  )
}
