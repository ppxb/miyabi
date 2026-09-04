import type { PropsWithChildren } from 'react'
import { Compass, Library, Settings } from 'lucide-react'
import { Link } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'

const navigation = [
  { to: '/', label: '媒体库', icon: Library, exact: true },
  { to: '/discover', label: '发现', icon: Compass, exact: false },
  { to: '/settings', label: '设置', icon: Settings, exact: false }
] as const

export function AppShell({ children }: PropsWithChildren) {
  return (
    <div className="mx-auto flex min-h-screen w-full max-w-7xl flex-col px-4 sm:px-6">
      <header className="flex h-16 items-center justify-between border-b border-zinc-800/80">
        <Link to="/" className="text-base font-semibold tracking-wide text-zinc-100">
          Miyabi
        </Link>
        <nav className="flex items-center gap-1" aria-label="主导航">
          {navigation.map(({ to, label, icon: Icon, exact }) => (
            <Button key={to} asChild variant="ghost" size="sm">
              <Link
                to={to}
                activeOptions={{ exact }}
                activeProps={{ className: 'bg-zinc-800 text-zinc-50' }}
              >
                <Icon className="size-4" aria-hidden="true" />
                <span className="hidden sm:inline">{label}</span>
              </Link>
            </Button>
          ))}
        </nav>
      </header>
      <main className="flex flex-1 flex-col py-8">{children}</main>
    </div>
  )
}
