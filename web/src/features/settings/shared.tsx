import type { ReactNode } from 'react'

import { Card, CardContent } from '@/components/ui/card'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldTitle
} from '@/components/ui/field'
import { cn } from 'cn'

export function SettingsCard({ children }: { children: ReactNode }) {
  return (
    <Card>
      <CardContent>
        <FieldGroup className="gap-8">{children}</FieldGroup>
      </CardContent>
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
      className={cn('justify-between gap-3 @md/field-group:gap-6', inline && 'gap-4')}
    >
      <FieldContent>
        <FieldTitle>{title}</FieldTitle>
        <FieldDescription className="wrap-break-word">{description}</FieldDescription>
      </FieldContent>
      <div className="max-w-full shrink-0">{children}</div>
    </Field>
  )
}

/** Right-aligned action row for a settings section's buttons. */
export function SettingsActions({ children }: { children: ReactNode }) {
  return <div className="flex items-center justify-end gap-2">{children}</div>
}
