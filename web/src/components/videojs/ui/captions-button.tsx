'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { CaptionsButton as CaptionsButtonPrimitive } from '@videojs/react'
import {
  CaptionsOffIcon as CaptionsOffIconPrimitive,
  CaptionsOnIcon as CaptionsOnIconPrimitive
} from '@videojs/react/icons'

import { Button } from '@/components/videojs/ui/button'
import { cn, resolveClassName } from '@/lib/resolve-class-name'

export type CaptionsButtonProps = Omit<CaptionsButtonPrimitive.Props, 'children'>

export function CaptionsButton({ className, ...props }: CaptionsButtonProps = {}) {
  return (
    <CaptionsButtonPrimitive
      render={<Button />}
      className={state => cn('group/captions', resolveClassName(className, state))}
      {...props}
    >
      <CaptionsOffIconPrimitive
        className={cn(
          'col-start-1 row-start-1 size-media-icon drop-shadow-media-icon [text-shadow:inherit]',
          'transition-[opacity,scale] duration-media-base ease-out',
          'opacity-0 group-not-data-active/captions:scale-100 group-not-data-active/captions:opacity-100'
        )}
      />
      <CaptionsOnIconPrimitive
        className={cn(
          'col-start-1 row-start-1 size-media-icon drop-shadow-media-icon [text-shadow:inherit]',
          'transition-[opacity,scale] duration-media-base ease-out',
          'opacity-0 group-data-active/captions:scale-100 group-data-active/captions:opacity-100'
        )}
      />
    </CaptionsButtonPrimitive>
  )
}
