import { UserRoundIcon } from 'lucide-react'

import type { PanAccount } from '@/api/pan'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Progress } from '@/components/ui/progress'

export function PanAccountInfo({ account }: { account: PanAccount }) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      <Avatar key={account.avatar} className="size-12">
        {account.avatar ? <AvatarImage src={account.avatar} /> : null}
        <AvatarFallback>
          <UserRoundIcon className="size-5" />
        </AvatarFallback>
      </Avatar>
      <div className="min-w-0 space-y-1">
        <div className="truncate text-sm font-semibold">{account.name}</div>
        <div className="truncate text-xs text-muted-foreground">{account.level}</div>
      </div>
    </div>
  )
}

export function PanStorageUsage({ space }: { space: PanAccount['space'] }) {
  const percent = space.total.bytes > 0 ? (space.used.bytes / space.total.bytes) * 100 : 0
  const fill = Math.min(100, percent)

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <span className="text-sm font-medium">网盘容量</span>
        <span className="text-xs text-muted-foreground tabular-nums">
          已使用 {space.used.formatted} / {space.total.formatted}
        </span>
      </div>
      <div className="relative">
        <Progress
          value={fill}

          className="h-10 rounded-lg bg-muted/60 ring-1 ring-border/60 **:data-[slot=progress-indicator]:bg-sky-500"
        />
        <div
          className="@container pointer-events-none absolute inset-y-0 right-0 flex items-center justify-center overflow-hidden transition-[width]"
          style={{ width: `${100 - fill}%` }}
        >
          <span className="hidden px-2 text-sm font-medium whitespace-nowrap tabular-nums @min-[6rem]:block">
            {space.remaining.formatted}
          </span>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1.5">
          <span className="size-2.5 rounded-full bg-sky-500" />
          已使用
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="size-2.5 rounded-full border border-border bg-muted" />
          剩余 {space.remaining.formatted}
        </span>
      </div>
    </div>
  )
}
