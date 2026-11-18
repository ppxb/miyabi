'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { Menu } from '@videojs/react'
import { CheckIcon as CheckIconPrimitive } from '@videojs/react/icons'

import { cn, resolveClassName } from '@/lib/resolve-class-name'

export interface RadioItemProps extends Omit<Menu.RadioItemProps, 'children'> {
  children?: Menu.RadioItemProps['children']
}

export function RadioItem({ children, className, ...props }: RadioItemProps) {
  return (
    <Menu.RadioItem
      className={state =>
        cn(
          'group/menu-radio-item',
          'relative flex cursor-pointer items-center gap-1.5 rounded-media-menu-item px-2 py-1.5 text-start whitespace-nowrap select-none',
          'focus-ring-media',
          'media-highlighted:highlight-media',
          'focus-visible:outline-offset-2 focus-visible:outline-media-ring',
          'text-shadow-media',
          'transition-[background-color,color] duration-media-fast ease-in-out',
          'media-anchored:duration-media-instant media-anchored:media-highlighted:duration-media-slow',
          'justify-between text-inherit tabular-nums',
          'data-[availability=unavailable]:hidden data-[availability=unsupported]:hidden',
          'aria-disabled:pointer-events-none aria-disabled:cursor-not-allowed aria-disabled:opacity-50',
          'media-anchored:data-highlighted:[anchor-name:--media-menu-item-highlight-anchor]',
          'media-anchored:media-highlighted:bg-transparent',
          'data-custom:italic',
          resolveClassName(className, state)
        )
      }
      {...props}
    >
      {children}
      <Menu.ItemIndicator
        forceMount
        className={
          'ms-auto -me-1 shrink-0 opacity-0 group-aria-checked/menu-radio-item:opacity-100'
        }
      >
        <CheckIconPrimitive
          className={cn(
            'shrink-0 text-media-muted-foreground drop-shadow-media-icon',
            'size-media-icon',
            'group-media-highlighted/menu-radio-item:text-inherit'
          )}
        />
      </Menu.ItemIndicator>
    </Menu.RadioItem>
  )
}
