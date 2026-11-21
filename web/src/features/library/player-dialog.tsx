import { useScrollFade } from '@animate-ui/react/primitives/effects/scroll-fade'
import { useQueryClient } from '@tanstack/react-query'
import { HlsJsVideo } from '@videojs/react/media/hlsjs-video'
import { I18nProvider } from '@videojs/react/i18n'
import { usePlayer, Video, VideoPlayer } from '@videojs/react/video'
import { CalendarIcon, ClockIcon, StarIcon, type LucideIcon } from 'lucide-react'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject
} from 'react'

import { libraryKeys, useLibraryMovie } from '@/api/library'
import type { LibraryMovieDetail } from '@/api/library'
import { historyKeys } from '@/api/history'
import { savePlaybackProgress, usePlayQualities, useStartMoviePlayback } from '@/api/play'
import type { PlaybackResume } from '@/api/play'
import { InlineError } from '@/components/error-state'
import { StatList } from '@/components/stat-list'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { VideoSkin } from '@/components/videojs/video/skin'

// The subset of the player store the parent needs to snapshot before a quality
// switch. Kept structural so the ref stays decoupled from the store's full type.
type PlayerControls = {
  state: { currentTime: number; duration: number; paused: boolean; playbackRate: number }
  seek: (time: number) => Promise<number>
  setPlaybackRate: (rate: number) => void
  play: () => Promise<void>
}

// A playhead snapshot to re-apply after the media element remounts on a switch.
type PendingSeek = { time: number; rate: number; paused: boolean }

