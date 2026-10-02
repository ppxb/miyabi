import { XIcon } from 'lucide-react'
import { toast } from 'sonner'

import { useRetryTask } from '@/api/tasks'
import { Button } from '@/components/ui/button'

export function TaskToastActions({ id, retryTaskID }: { id: string; retryTaskID?: number }) {
  const retry = useRetryTask()

  return (
    <div className="ml-auto flex shrink-0 items-center gap-1">
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
      <Button type="button" variant="ghost" size="icon-sm" onClick={() => toast.dismiss(id)}>
        <XIcon />
      </Button>
    </div>
  )
}
