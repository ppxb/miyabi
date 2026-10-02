export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly code?: string
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export function isPanUnauthorized(error: unknown): boolean {
  return error instanceof ApiError && error.code === 'PAN_UNAUTHORIZED'
}

export function describeApiError(error: unknown): string {
  if (isPanUnauthorized(error)) return '115 登录已失效，请前往设置重新登录。'
  return error instanceof ApiError ? error.message : '请检查后端服务和网络后重试。'
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

export function apiPatch<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  })
}

export function apiPut<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  })
}

export function notifyUnauthorized(): void {
  window.dispatchEvent(new CustomEvent('miyabi:unauthorized'))
}

// JavDB CDN hosts are not reachable from every browser network, so images go through the backend.
export function imageURL(source: string) {
  if (source.startsWith('/api/library/artwork/')) return source
  if (/^\/api\/library\/movies\/\d+\/previews\/\d+(?:\?v=\d+)?$/.test(source)) return source
  // Invalidate the encoded image responses cached before the backend decoded them.
  return `/api/image?v=3&url=${encodeURIComponent(source)}`
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: 'same-origin'
  })

  if (!response.ok) {
    let message = response.statusText || `请求失败（HTTP ${response.status}）`
    let code: string | undefined
    try {
      const payload: unknown = await response.json()
      if (payload !== null && typeof payload === 'object') {
        if ('error' in payload && typeof payload.error === 'string' && payload.error.trim()) {
          message = payload.error.trim()
        }
        if ('code' in payload && typeof payload.code === 'string' && payload.code.trim()) {
          code = payload.code.trim()
        }
      }
    } catch (error) {
      if (init?.signal?.aborted || (error instanceof Error && error.name === 'AbortError')) {
        throw error
      }
    }

    if (response.status === 401 && code === 'UNAUTHORIZED' && !path.startsWith('/api/auth/')) {
      notifyUnauthorized()
    }

    throw new ApiError(message, response.status, code)
  }
  return response.json() as Promise<T>
}
