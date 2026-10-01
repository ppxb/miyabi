import { XIcon } from 'lucide-react'
import { toast } from 'sonner'

import { useRetryTask } from '@/api/tasks'
import { Button } from '@/components/ui/button'
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
  const retry = useRetryTask()

  return (
    <div className="ml-auto flex shrink-0 items-center gap-1">
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
      <Button type="button" variant="ghost" size="icon-sm" onClick={() => toast.dismiss(id)}>
        <XIcon />
      </Button>
    </div>
  )
}
