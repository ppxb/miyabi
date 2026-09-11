import { useEffect, useMemo } from 'react'
import { toast } from 'sonner'

import { ApiError } from '@/api/client'
import { saveWatchProgress, type WatchSession } from '@/api/watch-history'
import { createWatchProgressWriter } from './watch-progress'

export function useWatchProgress(session: WatchSession | undefined, fileID: string) {
  const historyID = session?.id
  const sessionID = session?.session_id
  const writer = useMemo(
    () =>
      historyID === undefined || sessionID === undefined
        ? undefined
        : createPlayerProgressWriter(historyID, sessionID, fileID),
    [historyID, sessionID, fileID]
  )

  useEffect(() => {
    if (!writer) return
    const flush = () => {
      void writer.flush(true)
    }
    const visibilityChanged = () => {
      if (document.visibilityState === 'hidden') flush()
    }
    window.addEventListener('pagehide', flush)
    window.addEventListener('online', flush)
    document.addEventListener('visibilitychange', visibilityChanged)
    return () => {
      window.removeEventListener('pagehide', flush)
      window.removeEventListener('online', flush)
      document.removeEventListener('visibilitychange', visibilityChanged)
      flush()
    }
  }, [writer])

  return writer
}

function createPlayerProgressWriter(historyID: number, sessionID: string, fileID: string) {
  let active = true
  let errorShown = false
  const toastID = `history:progress:${historyID}`
  return createWatchProgressWriter({
    sessionID,
    fileID,
    write: async (progress, keepalive) => {
      if (!active) return
      try {
        await saveWatchProgress(historyID, progress, keepalive)
      } catch (error) {
        // Cleared history and switched sources invalidate this playback session.
        if (error instanceof ApiError && error.status < 500) {
          active = false
          return
        }
        throw error
      }
    },
    onError: () => {
      if (errorShown) return
      errorShown = true
      toast.error('播放进度暂未同步', { id: toastID, description: '恢复连接后会重试保存。' })
    },
    onSaved: () => {
      if (!errorShown) return
      errorShown = false
      toast.dismiss(toastID)
    }
  })
}
