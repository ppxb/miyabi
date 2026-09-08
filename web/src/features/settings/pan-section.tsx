import { CloudIcon, LoaderCircleIcon, LogOutIcon, QrCodeIcon, RefreshCwIcon } from 'lucide-react'

import { ApiError } from '@/api/client'
import { useDisconnectPan, usePanAccount } from '@/api/pan'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { PanLoginDialog } from './pan-login-dialog'
import { PanDirectoryRow } from './pan-directory-row'
import { PanAccountInfo, PanStorageUsage } from './pan-account'
import { SettingRow, SettingsSection } from './shared'

export function PanSection() {
  const account = usePanAccount()
  const disconnect = useDisconnectPan()
  const unauthorized = account.error instanceof ApiError && account.error.status === 401
  const connected = account.data?.connected === true && !unauthorized
  const profile = account.data?.account

  let description = '使用 115 手机客户端扫码登录'
  if (account.isPending) {
    description = '正在读取登录状态…'
  } else if (unauthorized) {
    description = '授权已失效，请重新扫码登录'
  } else if (account.isError) {
    description = '暂时无法读取账号信息，请检查后端服务和网络后重试'
  } else if (connected) {
    description = '已登录，正在读取账号信息…'
  }

  const actions = (
    <div className="flex shrink-0 items-center gap-2">
      {account.isError ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label="重新获取 115 账号状态"
              disabled={account.isFetching}
              onClick={() => void account.refetch()}
            >
              {account.isFetching ? (
                <LoaderCircleIcon className="size-4 animate-spin" />
              ) : (
                <RefreshCwIcon className="size-4" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>重试</TooltipContent>
        </Tooltip>
      ) : null}
      {account.isPending ? (
        <LoaderCircleIcon
          className="size-4 animate-spin text-muted-foreground"
          aria-label="正在读取账号"
        />
      ) : !connected ? (
        <PanLoginDialog>
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label="扫码登录 115"
            disabled={disconnect.isPending}
          >
            <QrCodeIcon className="size-4" />
          </Button>
        </PanLoginDialog>
      ) : null}
      {connected || unauthorized ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label="退出 115 登录"
              disabled={disconnect.isPending}
              onClick={() => disconnect.mutate()}
            >
              {disconnect.isPending ? (
                <LoaderCircleIcon className="size-4 animate-spin" />
              ) : (
                <LogOutIcon className="size-4" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>退出登录</TooltipContent>
        </Tooltip>
      ) : null}
    </div>
  )

  return (
    <SettingsSection icon={<CloudIcon className="size-4" />} title="115 网盘">
      {connected && profile ? (
        <div className="flex items-center justify-between gap-4">
          <PanAccountInfo account={profile} />
          {actions}
        </div>
      ) : (
        <SettingRow title="账号登录" description={description} inline>
          {actions}
        </SettingRow>
      )}
      {connected && profile && account.isError ? (
        <p role="status" className="text-xs text-muted-foreground">
          暂时无法更新账号信息，请检查后端服务和网络后重试。
        </p>
      ) : null}
      {connected && profile ? (
        <>
          <PanDirectoryRow
            key={profile.id}
            accountID={profile.id}
            directory={account.data?.directory}
            disabled={account.isError || disconnect.isPending}
          />
          <PanStorageUsage space={profile.space} />
        </>
      ) : null}
      {disconnect.isError ? (
        <p role="alert" className="text-sm text-destructive">
          退出登录未完成，请检查后端服务后重试。
        </p>
      ) : null}
    </SettingsSection>
  )
}
