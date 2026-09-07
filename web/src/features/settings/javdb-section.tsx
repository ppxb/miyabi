import { NetworkIcon, RefreshCwIcon } from 'lucide-react'

import { useJavDBRoute, useReselectJavDBRoute } from '@/api/discover'
import { Button } from '@/components/ui/button'
import { SettingRow, SettingsSection } from './shared'

export function JavDBSection() {
  const route = useJavDBRoute()
  const reselect = useReselectJavDBRoute()
  const status = route.data

  return (
    <SettingsSection icon={<NetworkIcon className="size-4" />} title="JavDB">
      <SettingRow
        title="当前线路"
        description={
          <>
            <p>{route.isLoading ? '正在读取线路' : status?.host || '尚未选择线路'}</p>
            {status?.host ? (
              <p>
                {status.active ? '已连接' : '首次访问发现页时验证'}
                {status.latency_ms > 0 ? ` · ${status.latency_ms} ms` : ''}
              </p>
            ) : null}
          </>
        }
      >
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
      </SettingRow>
      {route.isError || reselect.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {route.error?.message || reselect.error?.message}
        </p>
      ) : null}
    </SettingsSection>
  )
}
