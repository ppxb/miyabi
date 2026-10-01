import { useSetLibraryPaused } from '@/api/tasks'
import { Button } from '@/components/ui/button'

export function LibraryPauseButton({ paused }: { paused: boolean }) {
  const control = useSetLibraryPaused()
  const label = paused ? '继续全部扫描和刮削' : '暂停全部扫描和刮削'
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      title={label}
      disabled={control.isPending}
      onClick={() => control.mutate(!paused)}
    >
      {control.isPending ? '提交中…' : paused ? '继续' : '暂停'}
    </Button>
  )
}
