import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

type AppPageProps = {
  children: ReactNode
  className?: string
  contentClassName?: string
}

export function AppPage({ children, className, contentClassName }: AppPageProps) {
  return (
    <main
      className={cn('relative flex min-h-dvh flex-col bg-background px-4 pt-6 pb-28', className)}
    >
      <div className={cn('mx-auto flex w-full max-w-6xl flex-1 flex-col gap-8', contentClassName)}>
        {children}
      </div>
    </main>
  )
}
