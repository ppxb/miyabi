'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { StatusIndicator as StatusIndicatorPrimitive } from '@videojs/react'
import {
  CaptionsOnIcon as CaptionsOnIconPrimitive,
  CaptionsOffIcon as CaptionsOffIconPrimitive,
  FullscreenEnterIcon as FullscreenEnterIconPrimitive,
  FullscreenExitIcon as FullscreenExitIconPrimitive,
  PipEnterIcon as PipEnterIconPrimitive,
  PipExitIcon as PipExitIconPrimitive,
  PlayIcon as PlayIconPrimitive,
  PauseIcon as PauseIconPrimitive
} from '@videojs/react/icons'

import { cn, resolveClassName } from '@/lib/resolve-class-name'

const TOP_STATUS_ACTIONS = [
  'toggleSubtitles',
  'toggleFullscreen',
  'togglePictureInPicture'
] as const

const PLAYBACK_STATUS_ACTIONS = ['togglePaused'] as const

export type StatusIndicatorProps = Omit<StatusIndicatorPrimitive.RootProps, 'children'>

export function StatusIndicator({ className, ...props }: StatusIndicatorProps = {}) {
  return (
    <StatusIndicatorPrimitive.Root
      actions={TOP_STATUS_ACTIONS}
      className={state =>
        cn(
          'pointer-events-none absolute origin-top',
          'duration-media-fast ease-out',
          'media-transitioning:opacity-0 media-transitioning:duration-media-indicator media-transitioning:ease-in',
          'top-3 rounded-media-control font-medium',
          'bg-media-backdrop/25 text-media-popover-foreground surface-media after:surface-media-inset',
          'media-high-contrast:bg-media-background',
          'pointer-coarse:motion-media-[scale,translate,opacity]',
          'pointer-fine:motion-media-[scale,translate,filter,opacity]',
          'pointer-fine:media-transitioning:scale-media-hidden-indicator pointer-fine:media-transitioning:blur-media-hidden',
          'data-ending-style:translate-y-media-hidden-indicator-offset',
          'group/input-status',
          resolveClassName(className, state)
        )
      }
      {...props}
    >
      <div className={cn('items-center justify-between gap-2 px-2.5 py-1', 'flex', 'w-full')}>
        <CaptionsOnIconPrimitive
          className={cn(
            'hidden shrink-0',
            'group-data-[status=captions-on]/input-status:block',
            'mix-blend-difference'
          )}
        />
        <CaptionsOffIconPrimitive
          className={cn(
            'hidden shrink-0',
            'group-data-[status=captions-off]/input-status:block',
            'mix-blend-difference'
          )}
        />
        <FullscreenEnterIconPrimitive
          className={cn(
            'hidden shrink-0',
            'group-data-[status=fullscreen]/input-status:block',
            'mix-blend-difference'
          )}
        />
        <FullscreenExitIconPrimitive
          className={cn(
            'hidden shrink-0',
            'group-data-[status=exit-fullscreen]/input-status:block',
            'mix-blend-difference'
          )}
        />
        <PipEnterIconPrimitive
          className={cn(
            'hidden shrink-0',
            'group-data-[status=pip]/input-status:block',
            'mix-blend-difference'
          )}
        />
        <PipExitIconPrimitive
          className={cn(
            'hidden shrink-0',
            'group-data-[status=exit-pip]/input-status:block',
            'mix-blend-difference'
          )}
        />
        <StatusIndicatorPrimitive.Value className={cn('ml-auto', 'mix-blend-difference')} />
      </div>
    </StatusIndicatorPrimitive.Root>
  )
}

export type PlaybackStatusIndicatorProps = Omit<StatusIndicatorPrimitive.RootProps, 'children'>

export function PlaybackStatusIndicator({
  className,
  ...props
}: PlaybackStatusIndicatorProps = {}) {
  return (
    <StatusIndicatorPrimitive.Root
      actions={PLAYBACK_STATUS_ACTIONS}
      className={state =>
        cn(
          'group/playback-status col-start-2 row-start-1 grid place-content-center p-4 text-center',
          'transition-[opacity,scale] duration-media-slow ease-out',
          'media-transitioning:scale-media-hidden-playback media-transitioning:opacity-0',
          'data-ending-style:duration-media-fast data-ending-style:ease-in',
          'rounded-media-pill bg-media-backdrop/35 backdrop-filter-media-indicator pointer-coarse:hidden!',
          resolveClassName(className, state)
        )
      }
      {...props}
    >
      <PlayIconPrimitive
        className={cn(
          'col-start-1 row-start-1 scale-media-hidden-icon opacity-0',
          'transition-[opacity,scale] duration-media-base ease-out',
          'group-data-[status=play]/playback-status:scale-100 group-data-[status=play]/playback-status:opacity-100',
          'size-media-icon-lg',
          'group-data-[status=play]/playback-status:translate-x-px'
        )}
      />
      <PauseIconPrimitive
        className={cn(
          'col-start-1 row-start-1 scale-media-hidden-icon opacity-0',
          'transition-[opacity,scale] duration-media-base ease-out',
          'group-data-[status=pause]/playback-status:scale-100 group-data-[status=pause]/playback-status:opacity-100',
          'size-media-icon-lg'
        )}
      />
    </StatusIndicatorPrimitive.Root>
  )
}