export function LibraryPlayerDialog({
  movieId,
  onClose
}: {
  movieId: number
  onClose: () => void
}) {
  const [open, setOpen] = useState(true)
  const surfaceRef = useRef<HTMLDivElement>(null)
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
  const asideRef = useRef<HTMLElement>(null)
  useScrollFade(asideRef)
  const [qualitiesRequested, setQualitiesRequested] = useState(false)

  // Saved rendition is restored by deriving the effective label from the resume
  // payload; only a manual pick overrides it. No effect/state sync needed.
  const resume = playback.data?.resume
  const resumeApplies = !!resume && resume.file_id === playback.data?.file_id
  const savedLabel = resumeApplies ? resume?.settings?.quality || 'direct' : 'direct'
  const [selectedLabel, setSelectedLabel] = useState<string | null>(null)
  const effectiveLabel = selectedLabel ?? savedLabel
  const needsQualities = effectiveLabel !== 'direct'

  const qualities = usePlayQualities(
    playback.data?.file_id,
    !!playback.data?.file_id && (qualitiesRequested || needsQualities)
  )
  const playerRef = useRef<PlayerControls | null>(null)
  const pendingSeekRef = useRef<PendingSeek | null>(null)

  const qualityOptions = useMemo(() => {
    const options = [{ value: 'direct', label: '原画' }]
    for (const item of qualities.data?.sources ?? []) {
      options.push({ value: item.src, label: item.label })
    }
    return options
  }, [qualities.data])

  // Wait for the rendition list before mounting when a non-original rendition is
  // restored, so we start on it instead of the original then remounting.
  const waitingQuality = needsQualities && !qualities.data && !qualities.isError

  const matchedSource = needsQualities
    ? qualities.data?.sources.find(item => item.label === effectiveLabel)
    : undefined
  const originalSource =
    playback.data?.sources?.find(item => item.label.startsWith('原画')) ??
    playback.data?.sources?.[0]
  const source = matchedSource ?? originalSource
  const qualityValue = matchedSource?.src ?? 'direct'
  // Only persist the rendition once the list resolved it; otherwise omit it so
  // the backend keeps the previously saved rendition instead of overwriting it.
  const qualityForSave = needsQualities ? matchedSource?.label : 'direct'

  const handleQualityChange = (value: string) => {
    if (value === qualityValue) return
    // Snapshot the playhead before the media element remounts so the new
    // rendition resumes where the old one left off instead of restarting at 0.
    // Keep an existing snapshot when the new media has not produced a time yet.
    const current = playerRef.current
    if (current) {
      const time = current.state.currentTime
      if (Number.isFinite(time) && time > 0) {
        pendingSeekRef.current = {
          time,
          rate: current.state.playbackRate,
          paused: current.state.paused
        }
      } else if (!pendingSeekRef.current) {
        pendingSeekRef.current = { time: 0, rate: current.state.playbackRate, paused: current.state.paused }
      }
    }
    if (value === 'direct') {
      setSelectedLabel('direct')
      return
    }
    const next = qualities.data?.sources.find(item => item.src === value)
    if (next) setSelectedLabel(next.label)
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

  return (
    <>
      <DialogTitle className="sr-only">{title}</DialogTitle>
      <div
        ref={surfaceRef}
        tabIndex={-1}
        className="relative flex min-w-0 flex-1 items-center justify-center bg-black outline-none"
      >
        {source && !error && !waitingQuality ? (
          <I18nProvider locale="zh-CN">
            <VideoPlayer title={title}>
              <VideoSkin
                className="aspect-video w-full [font-family:var(--font-sans)] [--media-font-family:var(--font-sans)]"
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
                sourceKey={qualityValue}
                quality={qualityForSave}
                playerRef={playerRef}
                pendingSeekRef={pendingSeekRef}
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
      <aside
        ref={asideRef}
        className="min-w-0 shrink-0 overflow-y-auto border-t bg-background p-5 lg:w-[26rem] lg:border-t-0 lg:border-l"
      >
        <MovieInfo movieId={movieId} />
      </aside>
    </>
  )
}

function PlaybackSync({
  movieId,
  fileID,
  resume,
  sourceKey,
  quality,
  playerRef,
  pendingSeekRef
}: {
  movieId: number
  fileID?: string
  resume?: PlaybackResume
  sourceKey: string
  quality?: string
  playerRef: RefObject<PlayerControls | null>
  pendingSeekRef: RefObject<PendingSeek | null>
}) {
  // Bare store access does not subscribe, so the continuously changing
  // currentTime no longer re-renders this component on every frame. Only the
  // low-frequency paused/duration/rate values are subscribed.
  const player = usePlayer()
  const paused = usePlayer(state => state.paused)
  const duration = usePlayer(state => state.duration)
  const playbackRate = usePlayer(state => state.playbackRate)
  const queryClient = useQueryClient()
  const resumed = useRef(false)
  const appliedSeekKey = useRef<string | null>(null)
  // The source for which the media has been observed at duration 0, i.e. a
  // fresh load. A carry-over seek waits for this so it never runs against the
  // previous source's still-valid duration.
  const resetForSource = useRef<string | null>(null)
  const qualityRef = useRef(quality)

  // Expose the live player so the parent can snapshot the playhead before it
  // swaps rendition (the media element remounts and would otherwise reset to 0).
  useEffect(() => {
    playerRef.current = player
    return () => {
      playerRef.current = null
    }
  }, [player, playerRef])

  useEffect(() => {
    qualityRef.current = quality
  }, [quality])

  // Single source of truth for persisting progress and settings; reads the
  // current player state and quality at call time. Stable identity so the
  // heartbeat is not rebuilt on every quality change.
  const saveProgress = useCallback(() => {
    if (!fileID) return
    try {
      const { currentTime, duration, playbackRate } = player.state
      if (!(currentTime > 0) || !(duration > 0)) return
      const q = qualityRef.current
      void savePlaybackProgress(movieId, fileID, currentTime, duration, {
        rate: playbackRate,
        ...(q ? { quality: q } : {})
      }).catch(() => {})
    } catch {
      // The player can be torn down before this cleanup runs.
    }
  }, [player, movieId, fileID])

  // Restore the saved position and speed once the media is ready.
  useEffect(() => {
    if (resumed.current || !fileID || !resume || resume.file_id !== fileID) return
    if (!Number.isFinite(duration) || duration <= 0) return
    resumed.current = true
    const rate = resume.settings?.rate
    if (rate && rate > 0) player.setPlaybackRate(rate)
    if (resume.position > 0) {
      void player.seek(Math.min(resume.position, Math.max(0, duration - 1))).catch(() => {})
    }
  }, [duration, fileID, resume, player])

  // Mark each source that reaches a zero duration so a not-yet-reloaded new
  // source is never mistaken for a ready one.
  useEffect(() => {
    if (!Number.isFinite(duration) || duration <= 0) {
      resetForSource.current = sourceKey
    }
  }, [sourceKey, duration])

  // Re-apply the playhead snapshot after a rendition switch, only once the new
  // source has actually loaded (observed a duration reset, then a duration).
  useEffect(() => {
    const pending = pendingSeekRef.current
    if (!pending || appliedSeekKey.current === sourceKey) return
    if (resetForSource.current !== sourceKey) return
    if (!Number.isFinite(duration) || duration <= 0) return
    const target = Math.min(pending.time, Math.max(0, duration - 1))
    appliedSeekKey.current = sourceKey
    player
      .seek(target)
      .then(() => {
        if (pendingSeekRef.current === pending) pendingSeekRef.current = null
      })
      .catch(() => {
        // Allow a retry on the next duration change.
        appliedSeekKey.current = null
      })
    if (pending.rate > 0) player.setPlaybackRate(pending.rate)
    if (!pending.paused) void player.play().catch(() => {})
  }, [sourceKey, duration, player, pendingSeekRef])

  // Persist speed and quality changes promptly; the heartbeat also carries them.
  useEffect(() => {
    saveProgress()
  }, [playbackRate, quality, saveProgress])

  useEffect(() => {
    if (!fileID) return
    const timer = setInterval(() => {
      if (!player.state.paused) saveProgress()
    }, 5000)
    return () => {
      clearInterval(timer)
      saveProgress()
      void queryClient.invalidateQueries({ queryKey: historyKeys.all })
      // The finished badge on library cards is derived from saved progress.
      void queryClient.invalidateQueries({ queryKey: libraryKeys.all })
    }
  }, [fileID, player, saveProgress, queryClient])

  useEffect(() => {
    if (!paused) return
    saveProgress()
  }, [paused, saveProgress])

  return null
}

function MovieInfo({ movieId }: { movieId: number }) {
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
          {movie.source === '115' ? <Badge variant="frosted">115</Badge> : null}
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
  stats.push({
    label: '评分',
    value:
      movie.rating > 0
        ? movie.rating_max
          ? `${movie.rating.toFixed(1)} / ${movie.rating_max}`
          : movie.rating.toFixed(1)
        : '暂无评分',
    icon: StarIcon
  })

  return <StatList stats={stats} />
}

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="text-sm leading-6">{children}</div>
    </div>
  )
}
