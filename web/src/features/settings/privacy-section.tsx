import { ShieldIcon } from 'lucide-react'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { type ImageSafety, useSettingsStore } from '@/stores/settings'
import { SettingRow, SettingsSection } from './shared'

export function PrivacySection() {
  const imageSafety = useSettingsStore(state => state.imageSafety)
  const setImageSafety = useSettingsStore(state => state.setImageSafety)

  return (
    <SettingsSection icon={<ShieldIcon className="size-4" />} title="NSFW 保护">
      <SettingRow
        title="图片安全模式"
        description="模糊是默认安全档：仍加载图片，点击或点按可临时显示，离开或失焦后恢复模糊。隐藏模式不加载封面、预览和演员图片，仅显示文字占位。"
        inline
      >
        <Select value={imageSafety} onValueChange={value => setImageSafety(value as ImageSafety)}>
          <SelectTrigger aria-label="图片安全模式">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="show">显示</SelectItem>
            <SelectItem value="blur">模糊（默认安全档）</SelectItem>
            <SelectItem value="hide">隐藏</SelectItem>
          </SelectContent>
        </Select>
      </SettingRow>
    </SettingsSection>
  )
}
