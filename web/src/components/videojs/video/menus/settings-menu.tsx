import { TimerIcon } from 'lucide-react'
import type { ComponentProps } from 'react'

import { AudioTrackMenu } from '@/components/videojs/ui/audio-track-menu'
import { CaptionsSubmenu } from '@/components/videojs/ui/captions-submenu'
import { CustomQualityMenu, type QualityChoice } from '@/components/videojs/ui/custom-quality-menu'
import { PlaybackRateSubmenu } from '@/components/videojs/ui/playback-rate-submenu'
import { SettingsMenu } from '@/components/videojs/ui/settings-menu'

/** Choices of an app-defined settings submenu. */
export type ChoiceMenu = {
  options: QualityChoice[]
  value: string
  onValueChange: (value: string) => void
}

export type VideoSettingsMenuProps = Omit<
  NonNullable<ComponentProps<typeof SettingsMenu>>,
  'children'
> & {
  qualityOptions: QualityChoice[]
  qualityValue: string
  onQualityChange: (value: string) => void
  /** Shown only when the media has subtitles to shift. */
  subtitleOffsetMenu?: ChoiceMenu
}

export function VideoSettingsMenu({
  qualityOptions,
  qualityValue,
  onQualityChange,
  subtitleOffsetMenu,
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
      {subtitleOffsetMenu ? (
        <CustomQualityMenu title="字幕偏移" icon={TimerIcon} {...subtitleOffsetMenu} />
      ) : null}
    </SettingsMenu>
  )
}
