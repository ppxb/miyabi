'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { VolumeSlider as VolumeSliderPrimitive } from '@videojs/react'

import { SliderFill, SliderThumb, SliderTrack } from '@/components/videojs/ui/slider'
import { cn, resolveClassName } from '@/lib/resolve-class-name'

export type VolumeSliderProps = Omit<VolumeSliderPrimitive.RootProps, 'children'>

export function VolumeSlider({ className, ...props }: VolumeSliderProps = {}) {
  return (
    <VolumeSliderPrimitive.Root
      className={state =>
        cn(
          'group/slider relative flex flex-1 cursor-pointer items-center justify-center outline-hidden',
          'data-disabled:pointer-events-none',
          'transition-[--media-slider-fill,--media-slider-buffer] duration-media-slider ease-out data-dragging:duration-0',
          'rounded-media-pill',
          'data-[orientation=horizontal]:h-(--media-slider-height,--spacing(8))',
          'data-[orientation=vertical]:w-8 data-[orientation=vertical]:min-w-0',
          'data-[orientation=horizontal]:min-w-18 data-[orientation=vertical]:h-18',
          'media-volume-slider',
          resolveClassName(className, state)
        )
      }
      thumbAlignment="edge"
      {...props}
    >
      <VolumeSliderPrimitive.Track render={<SliderTrack />}>
        <VolumeSliderPrimitive.Fill render={<SliderFill />} />
      </VolumeSliderPrimitive.Track>
      <VolumeSliderPrimitive.Thumb render={<SliderThumb />} className={'scale-100 opacity-100'} />
    </VolumeSliderPrimitive.Root>
  )
}
