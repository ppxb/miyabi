import { CheckCircle2Icon, LoaderCircleIcon } from 'lucide-react'

import { useStartLibraryScan } from '@/api/library'
import { useCompleteSetup, type SetupSource } from '@/api/setup'
import { Button } from '@/components/ui/button'
import { StepActions } from './step-actions'

export function FinishStep({ sources, onBack }: { sources: SetupSource[]; onBack: () => void }) {
  const complete = useCompleteSetup()
  const scan = useStartLibraryScan()

  function handleFinish() {
    complete.mutate(sources, {
      onSuccess: () => {
        // Kick off the first library scan in the background; failure is not
        // fatal because the user can start it again from the library page.
        scan.mutate(false)
      }
    })
  }

  const labels: Record<SetupSource, string> = {
    local: '本地媒体',
    '115': '115 网盘'
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col items-center gap-4 rounded-2xl border bg-muted/30 p-8 text-center">
        <CheckCircle2Icon className="size-10 text-success" />
        <div className="space-y-1">
          <h2 className="text-base font-semibold">准备就绪</h2>
          <p className="text-sm text-muted-foreground">
            已选择：{sources.map(source => labels[source]).join('、')}
          </p>
        </div>
      </div>
      {complete.error ? <p className="text-sm text-destructive">{complete.error.message}</p> : null}
      <StepActions onBack={onBack}>
        <Button type="button" disabled={complete.isPending} onClick={handleFinish}>
          {complete.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
          完成设置
        </Button>
      </StepActions>
    </div>
  )
}
