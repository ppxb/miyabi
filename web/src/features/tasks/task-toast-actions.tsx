import { XIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function TaskToastActions({ id }: { id: string }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)

  return (
    <div ref={setContainer} className="ml-auto flex shrink-0 items-center gap-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button type="button" variant="ghost" size="icon-sm" onClick={() => toast.dismiss(id)}>
            <XIcon />
          </Button>
        </TooltipTrigger>
        <TooltipContent
          container={container?.closest<HTMLElement>('[data-sonner-toaster]')}
          side="top"
        >
          关闭通知
        </TooltipContent>
      </Tooltip>
    </div>
  )
}
