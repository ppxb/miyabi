import { Link, useLocation } from '@tanstack/react-router'
import { BookmarkIcon, CompassIcon, FilmIcon, SettingsIcon, type LucideIcon } from 'lucide-react'

import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar
} from '@/components/ui/sidebar'

type AppNavItem = {
  label: string
  icon: LucideIcon
  to: '/' | '/discover' | '/subscriptions' | '/settings'
}

export const APP_NAV_ITEMS: readonly AppNavItem[] = [
  { label: '媒体库', icon: FilmIcon, to: '/' },
  { label: '探索', icon: CompassIcon, to: '/discover' },
  { label: '收藏与关注', icon: BookmarkIcon, to: '/subscriptions' },
  { label: '设置', icon: SettingsIcon, to: '/settings' }
]

export function AppNavigation({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = useLocation({ select: location => location.pathname })
  const { state, isMobile } = useSidebar()
  const collapsed = state === 'collapsed' && !isMobile

  return (
    <SidebarMenu className="gap-1">
      {APP_NAV_ITEMS.map(item => {
        const active =
          item.to === '/'
            ? pathname === '/'
            : pathname === item.to ||
              pathname.startsWith(`${item.to}/`) ||
              (item.to === '/discover' && pathname === '/search')

        return (
          <SidebarMenuItem key={item.to}>
            <SidebarMenuButton
              render={
                <Link
                  to={item.to}
                  activeOptions={{ exact: item.to === '/', includeSearch: false }}
                  aria-current={active ? 'page' : undefined}
                  onClick={onNavigate}
                />
              }
              isActive={active}
              tooltip={collapsed ? item.label : undefined}
            >
              <item.icon aria-hidden="true" />
              <span>{item.label}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        )
      })}
    </SidebarMenu>
  )
}
