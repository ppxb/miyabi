import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

type EmptyStateProps = {
  emoji: string
  title: string
  description?: string
  actions?: ReactNode
  className?: string
}

export function EmptyState({ emoji, title, description, actions, className }: EmptyStateProps) {
  return (
    <div
      className={cn(
        'flex min-h-80 w-full flex-col items-center justify-center gap-3 text-center',
        className
      )}
    >
      <p className="text-6xl font-bold" aria-hidden="true">
        {emoji}
      </p>
      <p className="text-sm font-medium text-foreground">{title}</p>
      {description ? (
        <p className="max-w-md text-sm leading-6 text-muted-foreground">{description}</p>
      ) : null}
      {actions ? <div className="pointer-events-auto mt-1">{actions}</div> : null}
    </div>
  )
}
