import { ScanSearchIcon } from 'lucide-react'

import { sourceNames, useScrapingSources, useUpdateScrapingSources } from '@/api/metadata'
import { InlineError } from '@/components/error-state'
import { Switch } from '@/components/ui/switch'
import { SettingRow, SettingsSection } from './shared'

const descriptions: Record<string, string> = {
  fanza: '优先提供官方高清封面、预览图，并补充影片资料',
  fc2: 'FC2 商品资料、封面和预览图',
  heyzo: 'HEYZO 官方封面、公开预览原图及影片资料',
  pacopacomama: 'Pacopacomama 官方封面、公开预览原图及影片资料'
}

export function ScrapingSection() {
  const query = useScrapingSources()
  const update = useUpdateScrapingSources()
  const sources = query.data ?? []
  return (
    <SettingsSection icon={<ScanSearchIcon className="size-4" />} title="影片刮削">
      <p className="text-xs text-muted-foreground">
        以下来源补充图片及缺失资料。开启后对新刮削生效。注意需要开启代理。
      </p>
      {query.isPending ? <p className="text-sm text-muted-foreground">正在读取来源</p> : null}
      {sources.map(source => (
        <SettingRow
          key={source.id}
          title={sourceNames[source.id] ?? source.id}
          description={descriptions[source.id]}
          inline
        >
          <div className="flex items-center gap-2">
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
        </SettingRow>
      ))}
      {query.isError || update.isError ? (
        <InlineError
          onRetry={() => {
            if (query.isError) void query.refetch()
            else if (update.variables) update.mutate(update.variables)
          }}
          retrying={query.isFetching || update.isPending}
        >
          来源设置读取或保存失败，请重试。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}
