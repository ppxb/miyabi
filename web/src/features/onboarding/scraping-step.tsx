import { ScanSearchIcon } from 'lucide-react'

import { sourceNames, useScrapingSources, useUpdateScrapingSources } from '@/api/metadata'
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

export function ScrapingStep({ onNext, onBack }: { onNext: () => void; onBack: () => void }) {
  const query = useScrapingSources()
  const update = useUpdateScrapingSources()
  const sources = query.data ?? []

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
              disabled={update.isPending}
              onCheckedChange={enabled =>
                update.mutate(
                  sources.map(item => (item.id === source.id ? { ...item, enabled } : item))
                )
              }
            />
          </div>
        ))}
      </div>

      {query.isError || update.isError ? (
        <InlineError
          onRetry={() => {
            if (query.isError) void query.refetch()
            else if (update.variables) update.mutate(update.variables)
          }}
          retrying={query.isFetching || update.isPending}
        >
          刮削来源读取或保存失败，请重试。
        </InlineError>
      ) : null}

      <StepActions onBack={onBack}>
        <Button type="button" onClick={onNext}>
          继续
        </Button>
      </StepActions>
    </div>
  )
}
