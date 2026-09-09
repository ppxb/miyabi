import { Link } from '@tanstack/react-router'
import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { buttonVariants } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
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
  activeId: string | undefined
  children?: ReactNode
}

export function FloatingNav({ items, activeId, children }: FloatingNavProps) {
  return (
    <nav className="fixed bottom-8 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-full border border-border/70 bg-background/85 p-1.5 text-foreground backdrop-blur sm:bottom-8 sm:p-1">
      <ul className="flex items-center gap-2 sm:gap-2">
        {items.map(item => (
          <NavItem key={item.id} item={item} isActive={item.id === activeId} />
        ))}
      </ul>
      {children}
    </nav>
  )
}

type NavItemProps = {
  item: FloatingNavItem
  isActive: boolean
}

function NavItem({ item, isActive }: NavItemProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Link
          to={item.to}
          className={cn(
            buttonVariants({ variant: isActive ? 'default' : 'ghost', size: 'icon' }),
            'size-11 sm:size-9'
          )}
        >
          <item.icon className="size-5" />
        </Link>
      </TooltipTrigger>
      <TooltipContent side="top">{item.label}</TooltipContent>
    </Tooltip>
  )
}
