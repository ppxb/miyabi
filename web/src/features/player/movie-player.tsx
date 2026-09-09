import { useRef, useState, type ReactNode } from 'react'
import {
  isHLSProvider,
  MediaPlayer,
  MediaProvider,
  type MediaPlayerInstance
} from '@vidstack/react'
import { DefaultVideoLayout, defaultLayoutIcons } from '@vidstack/react/player/layouts/default'
import { ListVideoIcon } from 'lucide-react'

import type { LibraryFile } from '@/api/library'
import { usePlayback, usePlayFiles } from '@/api/play'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { formatSize } from '@/lib/format'
import { PlayerCloseButton, PlayerError, PlayerLoading, PlayerTitle } from './player-status'
import { playerTranslations } from './translations'

import '@vidstack/react/player/styles/default/theme.css'
import '@vidstack/react/player/styles/default/layouts/video.css'
import './player.css'

export default function MoviePlayer({ code }: { code: string }) {
  const files = usePlayFiles(code)

  if (files.isPending) return <PlayerLoading />
  if (files.isError) {
    return <PlayerError error={files.error} onRetry={() => void files.refetch()} />
  }
  if (files.data.files.length === 0) {
    return (
      <PlayerError
        title={files.data.title}
        message="没有可播放的文件，请重新扫描媒体库。"
        onRetry={() => void files.refetch()}
      />
    )
  }

  return <PlaybackPlayer title={files.data.title} files={files.data.files} />
}

function PlaybackPlayer({ title, files }: { title: string; files: LibraryFile[] }) {
  const [fileID, setFileID] = useState(files[0].id)
  const file = files.find(item => item.id === fileID) ?? files[0]
  const playback = usePlayback(file.id)
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

  const fileControl =
    files.length > 1 ? (
      <div className="miyabi-player-file shrink-0">
        <PlaybackSelect
          player={player}
          value={file.id}
          onValueChange={value => {
            position.current = 0
            resumeTime.current = 0
            setFailed(false)
            setAutoPlay(true)
            setSelectedSrc(undefined)
            setFileID(value)
          }}
          icon={<ListVideoIcon />}
          label="文件"
        >
          {files.map(item => (
            <SelectItem key={item.id} value={item.id}>
              <span className="truncate">
                {item.name} · {formatSize(item.size)}
              </span>
            </SelectItem>
          ))}
        </PlaybackSelect>
      </div>
    ) : null

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
      <MediaProvider />
      {loading || playback.isError || failed ? (
        <div className="absolute inset-0 z-20 cursor-auto">
          {loading ? (
            <PlayerLoading title={title} toolbar={fileControl} />
          ) : (
            <PlayerError
              title={title}
              error={playback.error ?? undefined}
              message="播放中断，请重新加载播放地址。"
              onRetry={retry}
              toolbar={fileControl}
            />
          )}
        </div>
      ) : (
        <DefaultVideoLayout
          icons={defaultLayoutIcons}
          translations={playerTranslations}
          colorScheme="dark"
          noModal
          slots={{
            topControlsGroupStart: <PlayerTitle title={title} />,
            topControlsGroupCenter: fileControl,
            topControlsGroupEnd: <PlayerCloseButton />,
            chapterTitle: <div className="vds-controls-spacer" />,
            beforeSettingsMenu:
              sources.length > 1 && source ? (
                <PlaybackSelect
                  player={player}
                  value={source.src}
                  side="top"
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
                </PlaybackSelect>
              ) : null
          }}
        />
      )}
    </MediaPlayer>
  )
}

function PlaybackSelect({
  player,
  value,
  onValueChange,
  side = 'bottom',
  icon,
  label,
  children
}: {
  player: MediaPlayerInstance | null
  value: string
  onValueChange: (value: string) => void
  side?: 'top' | 'bottom'
  icon?: ReactNode
  label?: string
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
      <SelectTrigger size="sm" className="miyabi-player-select max-w-full min-w-0">
        {icon}
        <SelectValue>{label}</SelectValue>
      </SelectTrigger>
      <SelectContent
        container={player?.el}
        position="popper"
        align="end"
        side={side}
        collisionBoundary={player?.el}
        collisionPadding={12}
        className="max-w-[min(32rem,var(--radix-select-content-available-width))] bg-neutral-900/90 text-white ring-white/10 backdrop-blur-xl"
      >
        {children}
      </SelectContent>
    </Select>
  )
}
