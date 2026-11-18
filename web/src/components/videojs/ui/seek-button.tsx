'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { SeekButton as SeekButtonPrimitive } from '@videojs/react'
import { SeekIcon as SeekIconPrimitive } from '@videojs/react/icons'

import { Button } from '@/components/videojs/ui/button'
import { cn, resolveClassName } from '@/lib/resolve-class-name'

export type SeekButtonProps = Omit<SeekButtonPrimitive.Props, 'children'>

export function SeekButton({ className, seconds = 10, ...props }: SeekButtonProps = {}) {
  return (
    <SeekButtonPrimitive
      render={<Button />}
      className={state => cn('media-seek-button', resolveClassName(className, state))}
      seconds={seconds}
      {...props}
    >
      <div className={'relative grid'}>
        <SeekIconPrimitive
          className={cn(
            'col-start-1 row-start-1 size-media-icon drop-shadow-media-icon [text-shadow:inherit]',
            'transition-[opacity,scale] duration-media-base ease-out',
            seconds < 0 && '-scale-x-100'
          )}
        />
        <span
          className={cn(
            'absolute bottom-[-3px] text-media-xs font-medium tracking-tighter tabular-nums',
            seconds < 0 ? '-left-px' : '-right-px'
          )}
        >
          {Math.abs(seconds)}
        </span>
      </div>
    </SeekButtonPrimitive>
  )
}
