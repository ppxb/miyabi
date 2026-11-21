import type { ReactNode } from 'react'

import { BackTopButton } from '@/components/back-top-button'
import { cn } from 'cn'

type AppPageProps = {
  children: ReactNode
  className?: string
  showBackTop?: boolean
}

export function AppPage({ children, className, showBackTop = true }: AppPageProps) {
  return (
    <div className={cn('relative flex min-h-0 flex-1 flex-col', className)}>
      <div className="flex w-full flex-1 flex-col gap-6">{children}</div>
      {showBackTop ? <BackTopButton /> : null}
    </div>
  )
}
