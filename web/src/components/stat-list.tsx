import type { LucideIcon } from 'lucide-react'

import { Skeleton } from '@/components/ui/skeleton'
import { cn } from 'cn'

export type StatItem = { label: string; value: string; icon: LucideIcon }

/** Shared stat strip used by movie and actor detail headers. */
export function StatList({ stats, className }: { stats: StatItem[]; className?: string }) {
  if (stats.length === 0) return null
  return (
    <dl
      className={cn(
        'flex flex-wrap divide-x divide-border/60 rounded-2xl bg-muted py-1 ring-1 ring-border dark:bg-card/60 dark:ring-0',
        className
      )}
    >
      {stats.map(stat => (
        <div key={stat.label} className="flex min-w-0 flex-1 flex-col items-center gap-2 p-3 sm:p-4">
          <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <stat.icon className="size-4" />
            {stat.label}
          </dt>
          <dd className="text-sm font-semibold whitespace-nowrap tabular-nums sm:text-base">
            {stat.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}

/** Skeleton counterpart of {@link StatList}. */
export function StatListSkeleton({ count = 3 }: { count?: number }) {
  return (
    <dl className="flex flex-wrap divide-x divide-border/60 rounded-2xl bg-muted py-1 ring-1 ring-border dark:bg-card/60 dark:ring-0">
      {Array.from({ length: count }, (_, index) => (
        <div key={index} className="flex min-w-0 flex-1 flex-col items-center gap-2 p-3 sm:p-4">
          <Skeleton className="h-4 w-16 max-w-full bg-background/70 dark:bg-muted" />
          <Skeleton className="h-5 w-20 max-w-full bg-background/70 sm:h-6 dark:bg-muted" />
        </div>
      ))}
    </dl>
  )
}
