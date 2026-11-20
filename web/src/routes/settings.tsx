import { createFileRoute, Link, Outlet, useLocation } from '@tanstack/react-router'
import {
  BellIcon,
  DatabaseIcon,
  HardDriveIcon,
  NetworkIcon,
  Share2Icon,
  SlidersHorizontalIcon,
  TagsIcon
} from 'lucide-react'
import { cn } from 'cn'

import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'

const SETTINGS_NAV = [
  { to: '/settings', label: '常规', icon: SlidersHorizontalIcon },
  { to: '/settings/sources', label: '源管理', icon: HardDriveIcon },
  { to: '/settings/metadata', label: '元数据', icon: TagsIcon },
  { to: '/settings/data', label: '数据', icon: DatabaseIcon },
  { to: '/settings/outputs', label: '输出目标', icon: Share2Icon },
  { to: '/settings/network', label: '网络', icon: NetworkIcon },
  { to: '/settings/subscription', label: '订阅', icon: BellIcon }
] as const

export const Route = createFileRoute('/settings')({
  component: SettingsLayout
})

function SettingsLayout() {
  const pathname = useLocation({ select: location => location.pathname })

  return (
    <AppPage showBackTop={false}>
      <PageHeader title="设置" description="管理应用选项、存储源与元数据" />
      <div className="flex flex-col gap-6 md:flex-row md:gap-8">
        <nav
          aria-label="设置导航"
          className="no-scrollbar flex gap-1 overflow-x-auto md:w-44 md:shrink-0 md:flex-col"
        >
          {SETTINGS_NAV.map(item => {
            const active =
              item.to === '/settings'
                ? pathname === '/settings'
                : pathname === item.to || pathname.startsWith(`${item.to}/`)

            return (
              <Link
                key={item.to}
                to={item.to}
                className={cn(
                  'flex shrink-0 items-center gap-2 rounded-md px-3 py-2 text-sm whitespace-nowrap transition-colors',
                  active
                    ? 'bg-muted font-medium text-foreground'
                    : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
                )}
              >
                <item.icon className="size-4" />
                {item.label}
              </Link>
            )
          })}
        </nav>
        <div className="min-w-0 flex-1">
          <Outlet />
        </div>
      </div>
    </AppPage>
  )
}
