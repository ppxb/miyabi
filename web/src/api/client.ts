export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

type QueryValue = string | number | readonly string[] | undefined

export async function apiGet<T>(
  path: string,
  query?: Record<string, QueryValue>,
  signal?: AbortSignal
): Promise<T> {
  const url = new URL(path, window.location.origin)
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value === undefined || value === '') continue
    if (Array.isArray(value)) {
      for (const item of value) url.searchParams.append(key, item)
    } else {
      url.searchParams.set(key, String(value))
    }
  }
  return request<T>(url.pathname + url.search, { signal })
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  })
}

export function apiDelete<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}

export function apiPut<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  })
}

// JavDB CDN hosts are not reachable from every browser network, so images go through the backend.
export function imageURL(source: string) {
  if (source.startsWith('/api/library/artwork/')) return source
  // Invalidate the encoded image responses cached before the backend decoded them.
  return `/api/image?v=3&url=${encodeURIComponent(source)}`
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init)
  if (!response.ok) {
    let message = response.statusText || `请求失败（HTTP ${response.status}）`
    try {
      const payload: unknown = await response.json()
      if (
        payload !== null &&
        typeof payload === 'object' &&
        'error' in payload &&
        typeof payload.error === 'string' &&
        payload.error.trim()
      ) {
        message = payload.error.trim()
      }
    } catch (error) {
      if (init?.signal?.aborted || (error instanceof Error && error.name === 'AbortError')) {
        throw error
      }
    }
    throw new ApiError(message, response.status)
  }
  return response.json() as Promise<T>
}
