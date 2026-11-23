import { MagnetIcon } from 'lucide-react'

import { magnetSourceNames, useMagnetSources, useUpdateMagnetSources } from '@/api/magnet-sources'
import { SettingsSection } from './shared'
import { SourceToggleList } from './source-toggle-list'

const descriptions: Record<string, string> = {
  sukebei: 'Nyaa Sukebei 种子站',
  projectjav: 'ProjectJav 种子站'
}

export function MagnetSourcesSection() {
  const query = useMagnetSources()
  const update = useUpdateMagnetSources()

  return (
    <SettingsSection
      icon={<MagnetIcon className="size-4" />}
      title="磁力链来源"
      description="以下来源补充影片磁力链接（需要代理）。"
    >
      <SourceToggleList
        query={query}
        update={update}
        names={magnetSourceNames}
        descriptions={descriptions}
        errorText="磁力来源设置读取或保存失败，请重试。"
      />
    </SettingsSection>
  )
}
