import type { ReactNode } from 'react'
import { RefreshCwIcon } from 'lucide-react'

import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import { cn } from 'cn'

type RetryProps = {
  onRetry?: () => void
  retrying?: boolean
  retryLabel?: string
}

function RetryAction({
  onRetry,
  retrying,
  retryLabel = '重试'
}: RetryProps & { onRetry: () => void }) {
  return (
    <Button type="button" variant="outline" size="sm" disabled={retrying} onClick={onRetry}>
      <RefreshCwIcon className={cn('size-4', retrying && 'animate-spin')} />
      {retryLabel}
    </Button>
  )
}

export function ErrorState({
  message,
  className,
  onRetry,
  ...retry
}: RetryProps & { message: string; className?: string }) {
  return (
    <EmptyState
      emoji="(･o･;)"
      title={message}
      className={className}
      actions={onRetry ? <RetryAction onRetry={onRetry} {...retry} /> : undefined}
    />
  )
}

export function InlineError({
  children,
  className,
  onRetry,
  ...retry
}: RetryProps & { children: ReactNode; className?: string }) {
  return (
    <div className={cn('flex min-w-0 flex-wrap items-center gap-3', className)}>
      <p className="min-w-0 text-sm text-destructive">{children}</p>
      {onRetry ? <RetryAction onRetry={onRetry} {...retry} /> : null}
    </div>
  )
}
