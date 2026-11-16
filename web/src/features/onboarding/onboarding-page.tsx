import { useMemo, useState } from 'react'

import type { ScrapingSource } from '@/api/metadata'
import type { PanDirectory } from '@/api/pan'
import { useSetupStatus, type SetupSource } from '@/api/setup'
import { Card, CardContent } from '@/components/ui/card'
import { AccountStep } from './account-step'
import { FinishStep } from './finish-step'
import { LocalDirectoryStep } from './local-directory-step'
import { OnboardingStepper } from './onboarding-stepper'
import { PanStep } from './pan-step'
import { ScrapingStep } from './scraping-step'
import { SourceStep, type SourceMode } from './source-step'

const STEP_LABELS: Record<string, string> = {
  account: '管理员账号',
  source: '影片来源',
  local: '本地目录',
  pan: '115 登录',
  scraping: '刮削偏好',
  finish: '完成'
}

export function OnboardingPage() {
  const status = useSetupStatus()
  const [mode, setMode] = useState<SourceMode | ''>('')
  const [stepIndex, setStepIndex] = useState(0)
  const [accountDraft, setAccountDraft] = useState<{
    username: string
    password: string
  } | null>(null)
  const [localDirectory, setLocalDirectory] = useState(status.data?.local_directory ?? '')
  const [panDirectory, setPanDirectory] = useState<PanDirectory>()
  const [scrapingSources, setScrapingSources] = useState<ScrapingSource[] | null>(null)

  const steps = useMemo(() => {
    const list = ['account', 'source']
    if (mode === 'local' || mode === 'both') list.push('local')
    if (mode === '115' || mode === 'both') list.push('pan')
    list.push('scraping', 'finish')
    return list
  }, [mode])

  const safeIndex = Math.min(stepIndex, steps.length - 1)
  const current = steps[safeIndex]
  const sources: SetupSource[] = mode === 'both' ? ['local', '115'] : mode ? [mode] : []

  const next = () => setStepIndex(Math.min(safeIndex + 1, steps.length - 1))
  const back = () => setStepIndex(Math.max(safeIndex - 1, 0))

  let content = null
  if (current === 'account') {
    content = (
      <AccountStep
        configured={status.data?.account_configured ?? false}
        currentUsername={status.data?.username ?? ''}
        draft={accountDraft}
        onNext={draft => {
          setAccountDraft(draft ?? null)
          next()
        }}
      />
    )
  } else if (current === 'source') {
    content = (
      <SourceStep
        mode={mode}
        onChange={value => {
          setMode(value)
          setStepIndex(1)
        }}
        onNext={next}
        onBack={back}
      />
    )
  } else if (current === 'local') {
    content = (
      <LocalDirectoryStep
        initialPath={localDirectory}
        onNext={path => {
          setLocalDirectory(path)
          next()
        }}
        onBack={back}
      />
    )
  } else if (current === 'pan') {
    content = (
      <PanStep
        directory={panDirectory}
        onDirectoryChange={setPanDirectory}
        onNext={next}
        onBack={back}
      />
    )
  } else if (current === 'scraping') {
    content = (
      <ScrapingStep
        initialSources={scrapingSources}
        onNext={sources => {
          setScrapingSources(sources)
          next()
        }}
        onBack={back}
      />
    )
  } else {
    content = (
      <FinishStep
        sources={sources}
        accountConfigured={status.data?.account_configured ?? false}
        accountDraft={accountDraft}
        localDirectory={localDirectory}
        panDirectory={panDirectory}
        scrapingSources={scrapingSources}
        onBack={back}
      />
    )
  }

  return (
    <main className="flex min-h-dvh justify-center overflow-y-auto bg-muted/40 px-4 py-8">
      <div className="my-auto w-full max-w-3xl">
        <Card>
          <CardContent className="space-y-8 p-5 sm:p-8">
            <OnboardingStepper
              steps={steps.map(step => STEP_LABELS[step] ?? step)}
              current={safeIndex}
            />
            {safeIndex === 0 ? (
              <div className="space-y-1">
                <h1 className="text-lg font-semibold">欢迎使用 Miyabi</h1>
                <p className="text-sm text-muted-foreground">
                  先设置管理员账号和影片来源，完成后就可以开始使用。
                </p>
              </div>
            ) : null}
            <div>{content}</div>
          </CardContent>
        </Card>
      </div>
    </main>
  )
}
