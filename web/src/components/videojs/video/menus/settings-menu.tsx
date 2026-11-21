import type { ComponentProps } from 'react'

import { AudioTrackMenu } from '@/components/videojs/ui/audio-track-menu'
import { CaptionsSubmenu } from '@/components/videojs/ui/captions-submenu'
import { CustomQualityMenu, type QualityChoice } from '@/components/videojs/ui/custom-quality-menu'
import { PlaybackRateSubmenu } from '@/components/videojs/ui/playback-rate-submenu'
import { SettingsMenu } from '@/components/videojs/ui/settings-menu'

export type VideoSettingsMenuProps = Omit<
  NonNullable<ComponentProps<typeof SettingsMenu>>,
  'children'
> & {
  qualityOptions: QualityChoice[]
  qualityValue: string
  onQualityChange: (value: string) => void
}

export function VideoSettingsMenu({
  qualityOptions,
  qualityValue,
  onQualityChange,
  ...props
}: VideoSettingsMenuProps) {
  return (
    <SettingsMenu {...props}>
      <CustomQualityMenu
        options={qualityOptions}
        value={qualityValue}
        onValueChange={onQualityChange}
      />
      <AudioTrackMenu />
      <PlaybackRateSubmenu />
      <CaptionsSubmenu />
    </SettingsMenu>
  )
}
