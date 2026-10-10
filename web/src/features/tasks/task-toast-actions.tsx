import { SquareIcon, LoaderCircleIcon, XIcon } from 'lucide-react'
import { toast } from 'sonner'

import { useRetryTask } from '@/api/tasks'
import { useOfflineControl } from '@/api/offline'
import { Button } from '@/components/ui/button'

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
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          title="取消任务"
          disabled={cancel.isPending}
          onClick={() => cancel.mutate(cancelTaskID)}
        >
          {cancel.isPending ? <LoaderCircleIcon className="animate-spin" /> : <SquareIcon />}
        </Button>
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
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        title="关闭通知"
        onClick={() => toast.dismiss(id)}
      >
        <XIcon />
      </Button>
    </div>
  )
}
