import type { PropsWithChildren } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { CompassIcon, FilmIcon, HistoryIcon, SearchIcon, SettingsIcon } from 'lucide-react'

import { FloatingNav, type FloatingNavItem } from '@/components/floating-nav'
import { Toaster } from '@/components/ui/sonner'
import { MoviePlaybackNavAction } from '@/features/movie-detail/playback-nav-action'
import { TaskEventsProvider } from '@/features/tasks/task-events'
import { TaskNotifications } from '@/features/tasks/task-notifications'
import { PlayerDialog } from '@/features/player/player-dialog'

const NAV_ITEMS: FloatingNavItem[] = [
  { id: 'library', label: '媒体库', icon: FilmIcon, to: '/' },
  { id: 'history', label: '观看历史', icon: HistoryIcon, to: '/history' },
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
        <FloatingNav items={NAV_ITEMS} activeId={activeId}>
          <MoviePlaybackNavAction />
        </FloatingNav>
        {children}
        <PlayerDialog />
        <Toaster position="top-right" closeButton duration={6000} />
        <TaskNotifications />
      </div>
    </TaskEventsProvider>
  )
}
