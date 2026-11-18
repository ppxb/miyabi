'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { MuteButton as MuteButtonPrimitive } from '@videojs/react'
import {
  VolumeOffIcon as VolumeOffIconPrimitive,
  VolumeLowIcon as VolumeLowIconPrimitive,
  VolumeHighIcon as VolumeHighIconPrimitive
} from '@videojs/react/icons'

import { Button } from '@/components/videojs/ui/button'
import { cn, resolveClassName } from '@/lib/resolve-class-name'

export type MuteButtonProps = Omit<MuteButtonPrimitive.Props, 'children'>

export function MuteButton({ className, ...props }: MuteButtonProps = {}) {
  return (
    <MuteButtonPrimitive
      render={<Button />}
      className={state => cn('group/mute', resolveClassName(className, state))}
      {...props}
    >
      <VolumeOffIconPrimitive
        className={cn(
          'col-start-1 row-start-1 size-media-icon drop-shadow-media-icon [text-shadow:inherit]',
          'transition-[opacity,scale] duration-media-base ease-out',
          'opacity-0 group-data-muted/mute:scale-100 group-data-muted/mute:opacity-100'
        )}
      />
      <VolumeLowIconPrimitive
        className={cn(
          'col-start-1 row-start-1 size-media-icon drop-shadow-media-icon [text-shadow:inherit]',
          'transition-[opacity,scale] duration-media-base ease-out',
          'opacity-0',
          'group-not-data-muted/mute:group-data-[volume-level=low]/mute:opacity-100',
          'group-not-data-muted/mute:group-data-[volume-level=low]/mute:scale-100'
        )}
      />
      <VolumeHighIconPrimitive
        className={cn(
          'col-start-1 row-start-1 size-media-icon drop-shadow-media-icon [text-shadow:inherit]',
          'transition-[opacity,scale] duration-media-base ease-out',
          'opacity-0',
          'group-not-data-muted/mute:group-not-data-[volume-level=low]/mute:opacity-100',
          'group-not-data-muted/mute:group-not-data-[volume-level=low]/mute:scale-100'
        )}
      />
    </MuteButtonPrimitive>
  )
}
