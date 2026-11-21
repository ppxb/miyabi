import { Progress as ProgressPrimitive } from '@base-ui/react/progress'
import { cn } from 'cn'

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
}: ProgressPrimitive.Root.Props & {
  variant?: keyof typeof indicatorColors
}) {
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
      <ProgressPrimitive.Track className="flex size-full items-center overflow-x-hidden">
        <ProgressPrimitive.Indicator
          data-slot="progress-indicator"
          className={cn(
            'h-full transition-all',
            indicatorColors[variant],
            value == null &&
              'w-1/3 flex-none animate-[progress-indeterminate_1.5s_ease-in-out_infinite]'
          )}
        />
      </ProgressPrimitive.Track>
    </ProgressPrimitive.Root>
  )
}

export { Progress }
