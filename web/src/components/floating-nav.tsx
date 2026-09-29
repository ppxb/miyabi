import { Link } from '@tanstack/react-router'
import type { LucideIcon } from 'lucide-react'

import { buttonVariants } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from 'cn'
import type { FileRoutesByTo } from '@/routeTree.gen'

type FloatingNavTo = keyof FileRoutesByTo

export type FloatingNavItem = {
  id: string
  icon: LucideIcon
  label: string
  to: FloatingNavTo
}

type FloatingNavProps = {
  items: FloatingNavItem[]
}

export function FloatingNav({ items }: FloatingNavProps) {
  return (
    <nav className="fixed bottom-8 left-1/2 z-50 flex -translate-x-1/2 items-center gap-1 rounded-full border border-border/70 bg-background/85 p-1.5 text-foreground backdrop-blur sm:gap-2 sm:p-1">
      <ul className="flex items-center gap-1 sm:gap-2">
        {items.map(item => (
          <NavItem key={item.id} item={item} />
        ))}
      </ul>
    </nav>
  )
}

type NavItemProps = {
  item: FloatingNavItem
}

function NavItem({ item }: NavItemProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Link
          to={item.to}
          activeOptions={{ exact: item.to === '/', includeSearch: false }}
          activeProps={{
            className: cn(buttonVariants({ variant: 'default', size: 'icon' }), 'size-11 sm:size-9')
          }}
          inactiveProps={{
            className: cn(buttonVariants({ variant: 'ghost', size: 'icon' }), 'size-11 sm:size-9')
          }}
        >
          <item.icon className="size-5" />
        </Link>
      </TooltipTrigger>
      <TooltipContent side="top">{item.label}</TooltipContent>
    </Tooltip>
  )
}
