import type { WatchSession } from '@/api/watch-history'

export function watchProgressPercent(position: number, duration: number) {
  if (!Number.isFinite(position) || !Number.isFinite(duration) || duration <= 0) return 0
  return Math.min(100, Math.max(0, (position / duration) * 100))
}

export function watchResumePosition(session: WatchSession | undefined, fileID: string) {
  if (
    !session ||
    session.file_id !== fileID ||
    !Number.isFinite(session.position) ||
    !Number.isFinite(session.duration) ||
    session.duration <= 0 ||
    session.position >= session.duration - 1
  ) {
    return 0
  }
  return Math.max(0, session.position)
}

export function formatWatchTime(seconds: number) {
  const total = Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const remainder = String(total % 60).padStart(2, '0')
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, '0')}:${remainder}`
    : `${minutes}:${remainder}`
}
