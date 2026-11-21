import type { ReactNode } from 'react'

import { Card, CardContent } from '@/components/ui/card'
import { Field, FieldContent, FieldDescription, FieldTitle } from '@/components/ui/field'
import { cn } from 'cn'

export function SettingsCard({ children }: { children: ReactNode }) {
  return (
    <Card>
      <CardContent className="@container/field-group space-y-8">{children}</CardContent>
    </Card>
  )
}

export function SettingsSection({
  icon,
  title,
  description,
  children
}: {
  icon: ReactNode
  title: string
  description?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="space-y-5">
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        <span className="text-muted-foreground">{icon}</span>
        {title}
      </h2>
      {description ? <p className="text-xs text-muted-foreground">{description}</p> : null}
      {children}
    </section>
  )
}

export function SettingRow({
  title,
  description,
  children,
  inline = false
}: {
  title: string
  description: ReactNode
  children: ReactNode
  inline?: boolean
}) {
  return (
    <Field
      orientation={inline ? 'horizontal' : 'responsive'}
      className={cn('justify-between gap-3 sm:gap-6', inline && 'gap-4')}
    >
      <FieldContent>
        <FieldTitle>{title}</FieldTitle>
        <FieldDescription className="text-xs leading-5 wrap-break-word">
          {description}
        </FieldDescription>
      </FieldContent>
      <div className="max-w-full shrink-0">{children}</div>
    </Field>
  )
}
