import { ChevronIcon as ChevronIconPrimitive } from '@videojs/react/icons'
import type { ClassValue } from 'cn'

import { cn } from '@/lib/resolve-class-name'

export interface MenuChevronProps {
  back?: boolean
  className?: ClassValue
}

export function MenuChevron({ back = false, className }: MenuChevronProps = {}) {
  return (
    <ChevronIconPrimitive
      className={cn(
        back
          ? 'size-media-icon-sm shrink-0 rotate-180 text-media-muted-foreground drop-shadow-media-icon group-media-highlighted/menu-back-item:text-inherit rtl:scale-[1_1] rtl:rotate-0'
          : 'size-media-icon-sm shrink-0 text-media-muted-foreground drop-shadow-media-icon group-media-highlighted/menu-trigger-item:text-inherit rtl:scale-[-1_1]',
        className
      )}
    />
  )
}
