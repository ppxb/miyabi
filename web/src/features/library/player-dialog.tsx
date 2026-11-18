import { useQueryClient } from '@tanstack/react-query'
import { HlsVideo } from '@videojs/react/media/hls-video'
import { usePlayer, Video, VideoPlayer, VideoSkin } from '@videojs/react/video'
import { CalendarIcon, ClockIcon, StarIcon, type LucideIcon } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from 'react'

import { useLibraryMovie } from '@/api/library'
import type { LibraryMovieDetail } from '@/api/library'
import { historyKeys } from '@/api/history'
import { savePlaybackProgress, usePlayQualities, useStartMoviePlayback } from '@/api/play'
import type { PlaySource } from '@/api/play'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'

import '@videojs/react/video/skin.css'

export function LibraryPlayerDialog({
  movieId,
  onClose
}: {
  movieId: number | null
  onClose: () => void
}) {
  const [open, setOpen] = useState(true)
  if (movieId === null) return null
  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      disablePointerDismissal
      onOpenChangeComplete={next => {
        if (!next) onClose()
      }}
    >
      <DialogContent className="flex h-[min(54rem,calc(100dvh-2rem))] w-[calc(100%-2rem)] max-w-[96rem] flex-col gap-0 overflow-hidden p-0 sm:max-w-[96rem] lg:flex-row">
        <LibraryPlayer movieId={movieId} />
      </DialogContent>
    </Dialog>
  )
}

function LibraryPlayer({ movieId }: { movieId: number }) {
  const detail = useLibraryMovie(movieId)
  const playback = useStartMoviePlayback(movieId)
  const [qualitySource, setQualitySource] = useState<PlaySource>()
  const source = useMemo(() => {
    if (qualitySource) return qualitySource
    const sources = playback.data?.sources ?? []
    return sources.find(item => item.label.startsWith('原画')) ?? sources[0]
  }, [playback.data, qualitySource])
  const error = playback.isError

  const title = detail.data?.title || detail.data?.code || '播放'
  const hls = source?.type.includes('mpegurl') ?? false
  const skinStyle = {
    aspectRatio: '16 / 9',
    width: '100%',
    fontFamily: 'var(--font-sans)',
    '--media-font-family': 'var(--font-sans)'
  } as CSSProperties

  return (
    <>
      <DialogTitle className="sr-only">{title}</DialogTitle>
      <div className="flex min-w-0 flex-1 items-center justify-center bg-black">
        {source && !error ? (
          <VideoPlayer title={title}>
            <VideoSkin style={skinStyle}>
              {hls ? (
                <HlsVideo src={source.src} playsInline autoPlay />
              ) : (
                <Video src={source.src} playsInline autoPlay />
              )}
            </VideoSkin>
            <PlaybackSync
              movieId={movieId}
              fileID={playback.data?.file_id}
              resume={playback.data?.resume}
            />
          </VideoPlayer>
        ) : (
          <div className="text-sm text-white/70">
            {error ? '播放地址加载失败' : '正在加载播放地址…'}
          </div>
        )}
      </div>
      <aside className="min-w-0 shrink-0 overflow-y-auto border-t bg-background p-5 lg:w-[26rem] lg:border-t-0 lg:border-l">
        <QualityPicker
          fileID={playback.data?.file_id}
          selectedSrc={qualitySource?.src}
          onSelect={setQualitySource}
        />
        <MovieInfo movieId={movieId} show115={!!playback.data?.source} />
      </aside>
    </>
  )
}

function QualityPicker({
  fileID,
  selectedSrc,
  onSelect
}: {
  fileID?: string
  selectedSrc?: string
  onSelect: (source?: PlaySource) => void
}) {
  const [open, setOpen] = useState(false)
  const qualities = usePlayQualities(fileID, open)

  return (
    <div className="mb-4 space-y-2">
      <p className="text-xs text-muted-foreground">清晰度</p>
      <div className="flex flex-wrap gap-1.5">
        <QualityButton active={!selectedSrc} onClick={() => onSelect(undefined)}>
          原画（直链）
        </QualityButton>
        {qualities.data?.sources.map(item => (
          <QualityButton
            key={item.src}
            active={selectedSrc === item.src}
            onClick={() => onSelect(item)}
          >
            {item.label}
          </QualityButton>
        ))}
        {!open ? (
          <QualityButton onClick={() => setOpen(true)} disabled={!fileID}>
            加载其他清晰度
          </QualityButton>
        ) : null}
        {open && qualities.isPending ? (
          <span className="self-center text-xs text-muted-foreground">加载中…</span>
        ) : null}
        {qualities.isError ? (
          <span className="self-center text-xs text-destructive">清晰度加载失败</span>
        ) : null}
      </div>
    </div>
  )
}

function QualityButton({
  active = false,
  disabled = false,
  onClick,
  children
}: {
  active?: boolean
  disabled?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={
        active
          ? 'rounded-full bg-foreground px-2.5 py-1 text-xs font-semibold text-background'
          : 'rounded-full bg-muted px-2.5 py-1 text-xs text-foreground hover:bg-muted/70 disabled:opacity-50'
      }
    >
      {children}
    </button>
  )
}

