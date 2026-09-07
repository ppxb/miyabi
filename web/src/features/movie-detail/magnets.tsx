import { CheckIcon, CopyIcon } from 'lucide-react'
import { useState } from 'react'

import type { DiscoverMagnet, useDiscoverMagnets } from '@/api/discover'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

export function MovieMagnets({ query }: { query: ReturnType<typeof useDiscoverMagnets> }) {
  const [copiedHash, setCopiedHash] = useState('')
  const [copyError, setCopyError] = useState(false)

  async function copy(magnet: DiscoverMagnet) {
    setCopyError(false)
    try {
      await navigator.clipboard.writeText(magnet.uri)
      setCopiedHash(magnet.hash)
    } catch {
      setCopyError(true)
    }
  }

  return (
    <section className="space-y-4">
      <h2 id="movie-magnets-title" className="text-xl font-semibold tracking-normal">
        磁力链
      </h2>
      {query.isPending ? (
        <div className="space-y-2">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-30 rounded-2xl" />
          ))}
        </div>
      ) : query.isError ? (
        <div className="flex items-center gap-3 text-sm text-destructive">
          <span>磁力加载失败</span>
          <Button type="button" variant="outline" size="sm" onClick={() => query.refetch()}>
            重试
          </Button>
        </div>
      ) : query.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">暂无磁力链</p>
      ) : (
        <div className="space-y-4">
          {query.data.map(magnet => (
            <Card key={magnet.hash} size="sm">
              <CardContent className="flex items-center">
                <div className="min-w-0 flex-1 space-y-2">
                  <h3 className="text-base leading-6 font-semibold wrap-break-word">
                    {magnet.name}
                  </h3>
                  <a
                    href={magnet.uri}
                    className="block font-mono text-xs break-all text-muted-foreground outline-ring select-all hover:text-foreground"
                  >
                    {magnet.uri}
                  </a>
                  <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm leading-6 text-muted-foreground">
                    <span className="text-xs">{formatSize(magnet.size)}</span>
                    {magnet.has_subtitle ? <Badge variant="outline">字幕</Badge> : null}
                    {magnet.hd ? <Badge variant="outline">高清</Badge> : null}
                  </div>
                </div>
                <Button type="button" variant="outline" size="sm" onClick={() => copy(magnet)}>
                  {copiedHash === magnet.hash ? <CheckIcon /> : <CopyIcon />}
                  {copiedHash === magnet.hash ? '已复制' : '复制'}
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
      {copyError ? (
        <p role="alert" className="text-sm text-destructive">
          复制失败，请重试。
        </p>
      ) : null}
    </section>
  )
}

function formatSize(bytes: number) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = bytes
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit++
  }
  return `${new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 }).format(size)} ${units[unit]}`
}
