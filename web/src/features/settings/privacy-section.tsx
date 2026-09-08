import { ShieldIcon } from 'lucide-react'

import { Switch } from '@/components/ui/switch'
import { useSettingsStore } from '@/stores/settings'
import { SettingRow, SettingsSection } from './shared'

export function PrivacySection() {
  const nsfwMode = useSettingsStore(state => state.nsfwMode)
  const setNsfwMode = useSettingsStore(state => state.setNsfwMode)

  return (
    <SettingsSection icon={<ShieldIcon className="size-4" />} title="NSFW 保护">
      <SettingRow
        title="开启隐私保护模式"
        description="开启后隐藏列表页、搜索页、详情页敏感图片。"
        inline
      >
        <Switch checked={nsfwMode} onCheckedChange={setNsfwMode} />
      </SettingRow>
    </SettingsSection>
  )
}
