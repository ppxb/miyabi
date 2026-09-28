const STORAGE_KEY = 'miyabi:viewed_movies'
const BATCH_SIZE = 50
const FLUSH_DELAY_MS = 10_000
const MAX_LOCAL_IDS = 5_000

export const VIEWED_MOVIES_PATH = '/api/discover/viewed'

export type BrowseHistoryOptions = {
  fetchViewed?: () => Promise<string[]>
  syncViewed?: (ids: string[]) => Promise<unknown>
  storageKey?: string
}

type PersistedState = {
  ids: string[]
  pending: string[]
}

const emptyState = (): PersistedState => ({ ids: [], pending: [] })

function loadPersistedState(key: string): PersistedState {
  if (typeof window === 'undefined' || !window.localStorage) return emptyState()
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return emptyState()
    const parsed = JSON.parse(raw) as Partial<PersistedState>
    return {
      ids: Array.isArray(parsed.ids) ? parsed.ids : [],
      pending: Array.isArray(parsed.pending) ? parsed.pending : []
    }
  } catch {
    return emptyState()
  }
}

function savePersistedState(key: string, state: PersistedState): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.setItem(key, JSON.stringify(state))
  } catch {
    // Ignore storage quota errors
  }
}

// BrowseHistoryStore remembers which JavDB movie IDs the user has opened.
// Views are kept locally first and batched to the server; JavDB IDs are the
// only identity stored, matching the catalogue identity rule in REFACTOR_PLAN.
export class BrowseHistoryStore {
  private viewedSet: Set<string>
  private pendingList: string[]
  private listeners = new Set<() => void>()
  private flushTimer: ReturnType<typeof setTimeout> | null = null
  private isFlushing = false
  private isInitialized = false
  private storageKey: string
  private fetchViewed?: () => Promise<string[]>
  private syncViewed?: (ids: string[]) => Promise<unknown>

  constructor(options: BrowseHistoryOptions = {}) {
    this.storageKey = options.storageKey ?? STORAGE_KEY
    this.fetchViewed = options.fetchViewed
    this.syncViewed = options.syncViewed

    const saved = loadPersistedState(this.storageKey)
    this.viewedSet = new Set(saved.ids)
    this.pendingList = [...saved.pending]

    if (typeof window !== 'undefined') {
      window.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') void this.flush()
      })
      window.addEventListener('pagehide', () => void this.flushKeepalive())
    }
  }

  init(): void {
    if (this.isInitialized || typeof window === 'undefined') return
    this.isInitialized = true
    void this.syncFromServer()
  }

  private async syncFromServer(): Promise<void> {
    if (!this.fetchViewed) return
    try {
      const serverIDs = await this.fetchViewed()
      let changed = false
      for (const id of serverIDs) {
        if (!this.viewedSet.has(id)) {
          this.viewedSet.add(id)
          changed = true
        }
      }
      if (changed) {
        this.persist()
        this.emit()
      }
    } catch {
      // Ignore network errors on initial sync; local cache remains active
    }
    if (this.pendingList.length > 0) void this.flush()
  }

  isViewed(id?: string): boolean {
    return !!id && this.viewedSet.has(id)
  }

  recordView(id: string): void {
    const cleanID = id.trim()
    if (!cleanID || this.viewedSet.has(cleanID)) return

    this.viewedSet.add(cleanID)
    this.pendingList.push(cleanID)
    this.persist()
    this.emit()

    if (this.pendingList.length >= BATCH_SIZE) {
      this.clearFlushTimer()
      void this.flush()
    } else {
      this.scheduleDebouncedFlush()
    }
  }

  private clearFlushTimer(): void {
    if (this.flushTimer) clearTimeout(this.flushTimer)
    this.flushTimer = null
  }

  private scheduleDebouncedFlush(): void {
    this.clearFlushTimer()
    const timer = setTimeout(() => {
      this.flushTimer = null
      void this.flush()
    }, FLUSH_DELAY_MS)
    this.flushTimer = timer
    // Node timers keep the event loop alive; unref them so unit tests can exit.
    if (typeof timer === 'object' && 'unref' in timer) {
      ;(timer as { unref: () => void }).unref()
    }
  }

  async flush(): Promise<void> {
    if (this.isFlushing || this.pendingList.length === 0 || !this.syncViewed) return
    this.clearFlushTimer()
    this.isFlushing = true
    const toSync = [...this.pendingList]
    try {
      await this.syncViewed(toSync)
      const synced = new Set(toSync)
      this.pendingList = this.pendingList.filter(item => !synced.has(item))
      this.persist()
    } catch {
      // Keep pending entries for the next retry
    } finally {
      this.isFlushing = false
    }
  }

  async flushKeepalive(): Promise<void> {
    if (this.isFlushing || this.pendingList.length === 0 || typeof window === 'undefined') return
    this.clearFlushTimer()
    this.isFlushing = true
    try {
      const ids = [...this.pendingList]
      const body = JSON.stringify({ ids })
      let sent = false
      if (typeof navigator !== 'undefined' && typeof navigator.sendBeacon === 'function') {
        sent = navigator.sendBeacon(
          VIEWED_MOVIES_PATH,
          new Blob([body], { type: 'application/json' })
        )
      }
      if (!sent) {
        const response = await fetch(VIEWED_MOVIES_PATH, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body,
          keepalive: true
        })
        sent = response.ok
      }
      if (sent) {
        const synced = new Set(ids)
        this.pendingList = this.pendingList.filter(item => !synced.has(item))
        this.persist()
      }
    } catch {
      // Keep pending entries when the fallback request fails.
    } finally {
      this.isFlushing = false
    }
  }

  private persist(): void {
    let ids = Array.from(this.viewedSet)
    if (ids.length > MAX_LOCAL_IDS) {
      ids = ids.slice(ids.length - MAX_LOCAL_IDS)
      this.viewedSet = new Set(ids)
      this.pendingList = this.pendingList.filter(id => this.viewedSet.has(id))
    }
    savePersistedState(this.storageKey, { ids, pending: this.pendingList })
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  private emit(): void {
    for (const listener of this.listeners) listener()
  }

  getPendingCount(): number {
    return this.pendingList.length
  }
}
