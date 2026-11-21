import { useState, type CSSProperties, type PropsWithChildren } from 'react'
import { MonitorIcon, MoonIcon, SunIcon } from 'lucide-react'

import { AppSidebar } from '@/components/app-sidebar'
import { SearchDialog, SearchTrigger } from '@/components/app-search'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { SidebarInset, SidebarProvider, SidebarTrigger } from '@/components/ui/sidebar'
import { Toaster } from '@/components/ui/sonner'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { TaskEventsProvider } from '@/features/tasks/task-events'
import { TaskNotifications } from '@/features/tasks/task-notifications'
import { useTheme } from '@/stores/theme'

const THEME_CYCLE = ['light', 'dark', 'system'] as const

export function AppShell({ children }: PropsWithChildren) {
  const [searchOpen, setSearchOpen] = useState(false)

  return (
    <TaskEventsProvider>
      <SidebarProvider className="flex-row" style={{ '--sidebar-width': '272px' } as CSSProperties}>
        <div className="flex min-h-0 w-full flex-1">
          <AppSidebar variant="inset" collapsible="icon" />
          <SidebarInset className="peer-data-[variant=inset]:border">
            <AppHeader onOpenSearch={() => setSearchOpen(true)} />
            <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden p-4">
              {children}
            </div>
          </SidebarInset>
        </div>
      </SidebarProvider>

      <SearchDialog open={searchOpen} onOpenChange={setSearchOpen} />
      <Toaster position="top-right" closeButton duration={6000} />
      <TaskNotifications />
    </TaskEventsProvider>
  )
}

function AppHeader({ onOpenSearch }: { onOpenSearch: () => void }) {
  return (
    <header className="sticky top-0 z-50 flex h-12 shrink-0 items-center gap-2 overflow-hidden rounded-t-[inherit] border-b bg-background/50 px-4 backdrop-blur-md lg:px-6">
      <div className="flex shrink-0 items-center gap-1 lg:gap-2">
        <SidebarTrigger className="-ml-1" />
        <Separator
          orientation="vertical"
          className="mx-1 data-[orientation=vertical]:h-4 data-[orientation=vertical]:self-center lg:mx-2"
        />
        <SearchTrigger onOpen={onOpenSearch} />
      </div>

      <div className="flex min-w-0 flex-1 items-center" />

      <div className="flex shrink-0 items-center gap-3">
        <ThemeSwitcher />
      </div>
    </header>
  )
}

function ThemeSwitcher() {
  const { theme, setTheme } = useTheme()
  const nextTheme = THEME_CYCLE[(THEME_CYCLE.indexOf(theme) + 1) % THEME_CYCLE.length] ?? 'system'

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="outline"
            size="icon-sm"
            aria-label="切换主题"
            onClick={() => setTheme(nextTheme)}
          />
        }
      >
        {theme === 'system' ? <MonitorIcon /> : theme === 'dark' ? <SunIcon /> : <MoonIcon />}
      </TooltipTrigger>
      <TooltipContent side="bottom" sideOffset={8}>
        切换主题
      </TooltipContent>
    </Tooltip>
  )
}
