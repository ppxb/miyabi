import { useRef, useState, type ReactNode } from 'react'
import {
  isHLSProvider,
  MediaPlayer,
  MediaProvider,
  useMediaState,
  type MediaPlayerInstance
} from '@vidstack/react'
import { DefaultVideoLayout } from '@vidstack/react/player/layouts/default'

import { usePlayback, usePlayFiles } from '@/api/play'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { PlayerControlsVisibility } from './controls-visibility'
import { playerIcons } from './icons'
import {
  PlayerCloseButton,
  PlayerError,
  PlayerLoading,
  PlayerLoadingIndicator,
  PlayerTitle
} from './player-status'
import { playerTranslations } from './translations'

import '@vidstack/react/player/styles/default/theme.css'
import '@vidstack/react/player/styles/default/layouts/video.css'
import './player.css'

export default function MoviePlayer({ movieID }: { movieID: number }) {
  const files = usePlayFiles(movieID)

  if (files.isPending) return <PlayerLoading />
  if (files.isError) {
    return <PlayerError error={files.error} onRetry={() => void files.refetch()} />
  }
  const file = files.data.files[0]
  if (!file) {
    return (
      <PlayerError
        title={files.data.title}
        message="没有可播放的文件，请重新扫描媒体库。"
        onRetry={() => void files.refetch()}
      />
    )
  }

  return <PlaybackPlayer key={file.id} title={files.data.title} fileID={file.id} />
}

function PlaybackPlayer({ title, fileID }: { title: string; fileID: string }) {
  const playback = usePlayback(fileID)
  const [selectedSrc, setSelectedSrc] = useState<string>()
  const [player, setPlayer] = useState<MediaPlayerInstance | null>(null)
  const [failed, setFailed] = useState(false)
  const [autoPlay, setAutoPlay] = useState(true)
  const position = useRef(0)
  const resumeTime = useRef<number | null>(0)
  const sources = playback.data?.sources ?? []
  const source = sources.find(item => item.src === selectedSrc) ?? sources[0]
  const loading = playback.isPending || playback.isFetching

  const retry = () => {
    resumeTime.current = position.current
    setFailed(false)
    setAutoPlay(true)
    void playback.refetch()
  }

  return (
    <MediaPlayer
      ref={setPlayer}
      className="miyabi-player dark"
      title={title}
      src={loading || playback.isError || failed ? undefined : source}
      viewType="video"
      streamType="on-demand"
      autoPlay={autoPlay}
      playsInline
      onCanPlay={() => {
        if (player && resumeTime.current !== null) {
          player.remoteControl.seek(resumeTime.current)
          resumeTime.current = null
        }
      }}
      onTimeUpdate={detail => {
        if (resumeTime.current === null) position.current = detail.currentTime
      }}
      onError={() => {
        resumeTime.current = position.current
        setFailed(true)
      }}
      onProviderChange={provider => {
        if (isHLSProvider(provider)) provider.library = () => import('hls.js')
      }}
    >
      <PlayerControlsVisibility />
      <MediaProvider />
      {loading || playback.isError || failed ? (
        <div className="absolute inset-0 z-20 cursor-auto">
          {loading ? (
            <PlayerLoading title={title} />
          ) : (
            <PlayerError
              title={title}
              error={playback.error ?? undefined}
              message="播放中断，请重新加载播放地址。"
              onRetry={retry}
            />
          )}
        </div>
      ) : (
        <PlayerReady title={title}>
          <DefaultVideoLayout
            icons={playerIcons}
            translations={playerTranslations}
            colorScheme="dark"
            noModal
            slots={{
              bufferingIndicator: null,
              googleCastButton: null,
              topControlsGroupStart: <PlayerTitle title={title} />,
              topControlsGroupEnd: <PlayerCloseButton />,
              chapterTitle: <div className="vds-controls-spacer" />,
              beforeSettingsMenu:
                sources.length > 1 && source ? (
                  <PlaybackQualitySelect
                    player={player}
                    value={source.src}
                    onValueChange={value => {
                      resumeTime.current = position.current
                      setAutoPlay(!player?.paused)
                      setSelectedSrc(value)
                    }}
                  >
                    {sources.map(item => (
                      <SelectItem key={item.src} value={item.src}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </PlaybackQualitySelect>
                ) : null
            }}
          >
            <PlayerBufferingIndicator />
          </DefaultVideoLayout>
        </PlayerReady>
      )}
    </MediaPlayer>
  )
}

function PlayerReady({ title, children }: { title: string; children: ReactNode }) {
  const canPlay = useMediaState('canPlay')

  if (!canPlay) {
    return (
      <div className="absolute inset-0 z-20 cursor-auto">
        <PlayerLoading title={title} />
      </div>
    )
  }

  return children
}

function PlayerBufferingIndicator() {
  const waiting = useMediaState('waiting')

  if (!waiting) return null

  return (
    <div className="pointer-events-none absolute inset-0 z-20 grid place-items-center">
      <div className="rounded-full bg-background/75 px-4 py-2.5 backdrop-blur-xl">
        <PlayerLoadingIndicator message="正在缓冲…" />
      </div>
    </div>
  )
}

function PlaybackQualitySelect({
  player,
  value,
  onValueChange,
  children
}: {
  player: MediaPlayerInstance | null
  value: string
  onValueChange: (value: string) => void
  children: ReactNode
}) {
  return (
    <Select
      value={value}
      onValueChange={onValueChange}
      onOpenChange={open => {
        if (open) player?.controls.pause()
        else player?.controls.resume()
      }}
    >
      <SelectTrigger size="sm" className="max-w-full min-w-0 shrink-0 px-2 text-xs">
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        container={player?.el}
        position="popper"
        align="end"
        side="top"
        collisionBoundary={player?.el}
        collisionPadding={12}
        className="max-w-[min(32rem,var(--radix-select-content-available-width))] bg-popover/90 backdrop-blur-xl"
      >
        {children}
      </SelectContent>
    </Select>
  )
}
