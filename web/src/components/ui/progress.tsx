import * as React from 'react'
import { cn } from 'cn'
import { Progress as ProgressPrimitive } from 'radix-ui'

function Progress({
  className,
  value,
  max = 100,
  variant = 'linear',
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root> & {
  variant?: 'linear' | 'circular'
}) {
  const percent = value == null ? null : (value / max) * 100

  if (variant === 'circular') {
    return (
      <ProgressPrimitive.Root
        data-slot="progress"
        value={value}
        max={max}
        className={cn('size-6 shrink-0 text-primary', className)}
        {...props}
      >
        <svg
          viewBox="0 0 24 24"
          aria-hidden="true"
          className={cn(
            'size-full -rotate-90',
            percent === null && 'animate-spin motion-reduce:animate-none'
          )}
        >
          <circle cx="12" cy="12" r="9" fill="none" strokeWidth="2.5" className="stroke-muted" />
          <ProgressPrimitive.Indicator asChild>
            <circle
              data-slot="progress-indicator"
              cx="12"
              cy="12"
              r="9"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.5"
              strokeLinecap="round"
              pathLength="100"
              strokeDasharray="100"
              strokeDashoffset={percent === null ? 75 : 100 - percent}
              className="transition-[stroke-dashoffset]"
            />
          </ProgressPrimitive.Indicator>
        </svg>
      </ProgressPrimitive.Root>
    )
  }

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
            'w-1/3 flex-none animate-[progress-indeterminate_1.5s_ease-in-out_infinite] motion-reduce:translate-x-full motion-reduce:animate-none'
        )}
        style={percent === null ? undefined : { transform: `translateX(-${100 - percent}%)` }}
      />
    </ProgressPrimitive.Root>
  )
}

export { Progress }
