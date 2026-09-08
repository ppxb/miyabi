import {
  CheckCircle2Icon,
  LoaderCircleIcon,
  NetworkIcon,
  RefreshCwIcon,
  XCircleIcon
} from 'lucide-react'

import {
  type JavDBRouteCandidate,
  useJavDBRoute,
  useReselectJavDBRoute,
  useSelectJavDBRoute
} from '@/api/discover'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { SettingRow, SettingsSection } from './shared'

const AUTO_ROUTE_VALUE = '__auto__'

export function JavDBSection() {
  const route = useJavDBRoute()
  const reselect = useReselectJavDBRoute()
  const selectRoute = useSelectJavDBRoute()
  const status = route.data
  const value = status?.manual ? status.host : AUTO_ROUTE_VALUE
  const activeCandidate = status?.candidates.find(candidate => candidate.host === status.host)
  const busy = route.isFetching || reselect.isPending || selectRoute.isPending

  function changeRoute(next: string) {
    reselect.reset()
    selectRoute.mutate(next === AUTO_ROUTE_VALUE ? '' : next)
  }

  function refreshRoutes() {
    selectRoute.reset()
    if (route.isError) {
      reselect.reset()
      void route.refetch()
    } else {
      reselect.mutate()
    }
  }

  return (
    <SettingsSection icon={<NetworkIcon className="size-4" />} title="JavDB">
      <SettingRow title="接口线路" description="自动优选或手动选择线路，连接失败时自动重选。">
        <div className="flex w-full items-center gap-2 sm:w-auto">
          <Select
            value={value}
            disabled={busy || !status || route.isError}
            onValueChange={changeRoute}
          >
            <SelectTrigger className="min-w-0 flex-1 sm:w-72">
              <SelectValue>
                {route.isError ? (
                  '后端不可用'
                ) : route.isPending ? (
                  '正在读取线路'
                ) : value === AUTO_ROUTE_VALUE ? (
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="text-xs">自动优选</span>
                    {status?.host ? (
                      <span className="hidden truncate text-xs text-muted-foreground sm:inline">
                        {formatHost(status.host)}
                      </span>
                    ) : null}
                  </span>
                ) : (
                  <RouteDisplay host={value} candidate={activeCandidate} />
                )}
              </SelectValue>
            </SelectTrigger>
            <SelectContent position="popper" align="end">
              <SelectGroup>
                <SelectItem value={AUTO_ROUTE_VALUE}>自动优选</SelectItem>
                {status?.candidates.map(candidate => (
                  <SelectItem key={candidate.host} value={candidate.host}>
                    <RouteDisplay host={candidate.host} candidate={candidate} />
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="outline"
                size="icon"
                disabled={busy}
                onClick={refreshRoutes}
              >
                <RefreshCwIcon className={cn('size-4', busy && 'animate-spin')} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{route.isError ? '重试' : '重新测速'}</TooltipContent>
          </Tooltip>
        </div>
      </SettingRow>
      {route.isError ? (
        <div className="rounded-lg bg-muted px-3 py-2 text-sm text-muted-foreground">
          后端服务暂不可用，请启动后端服务后点击重试。本地显示设置仍可使用。
        </div>
      ) : selectRoute.isError || reselect.isError ? (
        <p className="text-sm text-destructive">
          {selectRoute.isError
            ? '线路切换未完成，请稍后重试。'
            : '暂时没有找到可用线路，请稍后重新测速。'}
        </p>
      ) : status && !status.active ? (
        <p className="text-xs text-muted-foreground">
          尚无缓存线路，首次请求时将完成全部线路测速。
        </p>
      ) : null}
    </SettingsSection>
  )
}

function RouteDisplay({ host, candidate }: { host: string; candidate?: JavDBRouteCandidate }) {
  return (
    <span className="flex w-full min-w-0 items-center justify-between gap-2">
      <span className="truncate">{formatHost(host)}</span>
      {candidate?.status === 'available' ? (
        <span
          className={cn(
            'inline-flex shrink-0 items-center gap-1 text-xs',
            latencyTone(candidate.latency_ms)
          )}
        >
          <CheckCircle2Icon className="size-3" />
          {candidate.latency_ms} ms
        </span>
      ) : candidate?.status === 'unavailable' ? (
        <span className="inline-flex shrink-0 items-center gap-1 text-xs text-destructive">
          <XCircleIcon className="size-3" />
          不可用
        </span>
      ) : (
        <span className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
          <LoaderCircleIcon className="size-3" />
          未测速
        </span>
      )}
    </span>
  )
}

function formatHost(host: string) {
  return host.replace(/^https?:\/\//, '')
}

function latencyTone(latencyMS: number) {
  if (latencyMS <= 500) return 'text-emerald-600 dark:text-emerald-400'
  if (latencyMS <= 1500) return 'text-amber-600 dark:text-amber-400'
  return 'text-orange-600 dark:text-orange-400'
}
