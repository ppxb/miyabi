import * as React from 'react'
import { cn } from '@/lib/utils'
import { Progress as ProgressPrimitive } from 'radix-ui'

const indicatorColors = {
  default: 'bg-primary',
  success: 'bg-success',
  info: 'bg-info'
}

function Progress({
  className,
  value,
  max = 100,
  variant = 'default',
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root> & {
  variant?: keyof typeof indicatorColors
}) {
  const percent = value == null ? null : (value / max) * 100

  return (
    <ProgressPrimitive.Root
      data-slot="progress"
      value={value}
      max={max}
      className={cn(
        'relative flex h-3 w-full items-center overflow-x-hidden rounded-4xl bg-muted',
        className
      )}
      {...props}
    >
      <ProgressPrimitive.Indicator
        data-slot="progress-indicator"
        className={cn(
          'size-full flex-1 transition-transform',
          indicatorColors[variant],
          percent === null &&
            'w-1/3 flex-none animate-[progress-indeterminate_1.5s_ease-in-out_infinite]'
        )}
        style={percent === null ? undefined : { transform: `translateX(-${100 - percent}%)` }}
      />
    </ProgressPrimitive.Root>
  )
}

export { Progress }
