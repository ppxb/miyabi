import { SquareIcon, XIcon } from 'lucide-react'
import { toast } from 'sonner'

import { useRetryTask } from '@/api/tasks'
import { useOfflineControl } from '@/api/offline'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function TaskToastActions({
  id,
  retryTaskID,
  cancelTaskID
}: {
  id: string
  retryTaskID?: number
  cancelTaskID?: number
}) {
  const retry = useRetryTask()
  const cancel = useOfflineControl('cancel')

  return (
    <div className="ml-auto flex shrink-0 items-center gap-1">
      {cancelTaskID !== undefined && (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                disabled={cancel.isPending}
                onClick={() => cancel.mutate(cancelTaskID)}
              />
            }
          >
            {cancel.isPending ? <Spinner /> : <SquareIcon />}
          </TooltipTrigger>
          <TooltipContent>取消任务</TooltipContent>
        </Tooltip>
      )}
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
        <TooltipTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={() => toast.dismiss(id)}
            />
          }
        >
          <XIcon />
        </TooltipTrigger>
        <TooltipContent>关闭通知</TooltipContent>
      </Tooltip>
    </div>
  )
}
