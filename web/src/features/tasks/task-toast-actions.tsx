import { XIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { useRetryTask } from '@/api/tasks'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { LibraryPauseButton } from './library-pause-button'

export function TaskToastActions({
  id,
  retryTaskID,
  libraryPaused
}: {
  id: string
  retryTaskID?: number
  libraryPaused?: boolean
}) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const retry = useRetryTask()

  return (
    <div ref={setContainer} className="ml-auto flex shrink-0 items-center gap-1">
      {libraryPaused !== undefined ? <LibraryPauseButton paused={libraryPaused} /> : null}
      {retryTaskID !== undefined && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={retry.isPending}
          onClick={() => retry.mutate(retryTaskID)}
        >
          {retry.isPending ? '提交中…' : '重试'}
        </Button>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="关闭通知"
            onClick={() => toast.dismiss(id)}
          >
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
