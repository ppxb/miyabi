import { ScanSearchIcon, UserRoundIcon } from 'lucide-react'

import {
  actorSourceNames,
  sourceNames,
  useActorSources,
  useScrapingSources,
  useUpdateActorSources,
  useUpdateScrapingSources
} from '@/api/metadata'
import { SettingsSection } from './shared'
import { SourceToggleList } from './source-toggle-list'

const descriptions: Record<string, string> = {
  fanza: '官方封面、预览图和影片资料',
  fc2: '官方封面、预览图和影片资料',
  fc2db: 'FC2 备用数据库',
  heyzo: '官方封面、预览图和影片资料',
  pacopacomama: '官方封面、预览图和影片资料',
  javdb: '社区来源，补充影片资料与封面',
  javbus: '社区来源，补充影片资料与封面',
  jav321: '社区来源，补充影片资料与封面',
  tokyohot: '官方封面、预览图和影片资料',
  heydouga: '官方封面、预览图和影片资料',
  caribbeancom: '官方封面、预览图和影片资料',
  caribbeancompr: '官方封面、预览图和影片资料',
  c0930: '官方封面、预览图和影片资料',
  h0930: '官方封面、预览图和影片资料',
  h4610: '官方封面、预览图和影片资料',
  '10musume': '官方封面、预览图和影片资料',
  muramura: '官方封面、预览图和影片资料',
  dahlia: '官方封面、预览图和影片资料',
  duga: '官方封面、预览图和影片资料',
  r18dev: '官方镜像目录，补充官方封面、预览图和影片资料',
  onepondo: '官方封面、预览图和影片资料',
  codeav: '大型社区数据库，补充影片资料与图片',
  freejavbt: '备用来源，补充影片资料与磁力链接'
}

const actorDescriptions: Record<string, string> = {
  'av-league': '演员个性化数据来源',
  gfriends: '演员头像来源'
}

export function ScrapingSection() {
  const query = useScrapingSources()
  const update = useUpdateScrapingSources()
  return (
    <SettingsSection
      icon={<ScanSearchIcon className="size-4" />}
      title="影片刮削"
      description="以下来源补充图片及缺失资料（需要代理）。"
    >
      <SourceToggleList
        query={query}
        update={update}
        names={sourceNames}
        descriptions={descriptions}
        errorText="来源设置读取或保存失败，请重试。"
      />
    </SettingsSection>
  )
}

export function ActorSourcesSection() {
  const query = useActorSources()
  const update = useUpdateActorSources()
  return (
    <SettingsSection
      icon={<UserRoundIcon className="size-4" />}
      title="演员数据源"
      description="以下来源补充演员档案与图片（需要代理）。"
    >
      <SourceToggleList
        query={query}
        update={update}
        names={actorSourceNames}
        descriptions={actorDescriptions}
        errorText="演员来源设置读取或保存失败，请重试。"
      />
    </SettingsSection>
  )
}
