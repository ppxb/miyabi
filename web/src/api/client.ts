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

export async function apiGet<T>(path: string, query?: Record<string, QueryValue>): Promise<T> {
  const url = new URL(path, window.location.origin)
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value === undefined || value === '') continue
    if (Array.isArray(value)) {
      for (const item of value) url.searchParams.append(key, item)
    } else {
      url.searchParams.set(key, String(value))
    }
  }
  return request<T>(url.pathname + url.search)
}

export function apiPost<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'POST' })
}

// JavDB CDN hosts are not reachable from every browser network, so images go through the backend.
export function imageURL(source: string) {
  return `/api/image?url=${encodeURIComponent(source)}`
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init)
  if (!response.ok) {
    const payload = (await response.json()) as { error: string }
    throw new ApiError(payload.error, response.status)
  }
  return response.json() as Promise<T>
}
