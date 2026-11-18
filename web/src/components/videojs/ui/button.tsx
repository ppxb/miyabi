'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import type { ComponentProps } from 'react'

import { cn } from '@/lib/resolve-class-name'

/** Shared button carrying the base interactive styles used by media controls. */
export type ButtonProps = ComponentProps<'button'>

export function Button({ className, ...props }: ButtonProps) {
  return (
    <button
      className={cn(
        'grid size-media-control min-h-0 shrink-0 touch-manipulation place-items-center rounded-media-control border-0 bg-transparent p-0 text-center text-inherit select-none [corner-shape:var(--media-control-corner-shape)]',
        'cursor-pointer focus-ring-media',
        '[transition-property:background-color,color,outline-offset,scale] duration-media-base ease-out will-change-[scale]',
        'media-highlighted:highlight-media',
        'not-aria-disabled:hover:highlight-media',
        'focus-visible:outline-offset-2 focus-visible:outline-media-ring',
        'not-aria-disabled:active:scale-[0.97]',
        'motion-reduce:scale-100 motion-reduce:[transition-property:background-color,color] motion-reduce:will-change-auto',
        'aria-disabled:cursor-not-allowed aria-disabled:opacity-50',
        className
      )}
      {...props}
    />
  )
}
