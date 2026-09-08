import { ShieldIcon } from 'lucide-react'

import { Switch } from '@/components/ui/switch'
import { useSettingsStore } from '@/stores/settings'
import { SettingRow, SettingsSection } from './shared'

export function PrivacySection() {
  const nsfwMode = useSettingsStore(state => state.nsfwMode)
  const setNsfwMode = useSettingsStore(state => state.setNsfwMode)

  return (
    <SettingsSection icon={<ShieldIcon className="size-4" />} title="NSFW 保护">
      <SettingRow title="封面隐私模式" description="开启后遮挡影片封面和预览图" inline>
        <Switch checked={nsfwMode} onCheckedChange={setNsfwMode} />
      </SettingRow>
    </SettingsSection>
  )
}
