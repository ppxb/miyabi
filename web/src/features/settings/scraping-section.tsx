import { ArrowDownIcon, ArrowUpIcon, ScanSearchIcon } from 'lucide-react'

import { sourceNames, useScrapingSources, useUpdateScrapingSources } from '@/api/metadata'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { SettingRow, SettingsSection } from './shared'

const descriptions: Record<string, string> = {
  avbase: '影片资料、演员信息和多站点原图汇总',
  mgstage: '影片资料、完整封面和预览原图',
  fc2: 'FC2 商品资料、封面和预览图',
  javbus: '影片资料、演员和图片补充'
}

export function ScrapingSection() {
  const query = useScrapingSources()
  const update = useUpdateScrapingSources()
  const sources = query.data ?? []
  function move(index: number, offset: number) {
    const next = [...sources]
    ;[next[index], next[index + offset]] = [next[index + offset]!, next[index]!]
    update.mutate(next)
  }
  return (
    <SettingsSection icon={<ScanSearchIcon className="size-4" />} title="影片刮削">
      <p className="text-xs text-muted-foreground">
        按顺序选取资料，其他来源补充缺失信息；封面按实际清晰度自动选择。保存后对新刮削生效。
      </p>
      {query.isPending ? <p className="text-sm text-muted-foreground">正在读取来源…</p> : null}
      {sources.map((source, index) => (
        <SettingRow
          key={source.id}
          title={sourceNames[source.id] ?? source.id}
          description={descriptions[source.id]}
          inline
        >
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              aria-label={`上移 ${sourceNames[source.id]}`}
              disabled={index === 0 || update.isPending}
              onClick={() => move(index, -1)}
            >
              <ArrowUpIcon className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label={`下移 ${sourceNames[source.id]}`}
              disabled={index === sources.length - 1 || update.isPending}
              onClick={() => move(index, 1)}
            >
              <ArrowDownIcon className="size-4" />
            </Button>
            <Switch
              aria-label={`启用 ${sourceNames[source.id]}`}
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
