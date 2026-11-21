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
      size="lg"
      className="self-start gap-2"
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
