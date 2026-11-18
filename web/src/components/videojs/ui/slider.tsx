'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import type { ComponentProps } from 'react'

import { cn } from '@/lib/resolve-class-name'

/** Shared slider track. */
export type SliderTrackProps = ComponentProps<'div'>

export function SliderTrack({ className, ...props }: SliderTrackProps) {
  return (
    <div
      className={cn(
        'relative isolate w-full rounded-media-pill select-none before:pointer-events-none before:absolute before:inset-0 before:rounded-media-control before:bg-current/20',
        'data-[orientation=horizontal]:h-1 data-[orientation=vertical]:h-full data-[orientation=vertical]:w-1',
        className
      )}
      {...props}
    />
  )
}

/** Shared slider fill. */
export type SliderFillProps = ComponentProps<'div'>

export function SliderFill({ className, ...props }: SliderFillProps) {
  return (
    <div
      className={cn(
        'pointer-events-none absolute inset-0 before:absolute before:size-full before:rounded-media-control',
        'data-[orientation=horizontal]:before:left-0 data-[orientation=horizontal]:before:min-w-1',
        'data-[orientation=vertical]:before:bottom-0 data-[orientation=vertical]:before:min-h-1',
        'before:bg-media-primary',
        'data-[orientation=horizontal]:clip-media-x-[--media-slider-fill]',
        'group-data-dragging/slider:data-[orientation=horizontal]:clip-media-x-[--media-slider-pointer]',
        'data-[orientation=vertical]:clip-media-y-[--media-slider-fill]',
        'group-data-dragging/slider:data-[orientation=vertical]:clip-media-y-[--media-slider-pointer]',
        className
      )}
      {...props}
    />
  )
}

/** Shared slider buffer. */
export type SliderBufferProps = ComponentProps<'div'>

export function SliderBuffer({ className, ...props }: SliderBufferProps) {
  return (
    <div
      className={cn(
        'pointer-events-none absolute inset-0 before:absolute before:size-full before:rounded-media-control',
        'data-[orientation=horizontal]:before:left-0 data-[orientation=horizontal]:before:min-w-1',
        'data-[orientation=vertical]:before:bottom-0 data-[orientation=vertical]:before:min-h-1',
        'before:bg-current/20',
        'data-[orientation=horizontal]:clip-media-x-[--media-slider-buffer]',
        'data-[orientation=vertical]:clip-media-y-[--media-slider-buffer]',
        className
      )}
      {...props}
    />
  )
}

/** Shared slider thumb. */
export type SliderThumbProps = ComponentProps<'div'>

export function SliderThumb({ className, ...props }: SliderThumbProps) {
  return (
    <div
      className={cn(
        'absolute top-1/2 left-(--media-slider-fill,0%) z-10 size-3 -translate-x-1/2 -translate-y-1/2 rounded-media-control bg-white',
        'transition-[opacity,height,width,outline-offset,scale] duration-media-slider ease-out select-none',
        'group-data-dragging/slider:scale-90',
        'data-[orientation=vertical]:top-[calc(100%-var(--media-slider-fill,0%))] data-[orientation=vertical]:left-1/2',
        'group-data-dragging/slider:data-[orientation=horizontal]:left-(--media-slider-pointer)',
        'group-data-dragging/slider:data-[orientation=vertical]:top-[calc(100%-var(--media-slider-pointer))]',
        'shadow-media-thumb outline-transparent',
        'outline-4 -outline-offset-4',
        'hover:outline-offset-0 hover:outline-current/15 focus-visible:outline-offset-0 focus-visible:outline-current/15',
        'after:pointer-events-none after:absolute after:-inset-1 after:scale-50 after:rounded-[inherit] after:opacity-0',
        'after:shadow-[0_0_0_2px_currentColor] after:transition-[opacity,scale] after:duration-media-base after:ease-out',
        'focus-visible:after:scale-100 focus-visible:after:opacity-100',
        className
      )}
      {...props}
    />
  )
}
