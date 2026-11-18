'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { Poster as PosterPrimitive } from '@videojs/react'

import { cn, resolveClassName } from '@/lib/resolve-class-name'

export interface PosterProps extends Omit<PosterPrimitive.ImageProps, 'children' | 'render'> {
  /** Draws the poster image in place of the one the skin renders. */
  renderImage?: PosterPrimitive.ImageProps['render']

  children?: PosterPrimitive.RootProps['children']
}

export function Poster({ children, className, renderImage, ...props }: PosterProps = {}) {
  return (
    <PosterPrimitive.Root
      className={state =>
        cn(
          'pointer-events-none layer-media',
          'transition-opacity duration-media-slower not-data-visible:opacity-0',
          resolveClassName(className, state)
        )
      }
    >
      <PosterPrimitive.Image
        render={renderImage}
        className={cn('layer-media object-media', '[&:not([src]):not([srcset])]:invisible')}
        {...props}
      />

      {children}
    </PosterPrimitive.Root>
  )
}
