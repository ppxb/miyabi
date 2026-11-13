import { cn } from 'cn'

export function OnboardingStepper({ steps, current }: { steps: string[]; current: number }) {
  return (
    <div className="flex items-center gap-2 overflow-x-auto">
      {steps.map((label, index) => {
        const done = index < current
        const active = index === current
        return (
          <div key={label} className="flex shrink-0 items-center gap-2">
            <span
              className={cn(
                'flex size-6 items-center justify-center rounded-full border text-xs font-medium tabular-nums',
                done && 'border-primary bg-primary text-primary-foreground',
                active && 'border-primary text-primary',
                !done && !active && 'border-border text-muted-foreground'
              )}
            >
              {index + 1}
            </span>
            <span
              className={cn(
                'text-xs whitespace-nowrap',
                active ? 'font-medium text-foreground' : 'text-muted-foreground'
              )}
            >
              {label}
            </span>
            {index < steps.length - 1 ? <span className="mx-1 h-px w-4 bg-border" /> : null}
          </div>
        )
      })}
    </div>
  )
}
