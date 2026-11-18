'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { settingsText } from '@videojs/core/i18n/text/menu'
import { Menu } from '@videojs/react'
import { Text as TextPrimitive } from '@videojs/react'
import { GearIcon as GearIconPrimitive } from '@videojs/react/icons'
import type { ClassValue } from 'cn'

import { Button } from '@/components/videojs/ui/button'
import { ButtonTooltip } from '@/components/videojs/ui/button-tooltip'
import { cn, resolveClassName } from '@/lib/resolve-class-name'

export interface SettingsMenuProps extends Omit<Menu.RootProps, 'children'> {
  className?: ClassValue

  children?: Menu.ContentProps['children']
}

export function SettingsMenu({ children, className, ...props }: SettingsMenuProps) {
  return (
    <Menu.Root side="top" align="center" {...props}>
      <ButtonTooltip
        label={<TextPrimitive token={settingsText.key}>{settingsText.text}</TextPrimitive>}
        side="top"
      >
        <Menu.Trigger
          render={<Button />}
          className={state => cn('group/settings', resolveClassName(className, state))}
        >
          <GearIconPrimitive
            className={cn(
              'col-start-1 row-start-1 size-media-icon drop-shadow-media-icon [text-shadow:inherit]',
              'transition-transform duration-media-base ease-in-out motion-reduce:transition-none',
              'group-aria-expanded/settings:rotate-90'
            )}
          />
          <TextPrimitive className={'sr-only'} token={settingsText.key}>
            {settingsText.text}
          </TextPrimitive>
        </Menu.Trigger>
      </ButtonTooltip>
      <Menu.Popup
        keepMounted
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
          'bg-media-popover text-media-popover-foreground surface-media after:surface-media-inset',
          'm-0 max-w-(--media-menu-available-width) min-w-44 overflow-hidden! rounded-media-popup border-0 p-1 [--media-popup-side-offset:var(--media-popover-side-offset)]',
          'max-h-[min(var(--media-menu-available-height,--spacing(56)),--spacing(56))] overscroll-none',
          'h-(--media-menu-height) w-(--media-menu-width)',
          'transition-media-popup media-transitioning:transition-media-popup',
          'transition-media-menu-resize'
        )}
      >
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
          {children}
        </Menu.Content>
      </Menu.Popup>
    </Menu.Root>
  )
}
