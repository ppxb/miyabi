import type { ReactNode } from 'react'

import { BackTopButton } from '@/components/back-top-button'
import { cn } from 'cn'

type AppPageProps = {
  children: ReactNode
  className?: string
  contentClassName?: string
  showBackTop?: boolean
}

export function AppPage({
  children,
  className,
  contentClassName,
  showBackTop = true
}: AppPageProps) {
  return (
    <div className={cn('relative flex min-h-0 flex-1 flex-col', className)}>
      <div className={cn('flex w-full flex-1 flex-col gap-6', contentClassName)}>
        {children}
      </div>
      {showBackTop ? <BackTopButton /> : null}
    </div>
  )
}
