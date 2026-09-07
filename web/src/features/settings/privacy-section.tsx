import { ShieldIcon } from 'lucide-react'
import { Switch } from 'radix-ui'

import { usePreferences, useSavePreferences } from '@/api/settings'
import { Button } from '@/components/ui/button'
import { SettingRow, SettingsSection } from './shared'

export function PrivacySection() {
  const preferences = usePreferences()
  const save = useSavePreferences()

  return (
    <SettingsSection icon={<ShieldIcon className="size-4" />} title="NSFW 保护">
      <SettingRow
        title="NSFW 模式"
        description="开启后隐藏所有图片，包括封面、预览图和推荐图。"
        controlID="nsfw-mode"
        inline
      >
        <Switch.Root
          id="nsfw-mode"
          aria-describedby="nsfw-mode-description"
          checked={preferences.data?.nsfw_mode ?? true}
          disabled={!preferences.data || save.isPending}
          onCheckedChange={enabled => save.mutate({ nsfw_mode: enabled })}
          className="peer inline-flex h-5 w-9 shrink-0 items-center rounded-full border border-transparent bg-input shadow-xs transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary"
        >
          <Switch.Thumb className="pointer-events-none block size-4 translate-x-0 rounded-full bg-background shadow-sm transition-transform data-[state=checked]:translate-x-4" />
        </Switch.Root>
      </SettingRow>
      {preferences.isError ? (
        <div className="flex items-center gap-3 text-sm text-destructive" role="alert">
          <span>{preferences.error.message}</span>
          <Button variant="outline" size="sm" onClick={() => preferences.refetch()}>
            重试
          </Button>
        </div>
      ) : null}
      {save.isError ? (
        <p role="alert" className="text-sm text-destructive">
          保存失败：{save.error.message}
        </p>
      ) : null}
    </SettingsSection>
  )
}
