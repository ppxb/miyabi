import { type ReactNode } from 'react'

import { Button } from '@/components/ui/button'

export function StepActions({
  children,
  showBack = true,
  onBack
}: {
  children: ReactNode
  showBack?: boolean
  onBack: () => void
}) {
  return (
    <div className="flex items-center justify-between gap-3 pt-2">
      {showBack ? (
        <Button type="button" variant="ghost" onClick={onBack}>
          上一步
        </Button>
      ) : (
        <span />
      )}
      {children}
    </div>
  )
}
