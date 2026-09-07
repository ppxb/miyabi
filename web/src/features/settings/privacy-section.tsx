import { ShieldIcon } from 'lucide-react'
import { Switch } from 'radix-ui'

import { useSettingsStore } from '@/stores/settings'
import { SettingRow, SettingsSection } from './shared'

export function PrivacySection() {
  const nsfwMode = useSettingsStore(state => state.nsfwMode)
  const setNsfwMode = useSettingsStore(state => state.setNsfwMode)

  return (
    <SettingsSection icon={<ShieldIcon className="size-4" />} title="NSFW 保护">
      <SettingRow
        title="NSFW 模式"
        description="开启后隐藏所有图片，包括封面、预览图和推荐图。仅保存至当前浏览器。"
        controlID="nsfw-mode"
        inline
      >
        <Switch.Root
          id="nsfw-mode"
          aria-describedby="nsfw-mode-description"
          checked={nsfwMode}
          onCheckedChange={setNsfwMode}
          className="peer inline-flex h-5 w-9 shrink-0 items-center rounded-full border border-transparent bg-input shadow-xs transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary"
        >
          <Switch.Thumb className="pointer-events-none block size-4 translate-x-0 rounded-full bg-background shadow-sm transition-transform data-[state=checked]:translate-x-4" />
        </Switch.Root>
      </SettingRow>
    </SettingsSection>
  )
}
