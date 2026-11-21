'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { Container as ContainerPrimitive } from '@videojs/react'

import { cn } from '@/lib/resolve-class-name'

export interface ContainerProps extends Omit<ContainerPrimitive.Props, 'children'> {
  children?: ContainerPrimitive.Props['children']
}

export function Container({ children, className, ...props }: ContainerProps) {
  return (
    <ContainerPrimitive
      className={cn(
        'media-skin',
        '@container/media-root relative isolate block h-full w-full overflow-clip rounded-media-player bg-media-background',
        'font-media text-media leading-normal subpixel-antialiased [--spacing:var(--media-spacing)]',
        'outline-none',
        'after:pointer-events-none after:absolute after:inset-0 after:z-10 after:rounded-[inherit]',
        'after:border after:border-(--media-frame-border) [&:fullscreen]:after:hidden',
        className
      )}
      {...props}
    >
      {children}
    </ContainerPrimitive>
  )
}
