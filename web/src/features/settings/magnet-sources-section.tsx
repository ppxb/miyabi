import { MagnetIcon } from 'lucide-react'

import { magnetSourceNames, useMagnetSources, useUpdateMagnetSources } from '@/api/magnet-sources'
import { InlineError } from '@/components/error-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { SettingRow, SettingsSection } from './shared'

const descriptions: Record<string, string> = {
  sukebei: 'Nyaa Sukebei 种子站',
  projectjav: 'ProjectJav 种子站'
}

export function MagnetSourcesSection() {
  const query = useMagnetSources()
  const update = useUpdateMagnetSources()
  // Reflect the submitted state while saving so the switch reacts immediately.
  const sources = (update.isPending ? update.variables : query.data) ?? []

  return (
    <SettingsSection
      icon={<MagnetIcon className="size-4" />}
      title="磁力链来源"
      description="以下来源补充影片磁力链接（需要代理）。"
    >
      {query.isPending ? <Skeleton className="h-4 w-40" /> : null}
      {sources.map(source => (
        <SettingRow
          key={source.id}
          title={magnetSourceNames[source.id] ?? source.id}
          description={descriptions[source.id]}
          inline
        >
          <Switch
            checked={source.enabled}
            onCheckedChange={enabled =>
              update.mutate(
                sources.map(item => (item.id === source.id ? { ...item, enabled } : item))
              )
            }
          />
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
          磁力来源设置读取或保存失败，请重试。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}
