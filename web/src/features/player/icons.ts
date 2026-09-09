import type { DefaultLayoutIcons } from '@vidstack/react/player/layouts/default'
import {
  AArrowDownIcon,
  AArrowUpIcon,
  AccessibilityIcon,
  AirplayIcon,
  AudioLinesIcon,
  CaptionsIcon,
  CaptionsOffIcon,
  CastIcon,
  CheckIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  DownloadIcon,
  FastForwardIcon,
  ListVideoIcon,
  MaximizeIcon,
  MinimizeIcon,
  MonitorIcon,
  PauseIcon,
  PictureInPicture2Icon,
  PictureInPictureIcon,
  PlayIcon,
  RewindIcon,
  RotateCcwIcon,
  SettingsIcon,
  SunDimIcon,
  SunIcon,
  Volume1Icon,
  Volume2Icon,
  VolumeXIcon
} from 'lucide-react'

import { withPlayerTooltip } from './tooltip'

export const playerIcons = {
  AirPlayButton: { Default: withPlayerTooltip(AirplayIcon, 'AirPlay') },
  GoogleCastButton: { Default: withPlayerTooltip(CastIcon, 'Google Cast') },
  PlayButton: {
    Play: withPlayerTooltip(PlayIcon, 'Play'),
    Pause: withPlayerTooltip(PauseIcon, 'Pause'),
    Replay: withPlayerTooltip(RotateCcwIcon, 'Replay')
  },
  MuteButton: {
    Mute: withPlayerTooltip(VolumeXIcon, 'Unmute'),
    VolumeLow: withPlayerTooltip(Volume1Icon, 'Mute'),
    VolumeHigh: withPlayerTooltip(Volume2Icon, 'Mute')
  },
  CaptionButton: {
    On: withPlayerTooltip(CaptionsIcon, 'Closed-Captions Off'),
    Off: withPlayerTooltip(CaptionsOffIcon, 'Closed-Captions On')
  },
  PIPButton: {
    Enter: withPlayerTooltip(PictureInPicture2Icon, 'Enter PiP'),
    Exit: withPlayerTooltip(PictureInPictureIcon, 'Exit PiP')
  },
  FullscreenButton: {
    Enter: withPlayerTooltip(MaximizeIcon, 'Enter Fullscreen'),
    Exit: withPlayerTooltip(MinimizeIcon, 'Exit Fullscreen')
  },
  SeekButton: {
    Backward: withPlayerTooltip(RewindIcon, 'Seek Backward'),
    Forward: withPlayerTooltip(FastForwardIcon, 'Seek Forward')
  },
  DownloadButton: { Default: withPlayerTooltip(DownloadIcon, 'Download') },
  Menu: {
    Accessibility: AccessibilityIcon,
    ArrowLeft: ChevronLeftIcon,
    ArrowRight: ChevronRightIcon,
    Audio: AudioLinesIcon,
    AudioBoostUp: Volume2Icon,
    AudioBoostDown: Volume1Icon,
    Chapters: withPlayerTooltip(ListVideoIcon, 'Chapters'),
    Captions: CaptionsIcon,
    Playback: PlayIcon,
    Settings: withPlayerTooltip(SettingsIcon, 'Settings'),
    SpeedUp: FastForwardIcon,
    SpeedDown: RewindIcon,
    QualityUp: MonitorIcon,
    QualityDown: MonitorIcon,
    FontSizeUp: AArrowUpIcon,
    FontSizeDown: AArrowDownIcon,
    OpacityUp: SunIcon,
    OpacityDown: SunDimIcon,
    RadioCheck: CheckIcon
  },
  KeyboardDisplay: {
    Play: PlayIcon,
    Pause: PauseIcon,
    Mute: VolumeXIcon,
    VolumeUp: Volume2Icon,
    VolumeDown: Volume1Icon,
    EnterFullscreen: MaximizeIcon,
    ExitFullscreen: MinimizeIcon,
    EnterPiP: PictureInPicture2Icon,
    ExitPiP: PictureInPictureIcon,
    CaptionsOn: CaptionsIcon,
    CaptionsOff: CaptionsOffIcon,
    SeekForward: FastForwardIcon,
    SeekBackward: RewindIcon
  }
} satisfies DefaultLayoutIcons
