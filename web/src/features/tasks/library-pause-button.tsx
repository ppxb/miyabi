import { useSetLibraryPaused } from '@/api/tasks'
import { Button } from '@/components/ui/button'

export function LibraryPauseButton({ paused }: { paused: boolean }) {
  const control = useSetLibraryPaused()
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      disabled={control.isPending}
      onClick={() => control.mutate(!paused)}
    >
      {control.isPending ? '提交中…' : paused ? '继续' : '暂停'}
    </Button>
  )
}
