import { useState } from 'react'
import { CheckCircle2Icon, LoaderCircleIcon } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import { apiPut, describeApiError } from '@/api/client'
import { useStartLibraryScan } from '@/api/library'
import type { ScrapingSource } from '@/api/metadata'
import { panKeys, type PanDirectory } from '@/api/pan'
import {
  useCompleteSetup,
  useSetLocalDirectory,
  useSetupAccount,
  type SetupSource
} from '@/api/setup'
import { Button } from '@/components/ui/button'
import { StepActions } from './step-actions'

export function FinishStep({
  sources,
  accountConfigured,
  accountDraft,
  localDirectory,
  panDirectory,
  scrapingSources,
  onBack
}: {
  sources: SetupSource[]
  accountConfigured: boolean
  accountDraft: { username: string; password: string } | null
  localDirectory: string
  panDirectory?: PanDirectory
  scrapingSources: ScrapingSource[] | null
  onBack: () => void
}) {
  const queryClient = useQueryClient()
  const saveAccount = useSetupAccount()
  const saveLocal = useSetLocalDirectory()
  const complete = useCompleteSetup()
  const scan = useStartLibraryScan()
  const [error, setError] = useState('')

  const pending = saveAccount.isPending || saveLocal.isPending || complete.isPending

  async function handleFinish() {
    setError('')
    try {
      if (!accountConfigured) {
        if (!accountDraft) throw new Error('请先设置管理员账号')
        await saveAccount.mutateAsync(accountDraft)
      }
      if (sources.includes('local')) {
        if (!localDirectory) throw new Error('请填写本地媒体目录')
        await saveLocal.mutateAsync(localDirectory)
      }
      if (sources.includes('115') && panDirectory) {
        await apiPut<PanDirectory>('/api/pan/directory', { id: panDirectory.id })
        void queryClient.invalidateQueries({ queryKey: panKeys.account })
      }
      if (scrapingSources) {
        await apiPut<ScrapingSource[]>('/api/settings/scraping', scrapingSources)
        void queryClient.invalidateQueries({ queryKey: ['settings', 'scraping'] })
      }
      await complete.mutateAsync(sources)
      scan.mutate(false)
    } catch (err) {
      setError(describeApiError(err))
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col items-center gap-4">
        <CheckCircle2Icon className="size-12 text-success" />
        <h2 className="text-2xl font-semibold">准备就绪</h2>
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      <StepActions onBack={onBack}>
        <Button type="button" disabled={pending} onClick={() => void handleFinish()}>
          {pending ? <LoaderCircleIcon className="animate-spin" /> : null}
          完成设置
        </Button>
      </StepActions>
    </div>
  )
}
