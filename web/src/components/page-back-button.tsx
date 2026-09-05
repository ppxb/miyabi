import { useCanGoBack, useRouter } from '@tanstack/react-router'
import { ArrowLeftIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'

export function PageBackButton() {
  const router = useRouter()
  const canGoBack = useCanGoBack()

  return (
    <Button
      type="button"
      variant="ghost"
      className="h-11 gap-2 self-start px-3 text-base"
      onClick={() => {
        if (canGoBack) router.history.back()
        else void router.navigate({ to: '/discover', replace: true })
      }}
    >
      <ArrowLeftIcon className="size-5" />
      返回
    </Button>
  )
}
