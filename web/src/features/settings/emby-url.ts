/**
 * Splits an Emby server URL into separate host and port parts for UI display.
 * In the UI, the server address (host) is pure IP/domain (with scheme),
 * while the port is displayed and edited separately.
 */
export function splitServerUrl(serverUrl: string): { host: string; port: string } {
  const raw = serverUrl.trim()
  if (!raw) {
    return { host: '', port: '8096' }
  }

  const portMatch = raw.match(/:(\d+)(?:\/.*)?$/)
  const explicitPort = portMatch ? portMatch[1] : undefined

  const hasScheme = /^[a-zA-Z][a-zA-Z\d+\-.]*:\/\//.test(raw)
  const fullUrl = hasScheme ? raw : `http://${raw}`

  try {
    const u = new URL(fullUrl)
    const port = explicitPort ?? (u.protocol === 'https:' ? '' : '8096')
    u.port = ''
    let host = u.toString().replace(/\/$/, '')
    if (!hasScheme) {
      host = host.replace(/^https?:\/\//, '')
    }
    return { host, port }
  } catch {
    return { host: raw, port: explicitPort ?? '8096' }
  }
}

/**
 * Combines separate host and port into a full Emby server URL for underlying API calls.
 */
export function combineServerUrl(host: string, port: string): string {
  const trimmedHost = host.trim()
  const trimmedPort = port.trim()
  if (!trimmedHost) {
    return ''
  }

  const hasScheme = /^[a-zA-Z][a-zA-Z\d+\-.]*:\/\//.test(trimmedHost)
  const fullUrl = hasScheme ? trimmedHost : `http://${trimmedHost}`

  try {
    const u = new URL(fullUrl)
    u.port = trimmedPort || ''
    return u.toString().replace(/\/$/, '')
  } catch {
    if (trimmedPort) {
      return `${trimmedHost.replace(/\/$/, '')}:${trimmedPort}`
    }
    return trimmedHost
  }
}
