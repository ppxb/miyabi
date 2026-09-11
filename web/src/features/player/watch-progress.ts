import type { WatchProgress } from '@/api/watch-history'

type Position = Pick<WatchProgress, 'position' | 'duration'>
type WriteProgress = (progress: WatchProgress, keepalive: boolean) => Promise<unknown>

const SAVE_INTERVAL = 10_000

export function createWatchProgressWriter({
  sessionID,
  fileID,
  write,
  now = Date.now,
  onError,
  onSaved
}: {
  sessionID: string
  fileID: string
  write: WriteProgress
  now?: () => number
  onError?: (error: unknown) => void
  onSaved?: () => void
}) {
  let pending: Position | undefined
  let saved: Position | undefined
  let version = 0
  let savedVersion = 0
  let inFlight = 0
  let lastAttempt = now()
  let lastRequest:
    | { position: Position; keepalive: boolean; settled: boolean; promise: Promise<void> }
    | undefined

  function equal(left: Position | undefined, right: Position) {
    return left?.position === right.position && left.duration === right.duration
  }

  function flush(keepalive = false): Promise<void> {
    if (!pending || (inFlight === 0 && equal(saved, pending))) return Promise.resolve()
    if (!keepalive && inFlight > 0) return lastRequest?.promise ?? Promise.resolve()
    if (
      lastRequest &&
      !lastRequest.settled &&
      equal(lastRequest.position, pending) &&
      (!keepalive || lastRequest.keepalive)
    )
      return lastRequest.promise

    const position = pending
    const requestVersion = ++version
    lastAttempt = now()
    inFlight++
    const request = { position, keepalive, settled: false, promise: Promise.resolve() }
    let response: Promise<unknown>
    try {
      // Start keepalive requests inside pagehide, before the document is discarded.
      response = write(
        { ...position, session_id: sessionID, file_id: fileID, version: requestVersion },
        keepalive
      )
    } catch (error) {
      response = Promise.reject(error)
    }
    request.promise = response
      .then(() => {
        if (requestVersion >= savedVersion) {
          saved = position
          savedVersion = requestVersion
          onSaved?.()
        }
      })
      .catch(error => {
        if (requestVersion > savedVersion) onError?.(error)
      })
      .finally(() => {
        inFlight--
        request.settled = true
      })
    lastRequest = request
    return request.promise
  }

  function update(position: number, duration: number) {
    if (!Number.isFinite(position) || !Number.isFinite(duration) || duration <= 0) return
    pending = { position: Math.min(duration, Math.max(0, position)), duration }
    if (now() - lastAttempt >= SAVE_INTERVAL) void flush()
  }

  return { update, flush }
}
