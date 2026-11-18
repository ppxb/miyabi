'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { Title as TitlePrimitive } from '@videojs/react'

import { cn, resolveClassName } from '@/lib/resolve-class-name'

export type TitleProps = Omit<TitlePrimitive.Props, 'children'>

export function Title({ className, ...props }: TitleProps = {}) {
  return (
    <TitlePrimitive
      className={state =>
        cn(
          'pointer-events-none absolute inset-0 isolate z-20 media-max-3xs:hidden',
          'truncate px-6 pt-2.5 text-[calc(var(--media-spacing)*4)] font-medium tracking-[-0.0125em] text-media-controls-foreground text-shadow-media',
          'media-md:px-6 media-md:pt-4',
          'origin-top duration-media-controls-enter ease-out',
          'not-data-visible:-translate-y-media-hidden-offset not-data-visible:opacity-0 not-data-visible:duration-media-controls',
          'pointer-fine:not-data-visible:blur-media-hidden',
          'leading-[calc(var(--media-control-size)-(--spacing(0.5)))]',
          'media-md:leading-[calc(var(--media-control-size)-(--spacing(2)))]',
          'media-max-2xs:text-media media-max-lg:pr-36',
          'transition-[filter,opacity,scale,translate]',
          'not-data-visible:scale-media-hidden',
          'media-lg:before:absolute media-lg:before:inset-x-0 media-lg:before:top-0 media-lg:before:-z-1 media-lg:before:h-(--media-controls-gradient-height)',
          'media-lg:before:bg-(image:--media-controls-top-gradient)',
          resolveClassName(className, state)
        )
      }
      {...props}
    />
  )
}
