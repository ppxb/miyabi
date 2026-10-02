import { useSetLibraryPaused } from '@/api/tasks'
import { Button } from '@/components/ui/button'

export function LibraryPauseButton({ paused }: { paused: boolean }) {
  const control = useSetLibraryPaused()
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={control.isPending}
      onClick={() => control.mutate(!paused)}
    >
      {control.isPending ? '提交中…' : paused ? '继续全部' : '暂停全部'}
    </Button>
  )
}
