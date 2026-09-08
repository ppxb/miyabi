import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

export function SettingsSection({
  icon,
  title,
  children
}: {
  icon: ReactNode
  title: string
  children: ReactNode
}) {
  return (
    <section className="space-y-5">
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        <span className="text-muted-foreground">{icon}</span>
        {title}
      </h2>
      {children}
    </section>
  )
}

export function SettingRow({
  title,
  description,
  children,
  controlID,
  inline = false
}: {
  title: string
  description: ReactNode
  children: ReactNode
  controlID?: string
  inline?: boolean
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-stretch justify-between gap-3 sm:flex-row sm:items-center sm:gap-6',
        inline && 'flex-row items-center gap-4'
      )}
    >
      <div className="min-w-0 space-y-1">
        <div id={controlID ? `${controlID}-title` : undefined} className="text-sm font-medium">
          {title}
        </div>
        <div
          id={controlID ? `${controlID}-description` : undefined}
          className="text-xs leading-5 break-all text-muted-foreground"
        >
          {description}
        </div>
      </div>
      <div className="max-w-full shrink-0">{children}</div>
    </div>
  )
}
