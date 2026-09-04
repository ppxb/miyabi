import { RefreshCwIcon } from 'lucide-react'

import { useJavDBRoute, useReselectJavDBRoute } from '@/api/discover'
import { Button } from '@/components/ui/button'

export function JavDBSection() {
  const route = useJavDBRoute()
  const reselect = useReselectJavDBRoute()
  const status = route.data

  return (
    <section className="rounded-3xl border border-border/70 bg-card/40 p-5 shadow-sm shadow-black/5">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h2 className="font-semibold">JavDB</h2>
          <p className="mt-2 break-all text-sm leading-6 text-muted-foreground">
            {status?.host ? status.host : '尚未选择线路'}
          </p>
          {status?.host ? (
            <p className="mt-1 text-xs text-muted-foreground">
              {status.active ? '当前线路' : '缓存线路，首次访问发现页时验证'}
              {status.latency_ms > 0 ? ` · ${status.latency_ms} ms` : ''}
            </p>
          ) : null}
          {route.isError || reselect.isError ? (
            <p className="mt-2 text-xs text-destructive">
              {route.error?.message || reselect.error?.message}
            </p>
          ) : null}
        </div>

        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={route.isLoading || reselect.isPending}
          onClick={() => reselect.mutate()}
        >
          <RefreshCwIcon className={reselect.isPending ? 'animate-spin' : undefined} />
          {reselect.isPending ? '选线中' : '重新选线'}
        </Button>
      </div>
    </section>
  )
}