function PlaybackSync({
  movieId,
  fileID,
  resume
}: {
  movieId: number
  fileID?: string
  resume?: { file_id: string; position: number; duration: number }
}) {
  const currentTime = usePlayer(state => state.currentTime)
  const duration = usePlayer(state => state.duration)
  const paused = usePlayer(state => state.paused)
  const seek = usePlayer(state => state.seek)
  const queryClient = useQueryClient()
  const resumed = useRef(false)
  const latest = useRef({ currentTime: 0, duration: 0, paused: true })

  useEffect(() => {
    latest.current = { currentTime, duration, paused }
  }, [currentTime, duration, paused])

  useEffect(() => {
    if (resumed.current || !fileID || !resume || resume.file_id !== fileID) return
    if (resume.position <= 0 || duration <= 0) return
    resumed.current = true
    try {
      void seek(Math.min(resume.position, Math.max(0, duration - 1)))
    } catch {
      // Actions reject until the media attaches; the duration guard covers it.
    }
  }, [duration, fileID, resume, seek])

  useEffect(() => {
    if (!fileID) return
    const save = () => {
      const state = latest.current
      if (state.currentTime > 0 && state.duration > 0) {
        void savePlaybackProgress(movieId, fileID, state.currentTime, state.duration).catch(
          () => {}
        )
      }
    }
    const timer = setInterval(() => {
      if (!latest.current.paused) save()
    }, 5000)
    return () => {
      clearInterval(timer)
      save()
      void queryClient.invalidateQueries({ queryKey: historyKeys.all })
    }
  }, [fileID, movieId, queryClient])

  useEffect(() => {
    if (!paused || !fileID) return
    const state = latest.current
    if (state.currentTime > 0 && state.duration > 0) {
      void savePlaybackProgress(movieId, fileID, state.currentTime, state.duration).catch(() => {})
    }
  }, [paused, fileID, movieId])

  return null
}

function MovieInfo({ movieId, show115 = false }: { movieId: number; show115?: boolean }) {
  const detail = useLibraryMovie(movieId)
  const movie = detail.data

  if (!movie) {
    return <p className="text-sm text-muted-foreground">正在加载影片信息…</p>
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="frosted">{movie.code}</Badge>
          {show115 ? <Badge variant="frosted">115</Badge> : null}
        </div>
        <h2 className="text-xl leading-snug font-bold">{movie.title || movie.code}</h2>
        {movie.origin_title && movie.origin_title !== movie.title ? (
          <p className="text-sm text-muted-foreground">{movie.origin_title}</p>
        ) : null}
      </div>

      <MovieStats movie={movie} />

      <InfoRow label="厂商">{movie.maker?.name || 'N/A'}</InfoRow>
      <InfoRow label="系列">{movie.series?.name || 'N/A'}</InfoRow>

      <InfoRow label="演员">
        {movie.actors.length > 0 ? (
          <span className="flex flex-wrap gap-x-2 gap-y-1">
            {movie.actors.map(actor => (
              <span key={`${actor.provider ?? ''}:${actor.id}`}>{actor.name}</span>
            ))}
          </span>
        ) : (
          <span className="text-muted-foreground">N/A</span>
        )}
      </InfoRow>

      <InfoRow label="标签">
        {movie.tags.length > 0 ? (
          <span className="flex flex-wrap gap-1.5">
            {movie.tags.map(tag => (
              <Badge key={`${tag.provider}:${tag.id}`} variant="outline">
                {tag.name}
              </Badge>
            ))}
          </span>
        ) : (
          <span className="text-muted-foreground">N/A</span>
        )}
      </InfoRow>
    </div>
  )
}

function MovieStats({ movie }: { movie: LibraryMovieDetail }) {
  const stats: Array<{ label: string; value: string; icon: LucideIcon }> = []
  if (movie.release_date) {
    stats.push({ label: '发行日期', value: movie.release_date, icon: CalendarIcon })
  }
  if (movie.duration > 0) {
    stats.push({ label: '片长', value: `${movie.duration} 分钟`, icon: ClockIcon })
  }
  if (movie.rating > 0) {
    stats.push({
      label: '评分',
      value: movie.rating_max
        ? `${movie.rating.toFixed(1)} / ${movie.rating_max}`
        : movie.rating.toFixed(1),
      icon: StarIcon
    })
  }
  if (stats.length === 0) return null

  return (
    <dl className="flex flex-wrap divide-x divide-border/60 rounded-2xl bg-muted py-1 ring-1 ring-border dark:bg-card/60 dark:ring-0">
      {stats.map(stat => (
        <div
          key={stat.label}
          className="flex min-w-0 flex-1 flex-col items-center gap-2 p-3 sm:p-4"
        >
          <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <stat.icon className="size-4" />
            {stat.label}
          </dt>
          <dd className="text-sm font-semibold whitespace-nowrap tabular-nums">{stat.value}</dd>
        </div>
      ))}
    </dl>
  )
}

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="text-sm leading-6">{children}</div>
    </div>
  )
}
