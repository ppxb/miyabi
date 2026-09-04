import type { PropsWithChildren } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { CompassIcon, LibraryIcon, SettingsIcon } from 'lucide-react'

import { FloatingNav, type FloatingNavItem } from '@/components/floating-nav'

const NAV_ITEMS: FloatingNavItem[] = [
  { id: 'library', label: '媒体库', icon: LibraryIcon, to: '/' },
  { id: 'discover', label: '发现', icon: CompassIcon, to: '/discover' },
  { id: 'settings', label: '设置', icon: SettingsIcon, to: '/settings' }
]

export function AppShell({ children }: PropsWithChildren) {
  const pathname = useRouterState({
    select: state => state.location.pathname
  })
  const activeId = NAV_ITEMS.find(item =>
    item.to === '/' ? pathname === '/' : pathname.startsWith(item.to)
  )?.id

  return (
    <div className="relative min-h-dvh bg-background">
      {children}
      <FloatingNav items={NAV_ITEMS} activeId={activeId} />
    </div>
  )
}
