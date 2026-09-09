import * as React from 'react'
import { cn } from 'cn'
import { Progress as ProgressPrimitive } from 'radix-ui'

function Progress({
  className,
  value,
  max = 100,
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root>) {
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
          'size-full flex-1 bg-primary transition-transform',
          percent === null &&
            'w-1/3 flex-none animate-[progress-indeterminate_1.5s_ease-in-out_infinite]'
        )}
        style={percent === null ? undefined : { transform: `translateX(-${100 - percent}%)` }}
      />
    </ProgressPrimitive.Root>
  )
}

export { Progress }
