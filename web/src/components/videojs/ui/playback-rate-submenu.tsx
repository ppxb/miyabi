'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { speedText } from '@videojs/core/i18n/text/menu'
import { Menu } from '@videojs/react'
import { Text as TextPrimitive } from '@videojs/react'
import { SpeedIcon as SpeedIconPrimitive } from '@videojs/react/icons'
import { PlaybackRateRadioGroup } from '@videojs/react/ui/playback-rate-radio-group'

import { MenuChevron } from '@/components/videojs/ui/menu-chevron'
import { RadioItem } from '@/components/videojs/ui/radio-item'
import { cn } from '@/lib/resolve-class-name'

export type PlaybackRateSubmenuProps = Omit<Menu.RootProps, 'children'>

export function PlaybackRateSubmenu({ ...props }: PlaybackRateSubmenuProps = {}) {
  return (
    <Menu.Root {...props}>
      <PlaybackRateRadioGroup.Root>
        <Menu.Trigger
          className={cn(
            'group/menu-trigger-item',
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
            'media-anchored:media-highlighted:bg-transparent'
          )}
        >
          <SpeedIconPrimitive
            className={cn(
              'shrink-0 text-media-muted-foreground drop-shadow-media-icon',
              'size-media-icon',
              'group-media-highlighted/menu-trigger-item:text-inherit'
            )}
          />
          <TextPrimitive token={speedText.key}>{speedText.text}</TextPrimitive>
          <span className={'ms-auto inline-flex min-w-0 items-center gap-1 ps-2'}>
            <PlaybackRateRadioGroup.Value className={'max-w-24 truncate opacity-65'} />
            <MenuChevron />
          </span>
        </Menu.Trigger>
        <Menu.Content
          className={cn(
            '[anchor-scope:--media-menu-item-highlight-anchor]',
            'media-anchored:before:anchor-media-highlight',
            'media-transitioning:before:hidden',
            'media-transitioning:[&_*]:before:hidden',
            'media-anchored:has-data-[highlighted=]:before:duration-0',
            'not-data-submenu:flex not-data-submenu:flex-col not-data-submenu:gap-0.5',
            'not-data-submenu:data-child-open:blur-media-hidden',
            'not-data-submenu:data-child-open:before:hidden',
            'data-submenu:media-transitioning:pointer-events-none data-submenu:media-transitioning:overflow-hidden',
            'data-submenu:media-transitioning:blur-media-hidden',
            'absolute overflow-auto overscroll-none outline-hidden',
            'transition-[translate,filter] duration-media-menu ease-out',
            'not-data-submenu:inset-x-1 not-data-submenu:top-1',
            'not-data-submenu:max-h-[calc(min(var(--media-menu-available-height,--spacing(56)),--spacing(56))-(--spacing(2)))]',
            'not-data-submenu:data-child-open:-translate-x-full',
            'not-data-submenu:data-child-open:rtl:translate-x-full',
            'data-submenu:inset-x-0 data-submenu:top-0 data-submenu:z-10 data-submenu:max-h-[inherit] data-submenu:p-1',
            'data-submenu:media-transitioning:translate-x-full data-submenu:media-transitioning:rtl:-translate-x-full'
          )}
        >
          <Menu.Item
            className={cn(
              'group/menu-back-item',
              'relative flex cursor-pointer items-center gap-1.5 rounded-media-menu-item px-2 py-1.5 text-start whitespace-nowrap select-none',
              'focus-ring-media',
              'media-highlighted:highlight-media',
              'focus-visible:outline-offset-2 focus-visible:outline-media-ring',
              'text-shadow-media',
              'transition-[background-color,color] duration-media-fast ease-in-out',
              'media-anchored:duration-media-instant media-anchored:media-highlighted:duration-media-slow',
              'mb-0.5 w-full'
            )}
          >
            <MenuChevron back />
            <TextPrimitive token={speedText.key}>{speedText.text}</TextPrimitive>
          </Menu.Item>
          <Menu.Separator
            className={cn(
              'my-1 block border-b border-media-border media-high-contrast:border-media-foreground/25',
              'shadow-media-separator'
            )}
          />
          <PlaybackRateRadioGroup.Options
            className={cn(
              'flex flex-col gap-0.5',
              '[anchor-scope:--media-menu-item-highlight-anchor]',
              'media-anchored:before:anchor-media-highlight',
              'media-transitioning:before:hidden',
              'media-transitioning:[&_*]:before:hidden',
              'media-anchored:has-data-[highlighted=]:before:duration-0'
            )}
            renderItem={(props, item) => (
              <RadioItem {...props}>
                <span>{item.label}</span>
              </RadioItem>
            )}
          ></PlaybackRateRadioGroup.Options>
        </Menu.Content>
      </PlaybackRateRadioGroup.Root>
    </Menu.Root>
  )
}
