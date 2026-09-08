import { useRef, useState } from 'react'
import {
  isHLSProvider,
  MediaPlayer,
  MediaProvider,
  VideoProviderLoader,
  type Src
} from '@vidstack/react'
import { DefaultVideoLayout, defaultLayoutIcons } from '@vidstack/react/player/layouts/default'

import { usePlayback, usePlayFiles, type PlayMode, type PlaySource } from '@/api/play'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatSize } from '@/lib/format'
import { PlayerError, PlayerLoading } from './player-status'
import { playerTranslations } from './translations'

import '@vidstack/react/player/styles/default/theme.css'
import '@vidstack/react/player/styles/default/layouts/video.css'
import './player.css'

// Proxy paths have no file extension. Let the native video element inspect original files;
// browser codec support is decided when loading, rather than guessed from a container name.
class FileVideoLoader extends VideoProviderLoader {
  canPlay(source: Src) {
    return source.type === '?' && typeof source.src === 'string'
  }
}

type PlaybackPosition = {
  read: () => number
  update: (time: number) => void
}

export default function MoviePlayer({ code }: { code: string }) {
  const files = usePlayFiles(code)
  const [fileID, setFileID] = useState<string>()
  const [mode, setMode] = useState<PlayMode>('original')

  if (files.isPending) return <PlayerLoading />
  if (files.isError) {
    return <PlayerError error={files.error} onRetry={() => void files.refetch()} />
  }
  const file = files.data.files.find(item => item.id === fileID) ?? files.data.files[0]
  if (!file) {
    return (
      <PlayerError
        message="没有可播放的文件，请重新扫描媒体库。"
        onRetry={() => void files.refetch()}
      />
    )
  }

  return (
    <div className="min-w-0 space-y-3">
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        {files.data.files.length > 1 ? (
          <Select value={file.id} onValueChange={setFileID}>
            <SelectTrigger className="min-w-0 flex-1 basis-full sm:basis-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent position="popper" className="max-w-[calc(100vw-3rem)]">
              {files.data.files.map(item => (
                <SelectItem key={item.id} value={item.id}>
                  <span className="truncate">
                    {item.name} · {formatSize(item.size)}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <p className="min-w-0 flex-1 basis-full truncate text-sm sm:basis-0">{file.name}</p>
        )}
        <Tabs value={mode} onValueChange={value => setMode(value as PlayMode)}>
          <TabsList>
            <TabsTrigger value="original">原文件</TabsTrigger>
            <TabsTrigger value="hls">115 转码</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
      <FilePlayer key={file.id} fileID={file.id} mode={mode} />
    </div>
  )
}

function FilePlayer({ fileID, mode }: { fileID: string; mode: PlayMode }) {
  const playback = usePlayback(fileID, mode)
  const position = useRef(0)

  if (playback.isPending || playback.isFetching) return <PlayerLoading />
  if (playback.isError) {
    return <PlayerError error={playback.error} onRetry={() => void playback.refetch()} />
  }

  return (
    <PlaybackSources
      key={playback.data.id}
      sources={playback.data.sources}
      position={{
        read: () => position.current,
        update: time => {
          position.current = time
        }
      }}
      onRetry={() => void playback.refetch()}
    />
  )
}

function PlaybackSources({
  sources,
  position,
  onRetry
}: {
  sources: PlaySource[]
  position: PlaybackPosition
  onRetry: () => void
}) {
  const [index, setIndex] = useState(0)
  const source = sources[index]

  return (
    <div className="space-y-3">
      <Video key={source.src} source={source} position={position} onRetry={onRetry} />
      {sources.length > 1 ? (
        <div className="flex items-center justify-end gap-2">
          <span className="text-xs text-muted-foreground">清晰度</span>
          <Select value={String(index)} onValueChange={value => setIndex(Number(value))}>
            <SelectTrigger size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {sources.map((item, itemIndex) => (
                <SelectItem key={item.src} value={String(itemIndex)}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}
    </div>
  )
}

function Video({
  source,
  position,
  onRetry
}: {
  source: PlaySource
  position: PlaybackPosition
  onRetry: () => void
}) {
  const [failed, setFailed] = useState(false)
  const [startTime] = useState(position.read)

  if (failed) {
    return (
      <PlayerError
        message={
          source.type === 'video/object'
            ? '此文件无法直接播放，可切换 115 转码，或重新加载播放地址。'
            : '播放中断，请重新加载播放地址。'
        }
        onRetry={onRetry}
      />
    )
  }

  return (
    <MediaPlayer
      className="miyabi-player"
      src={source.type === 'video/object' ? source.src : source}
      viewType="video"
      streamType="on-demand"
      autoPlay
      playsInline
      currentTime={startTime}
      onTimeUpdate={detail => {
        position.update(detail.currentTime)
      }}
      onError={() => setFailed(true)}
      onProviderChange={provider => {
        if (isHLSProvider(provider)) provider.library = () => import('hls.js')
      }}
    >
      <MediaProvider loaders={[FileVideoLoader]} />
      <DefaultVideoLayout
        icons={defaultLayoutIcons}
        translations={playerTranslations}
        colorScheme="dark"
        noModal
      />
    </MediaPlayer>
  )
}
