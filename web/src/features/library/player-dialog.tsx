import { useQueryClient } from '@tanstack/react-query'
import { HlsJsVideo } from '@videojs/react/media/hlsjs-video'
import { I18nProvider } from '@videojs/react/i18n'
import { usePlayer, Video, VideoPlayer } from '@videojs/react/video'
import { CalendarIcon, ClockIcon, StarIcon, type LucideIcon } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode, type RefObject } from 'react'

import { useLibraryMovie } from '@/api/library'
import type { LibraryMovieDetail } from '@/api/library'
import { historyKeys } from '@/api/history'
import { savePlaybackProgress, usePlayQualities, useStartMoviePlayback } from '@/api/play'
import type { PlaySource } from '@/api/play'
import { InlineError } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { VideoSkin } from '@/components/videojs/video/skin'

export function LibraryPlayerDialog({
  movieId,
  onClose
}: {
  movieId: number | null
  onClose: () => void
}) {
  const [open, setOpen] = useState(true)
  const surfaceRef = useRef<HTMLDivElement>(null)
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
      <DialogContent
        initialFocus={() => surfaceRef.current}
        className="flex h-[min(54rem,calc(100dvh-2rem))] w-[calc(100%-2rem)] max-w-[96rem] flex-col gap-0 overflow-hidden p-0 sm:max-w-[96rem] lg:flex-row"
      >
        <LibraryPlayer movieId={movieId} surfaceRef={surfaceRef} />
      </DialogContent>
    </Dialog>
  )
}

