import { useEffect, useState } from 'react'
import { ScanSearchIcon } from 'lucide-react'

import { sourceNames, useScrapingSources, type ScrapingSource } from '@/api/metadata'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { InlineError } from '@/components/error-state'
import { Switch } from '@/components/ui/switch'
import { StepActions } from './step-actions'

const descriptions: Record<string, string> = {
  fanza: '官方高清封面、预览图和影片资料',
  fc2: 'FC2 商品资料、封面和预览图',
  heyzo: 'HEYZO 官方封面和公开预览原图',
  pacopacomama: 'Pacopacomama 官方封面和公开预览原图'
}

export function ScrapingStep({
  initialSources,
  onNext,
  onBack
}: {
  initialSources: ScrapingSource[] | null
  onNext: (sources: ScrapingSource[]) => void
  onBack: () => void
}) {
  const query = useScrapingSources()
  const [sources, setSources] = useState<ScrapingSource[]>(initialSources ?? [])

  useEffect(() => {
    if (!initialSources && query.data) setSources(query.data)
  }, [query.data, initialSources])

  return (
    <div className="space-y-6">
      <Alert variant="warning">
        <ScanSearchIcon />
        <AlertTitle>刮削来源</AlertTitle>
        <AlertDescription>
          这些官方来源用于补充封面、预览图和影片资料，访问时需要可用的网络代理。可以稍后在设置中调整。
        </AlertDescription>
      </Alert>

      <div className="space-y-3">
        {query.isPending ? (
          <p className="text-sm text-muted-foreground">正在读取刮削来源…</p>
        ) : null}
        {sources.map(source => (
          <div
            key={source.id}
            className="flex items-center justify-between gap-4 rounded-2xl border p-3"
          >
            <div className="space-y-0.5">
              <div className="text-sm font-medium">{sourceNames[source.id] ?? source.id}</div>
              <div className="text-xs text-muted-foreground">
                {descriptions[source.id] ?? '官方资料补充'}
              </div>
            </div>
            <Switch
              checked={source.enabled}
              onCheckedChange={enabled =>
                setSources(items =>
                  items.map(item => (item.id === source.id ? { ...item, enabled } : item))
                )
              }
            />
          </div>
        ))}
      </div>

      {query.isError ? (
        <InlineError onRetry={() => void query.refetch()} retrying={query.isFetching}>
          刮削来源读取失败，请重试。
        </InlineError>
      ) : null}

      <StepActions onBack={onBack}>
        <Button
          type="button"
          disabled={query.isPending || query.isError}
          onClick={() => onNext(sources)}
        >
          继续
        </Button>
      </StepActions>
    </div>
  )
}
