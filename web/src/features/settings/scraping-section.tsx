import { ScanSearchIcon, UserRoundIcon } from 'lucide-react'

import {
  actorSourceNames,
  sourceNames,
  useActorSources,
  useScrapingSources,
  useUpdateActorSources,
  useUpdateScrapingSources
} from '@/api/metadata'
import { InlineError } from '@/components/error-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { SettingRow, SettingsSection } from './shared'

const descriptions: Record<string, string> = {
  fanza: '优先提供官方高清封面、预览图，并补充影片资料',
  fc2: 'FC2 商品资料、封面和预览图',
  fc2db: 'FC2 备用数据库，补充演员、标签与贩售者',
  heyzo: 'HEYZO 官方封面、公开预览原图及影片资料',
  pacopacomama: 'Pacopacomama 官方封面、公开预览原图及影片资料',
  javdb: '社区目录来源，补充影片资料与封面',
  javbus: '社区目录来源，补充影片资料、演员与封面',
  jav321: '社区目录来源，番号直达详情，补充演职员、评分与预览图',
  tokyohot: 'Tokyo-Hot 官方资料，补充封面、预览图、演员与分类',
  heydouga: 'Hey動画官方资料，补充封面、演员、厂牌与日期（需代理）',
  caribbeancom: '加勒比官方资料，补充封面、预览图、演员与标签（EUC-JP，需代理）',
  caribbeancompr: '加勒比 Premium 官方资料，补充封面、预览图与演员（需代理）',
  c0930: 'C0930 人妻斬り官方资料，补充封面、演员、日期与标签（需代理）',
  h0930: 'H0930 官方资料，补充封面、演员、日期与标签（需代理）',
  h4610: 'H4610 官方资料，补充封面、演员、日期与标签（需代理）',
  '10musume': '天然むすめ 官方资料，补充封面、演员、标签与简介（需代理）',
  muramura: 'ムラムラ 官方资料，补充封面、演员、标签与简介（需代理）',
  dahlia: 'Dahlia 官方资料，补充封面、预览图、演员与日期（需代理）',
  duga: 'DUGA 商店目录，补充封面、预览图、演职员与标签（番号较通用，可按需关闭）',
  r18dev: '官方镜像目录，补充影片资料与封面',
  onepondo: '一本道官方资料与封面（仅日期式番号匹配时使用）',
  codeav: '大型社区数据库，番号直达详情，资料与 DMM 图片较完整',
  freejavbt: '备用来源，番号直达详情，补充影片资料与磁力链接（需代理）'
}

export function ScrapingSection() {
  const query = useScrapingSources()
  const update = useUpdateScrapingSources()
  // Show the submitted state while the save is in flight so the toggle reacts
  // immediately instead of waiting for the refetch.
  const sources = (update.isPending ? update.variables : query.data) ?? []
  return (
    <SettingsSection
      icon={<ScanSearchIcon className="size-4" />}
      title="影片刮削"
      description="以下来源补充图片及缺失资料。开启后对新刮削生效。注意需要开启代理。"
    >
      {query.isPending ? <Skeleton className="h-4 w-40" /> : null}
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

const actorDescriptions: Record<string, string> = {
  'av-league': '演员档案来源：生日、身高、三围、罩杯、血型、出身、出道日与图集（需代理）',
  gfriends: '演员图片图集来源（头像与多图墙）'
}

export function ActorSourcesSection() {
  const query = useActorSources()
  const update = useUpdateActorSources()
  const sources = (update.isPending ? update.variables : query.data) ?? []
  return (
    <SettingsSection
      icon={<UserRoundIcon className="size-4" />}
      title="演员数据源"
      description="以下来源用于补充演员档案与图片；关闭后对应数据不再抓取或展示。"
    >
      {query.isPending ? <Skeleton className="h-4 w-40" /> : null}
      {sources.map(source => (
        <SettingRow
          key={source.id}
          title={actorSourceNames[source.id] ?? source.id}
          description={actorDescriptions[source.id]}
          inline
        >
          <div className="flex items-center gap-2">
            <Switch
              checked={source.enabled}
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
          演员来源设置读取或保存失败，请重试。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}
