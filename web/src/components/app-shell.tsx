import type { PropsWithChildren } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { BellIcon, CompassIcon, FilmIcon, SearchIcon, SettingsIcon } from 'lucide-react'

import { FloatingNav, type FloatingNavItem } from '@/components/floating-nav'
import { Toaster } from '@/components/ui/sonner'
import { TaskEventsProvider } from '@/features/tasks/task-events'
import { TaskNotifications } from '@/features/tasks/task-notifications'

const NAV_ITEMS: FloatingNavItem[] = [
  { id: 'library', label: '媒体库', icon: FilmIcon, to: '/' },
  { id: 'subscriptions', label: '订阅', icon: BellIcon, to: '/subscriptions' },
  { id: 'discover', label: '发现', icon: CompassIcon, to: '/discover' },
  { id: 'search', label: '搜索', icon: SearchIcon, to: '/search' },
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
    <TaskEventsProvider>
      <div className="relative min-h-dvh">
        <FloatingNav items={NAV_ITEMS} activeId={activeId} />
        {children}
        <Toaster position="top-right" closeButton duration={6000} />
        <TaskNotifications />
      </div>
    </TaskEventsProvider>
  )
}