function LibraryPlayer({
  movieId,
  surfaceRef
}: {
  movieId: number
  surfaceRef: RefObject<HTMLDivElement | null>
}) {
  const detail = useLibraryMovie(movieId)
  const playback = useStartMoviePlayback(movieId)
  const [qualitiesRequested, setQualitiesRequested] = useState(false)
  const qualities = usePlayQualities(
    playback.data?.file_id,
    !!playback.data?.file_id && qualitiesRequested
  )
  const [qualitySource, setQualitySource] = useState<PlaySource>()
  const source = useMemo(() => {
    if (qualitySource) return qualitySource
    const sources = playback.data?.sources ?? []
    return sources.find(item => item.label.startsWith('原画')) ?? sources[0]
  }, [playback.data, qualitySource])
  const qualityOptions = useMemo(() => {
    const options = [{ value: 'direct', label: '原画' }]
    for (const item of qualities.data?.sources ?? []) {
      options.push({ value: item.src, label: item.label })
    }
    return options
  }, [qualities.data])
  const qualityValue = qualitySource?.src ?? 'direct'
  const handleQualityChange = (value: string) => {
    if (value === 'direct') {
      setQualitySource(undefined)
      return
    }
    const next = qualities.data?.sources.find(item => item.src === value)
    if (next) setQualitySource(next)
  }
  const error = playback.isError

  // The player mounts only after the playback source resolves; move focus onto
  // it so keyboard shortcuts work immediately instead of staying on the dialog.
  useEffect(() => {
    if (!source || error) return
    surfaceRef.current?.querySelector<HTMLElement>('.media-skin')?.focus({ preventScroll: true })
  }, [source, error, surfaceRef])

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
      <div
        ref={surfaceRef}
        tabIndex={-1}
        className="relative flex min-w-0 flex-1 items-center justify-center bg-black outline-none"
      >
        {source && !error ? (
          <I18nProvider locale="zh-CN">
            <VideoPlayer title={title}>
              <VideoSkin
                style={skinStyle}
                qualityOptions={qualityOptions}
                qualityValue={qualityValue}
                onQualityChange={handleQualityChange}
                onSettingsOpen={() => setQualitiesRequested(true)}
              >
                {hls ? (
                  <HlsJsVideo
                    source={{
                      src: source.src,
                      type: 'application/vnd.apple.mpegurl',
                      preferPlayback: 'mse'
                    }}
                    playsInline
                    autoPlay
                  />
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
          </I18nProvider>
        ) : error ? (
          <InlineError
            onRetry={() => void playback.refetch()}
            retrying={playback.isFetching}
            retryLabel="重试"
          >
            播放地址加载失败
          </InlineError>
        ) : (
          <div className="text-sm text-white/70">正在加载播放地址</div>
        )}
      </div>
      <aside className="min-w-0 shrink-0 overflow-y-auto border-t bg-background p-5 lg:w-[26rem] lg:border-t-0 lg:border-l">
        <MovieInfo movieId={movieId} show115={detail.data?.source === '115'} />
      </aside>
    </>
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
  // Bare store access does not subscribe, so the continuously changing
  // currentTime no longer re-renders this component on every frame. Only the
  // low-frequency paused/duration values are subscribed.
  const player = usePlayer()
  const paused = usePlayer(state => state.paused)
  const duration = usePlayer(state => state.duration)
  const queryClient = useQueryClient()
  const resumed = useRef(false)

  // Single source of truth for persisting progress; reads the current player
  // state at call time instead of a per-frame ref.
  const saveProgress = useCallback(() => {
    if (!fileID) return
    const { currentTime, duration } = player.state
    if (currentTime > 0 && duration > 0) {
      void savePlaybackProgress(movieId, fileID, currentTime, duration).catch(() => {})
    }
  }, [player, movieId, fileID])

  useEffect(() => {
    if (resumed.current || !fileID || !resume || resume.file_id !== fileID) return
    if (resume.position <= 0 || duration <= 0) return
    resumed.current = true
    void player.seek(Math.min(resume.position, Math.max(0, duration - 1))).catch(() => {})
  }, [duration, fileID, resume, player])

  useEffect(() => {
    if (!fileID) return
    const timer = setInterval(() => {
      if (!player.state.paused) saveProgress()
    }, 5000)
    return () => {
      clearInterval(timer)
      saveProgress()
      void queryClient.invalidateQueries({ queryKey: historyKeys.all })
    }
  }, [fileID, player, saveProgress, queryClient])

  useEffect(() => {
    if (!paused) return
    saveProgress()
  }, [paused, saveProgress])

  return null
}

function MovieInfo({ movieId, show115 = false }: { movieId: number; show115?: boolean }) {
  const detail = useLibraryMovie(movieId)
  const movie = detail.data

  if (detail.isError) {
    return (
      <InlineError
        onRetry={() => void detail.refetch()}
        retrying={detail.isFetching}
        retryLabel="重试"
      >
        影片信息加载失败
      </InlineError>
    )
  }

  if (!movie) {
    return <MovieInfoSkeleton />
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="frosted">{movie.code}</Badge>
          {show115 ? <Badge variant="frosted">115</Badge> : null}
          {movie.has_subtitle ? <Badge variant="frosted">中文</Badge> : null}
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
            {movie.actors.map((actor, index) => (
              <span
                key={`${actor.provider ?? ''}:${actor.id || actor.name}:${index}`}
                className="inline-flex items-center gap-2"
              >
                {index > 0 ? <span className="text-muted-foreground">/</span> : null}
                {actor.name}
              </span>
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

      {movie.plot ? (
        <InfoRow label="简介">
          <p className="whitespace-pre-line">{movie.plot}</p>
        </InfoRow>
      ) : null}
    </div>
  )
}

function MovieInfoSkeleton() {
  return (
    <div className="space-y-4" aria-hidden>
      <div className="space-y-2">
        <div className="flex gap-1.5">
          <Skeleton className="h-5 w-16" />
          <Skeleton className="h-5 w-10" />
        </div>
        <Skeleton className="h-6 w-3/4" />
        <Skeleton className="h-4 w-1/2" />
      </div>
      <Skeleton className="h-16 w-full" />
      <div className="space-y-2">
        <Skeleton className="h-3 w-12" />
        <Skeleton className="h-4 w-40" />
      </div>
      <div className="space-y-2">
        <Skeleton className="h-3 w-12" />
        <Skeleton className="h-4 w-32" />
      </div>
      <div className="space-y-2">
        <Skeleton className="h-3 w-12" />
        <Skeleton className="h-4 w-full" />
      </div>
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
