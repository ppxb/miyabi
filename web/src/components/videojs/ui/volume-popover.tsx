'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import type { VolumeSliderProps as CoreVolumeSliderProps } from '@videojs/core'
import { VolumePopover as VolumePopoverPrimitive } from '@videojs/react'
import type { ClassValue } from 'cn'

import { ButtonTooltip } from '@/components/videojs/ui/button-tooltip'
import { MuteButton } from '@/components/videojs/ui/mute-button'
import { VolumeSlider } from '@/components/videojs/ui/volume-slider'
import { cn } from '@/lib/resolve-class-name'

export interface VolumePopoverProps extends Omit<VolumePopoverPrimitive.RootProps, 'children'> {
  className?: ClassValue
  orientation?: CoreVolumeSliderProps['orientation']
  showTooltip?: boolean
}

export function VolumePopover({
  className,
  showTooltip = false,
  side = 'top',
  orientation = 'vertical',
  ...props
}: VolumePopoverProps = {}) {
  return (
    <VolumePopoverPrimitive.Root openOnHover delay={200} closeDelay={100} side={side} {...props}>
      <ButtonTooltip delay={0} disabled={!showTooltip} sticky side="top">
        <VolumePopoverPrimitive.Trigger render={<MuteButton className={cn(className)} />} />
      </ButtonTooltip>
      <VolumePopoverPrimitive.Popup
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
          'rounded-media-control px-0 py-3 [--media-popup-side-offset:var(--media-popover-side-offset)]',
          'data-[side=right]:rounded-none data-[side=right]:surface-media-none! data-[side=right]:p-0 data-[side=right]:px-3 data-[side=right]:after:hidden',
          'data-[side=right]:[--media-popover-side-offset:0rem]'
        )}
      >
        <VolumeSlider orientation={orientation} />
      </VolumePopoverPrimitive.Popup>
    </VolumePopoverPrimitive.Root>
  )
}
