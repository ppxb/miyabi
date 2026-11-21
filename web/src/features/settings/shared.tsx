import type { ReactNode } from 'react'

import { Card, CardContent } from '@/components/ui/card'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldTitle
} from '@/components/ui/field'
import { Skeleton } from '@/components/ui/skeleton'
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
  inline = false,
  descriptionLoading = false
}: {
  title: string
  description: ReactNode
  children: ReactNode
  inline?: boolean
  /** Show a skeleton instead of the description (it renders in a <p>, so a
   * block Skeleton can't be passed as the description itself). */
  descriptionLoading?: boolean
}) {
  return (
    <Field
      orientation={inline ? 'horizontal' : 'responsive'}
      className={cn('justify-between gap-3 @md/field-group:gap-6', inline && 'gap-4')}
    >
      <FieldContent>
        <FieldTitle>{title}</FieldTitle>
        {descriptionLoading ? (
          <Skeleton className="h-4 w-56 max-w-full" />
        ) : (
          <FieldDescription className="wrap-break-word">{description}</FieldDescription>
        )}
      </FieldContent>
      <div className="max-w-full shrink-0">{children}</div>
    </Field>
  )
}

/** Right-aligned action row for a settings section's buttons. */
export function SettingsActions({ children }: { children: ReactNode }) {
  return <div className="flex items-center justify-end gap-2">{children}</div>
}
