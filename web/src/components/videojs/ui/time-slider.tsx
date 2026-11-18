'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import type { SliderPreviewOverflow } from '@videojs/core'
import { Slider as SliderPrimitive, TimeSlider as TimeSliderPrimitive } from '@videojs/react'
import { SpinnerIcon as SpinnerIconPrimitive } from '@videojs/react/icons'

import { SliderBuffer, SliderFill, SliderThumb, SliderTrack } from '@/components/videojs/ui/slider'
import { cn, resolveClassName } from '@/lib/resolve-class-name'

export interface TimeSliderProps extends Omit<TimeSliderPrimitive.RootProps, 'children'> {
  previewOverflow?: SliderPreviewOverflow | undefined
  /** Draws the thumbnail preview image in place of the one the skin renders. */
  renderThumbnail?: SliderPrimitive.Thumbnail.ImageProps['render']
}

export function TimeSlider({
  className,
  previewOverflow = 'visible',
  renderThumbnail,
  ...props
}: TimeSliderProps = {}) {
  return (
    <TimeSliderPrimitive.Root
      className={state =>
        cn(
          'group/slider relative flex flex-1 cursor-pointer items-center justify-center outline-hidden',
          'data-disabled:pointer-events-none',
          'transition-[--media-slider-fill,--media-slider-buffer] duration-media-slider ease-out data-dragging:duration-0',
          'rounded-media-pill',
          'data-[orientation=horizontal]:h-(--media-slider-height,--spacing(8))',
          'data-[orientation=vertical]:w-8 data-[orientation=vertical]:min-w-0',
          'data-[orientation=horizontal]:min-w-18 data-[orientation=vertical]:h-18',
          'data-playing:transition-[--media-slider-buffer]',
          resolveClassName(className, state)
        )
      }
      {...props}
    >
      <TimeSliderPrimitive.Chapters
        className={'relative flex size-full min-h-0 min-w-0 flex-1 items-center rounded-[inherit]'}
        renderChapter={props => (
          <div
            className={cn(
              'group/chapter absolute inset-0 flex min-h-0 min-w-0 items-center justify-center',
              '[--media-chapter-inset-end:0.5] [--media-chapter-inset-start:0.5]',
              'first-of-type:[--media-chapter-inset-start:0] last-of-type:[--media-chapter-inset-end:0]',
              'data-[orientation=horizontal]:clip-media-chapter-x data-[orientation=vertical]:clip-media-chapter-y'
            )}
            {...props}
          >
            <TimeSliderPrimitive.Track
              render={<SliderTrack />}
              className={cn(
                'transition-[height,width] duration-media-slow ease-out',
                'data-[orientation=horizontal]:before:clip-media-chapter-track-x data-[orientation=vertical]:before:clip-media-chapter-track-y',
                'group-data-highlighted/chapter:data-[orientation=horizontal]:h-1.75',
                'group-data-highlighted/chapter:data-[orientation=vertical]:w-1.75'
              )}
            >
              <TimeSliderPrimitive.Buffer
                render={<SliderBuffer />}
                className={cn(
                  'group-data-highlighted/chapter:data-[orientation=horizontal]:before:min-w-1.75',
                  'group-data-highlighted/chapter:data-[orientation=vertical]:before:min-h-1.75'
                )}
              />
              <TimeSliderPrimitive.Fill
                render={<SliderFill />}
                className={cn(
                  'group-data-highlighted/chapter:data-[orientation=horizontal]:before:min-w-1.75',
                  'group-data-highlighted/chapter:data-[orientation=vertical]:before:min-h-1.75'
                )}
              />
            </TimeSliderPrimitive.Track>
          </div>
        )}
      ></TimeSliderPrimitive.Chapters>
      <TimeSliderPrimitive.Thumb
        render={<SliderThumb />}
        className={cn(
          'opacity-0 focus-visible:opacity-100 data-interactive:opacity-100',
          'pointer-fine:group-hover/slider:scale-100 pointer-fine:group-hover/slider:opacity-100',
          'scale-80'
        )}
      />
      <TimeSliderPrimitive.Preview
        className={cn(
          'group/preview relative h-1 [--media-slider-preview-max-height:var(--media-slider-preview-max-width)]',
          'media-2xl:[--media-slider-preview-max-width:min(--spacing(48),100cqi)]',
          'before:pointer-events-none before:absolute before:z-1 before:-translate-1/2 before:scale-50 before:opacity-0',
          'before:transition-[opacity,scale] before:duration-media-slow before:ease-out',
          'data-pointing:not-data-dragging:before:scale-100 data-pointing:not-data-dragging:before:opacity-100',
          'min-w-(--media-slider-preview-max-width)',
          '[--media-slider-preview-max-width:min(--spacing(32),100cqi)]',
          '@min-[30rem]/media-root:[--media-slider-preview-max-width:min(--spacing(40),100cqi)]',
          'before:top-1/2 before:left-1/2 before:size-1 before:rounded-media-control before:bg-current'
        )}
        overflow={previewOverflow}
      >
        <SliderPrimitive.Thumbnail.Root
          className={cn(
            'absolute max-w-(--media-slider-preview-max-width) -translate-x-1/2 translate-y-media-hidden-preview-offset scale-media-hidden-preview opacity-0',
            'origin-bottom blur-media-hidden',
            'transition-[filter,opacity,scale] duration-media-base ease-out',
            'group-data-pointing/preview:scale-100 group-data-pointing/preview:opacity-100 group-data-pointing/preview:filter-none',
            'group-has-focus-visible/slider:scale-100 group-has-focus-visible/slider:opacity-100 group-has-focus-visible/slider:filter-none',
            'bg-media-popover text-media-popover-foreground surface-media after:surface-media-inset',
            'group/thumbnail pointer-events-none overflow-hidden rounded-media-popup bg-media-backdrop/90',
            'bottom-[calc(100%+var(--media-slider-preview-offset))]',
            'max-h-(--media-slider-preview-max-height)',
            'data-loading:aspect-video data-loading:w-(--media-slider-preview-max-width)',
            'left-1/2',
            'after:pointer-events-none after:absolute after:inset-0 after:rounded-[inherit] after:bg-(image:--media-thumbnail-gradient)'
          )}
        >
          <SliderPrimitive.Thumbnail.Image
            render={renderThumbnail}
            className={cn(
              'block transition-opacity duration-media-base ease-out',
              'group-data-loading/thumbnail:opacity-0'
            )}
          />

          <SpinnerIconPrimitive
            className={cn(
              'absolute top-1/2 left-1/2 z-10 size-media-icon -translate-x-1/2 -translate-y-1/2 opacity-0',
              'transition-opacity duration-media-base ease-out',
              'group-not-data-loading/thumbnail:[--media-spinner-animation:none]',
              'group-data-loading/thumbnail:opacity-100',
              'drop-shadow-media-icon'
            )}
          />
        </SliderPrimitive.Thumbnail.Root>
        <div
          className={cn(
            'absolute max-w-(--media-slider-preview-max-width) -translate-x-1/2 translate-y-media-hidden-preview-offset scale-media-hidden-preview opacity-0',
            'origin-bottom blur-media-hidden',
            'transition-[filter,opacity,scale] duration-media-base ease-out',
            'group-data-pointing/preview:scale-100 group-data-pointing/preview:opacity-100 group-data-pointing/preview:filter-none',
            'group-has-focus-visible/slider:scale-100 group-has-focus-visible/slider:opacity-100 group-has-focus-visible/slider:filter-none',
            'bottom-[calc(100%+var(--media-slider-preview-label-offset))] flex tabular-nums',
            'left-1/2 flex-col items-center'
          )}
        >
          <TimeSliderPrimitive.ChapterTitle
            className={cn(
              'max-w-(--media-slider-preview-max-width) min-w-0 truncate empty:hidden',
              'px-6'
            )}
          />
          <TimeSliderPrimitive.Value className={'tabular-nums'} type="pointer" />
        </div>
      </TimeSliderPrimitive.Preview>
    </TimeSliderPrimitive.Root>
  )
}
