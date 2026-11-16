import { CloudIcon, FolderOpenIcon, LoaderCircleIcon, QrCodeIcon } from 'lucide-react'

import { usePanAccount, type PanDirectory } from '@/api/pan'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { InlineError } from '@/components/error-state'
import { PanAccountInfo, PanStorageUsage } from '@/features/settings/pan-account'
import { PanLoginDialog } from '@/features/settings/pan-login-dialog'
import { PanDirectoryPicker } from './pan-directory-picker'
import { StepActions } from './step-actions'

export function PanStep({
  directory,
  onDirectoryChange,
  onNext,
  onBack
}: {
  directory?: PanDirectory
  onDirectoryChange: (directory: PanDirectory) => void
  onNext: () => void
  onBack: () => void
}) {
  const account = usePanAccount()
  const connected = account.data?.connected === true
  const profile = account.data?.account
  const selected = directory ?? account.data?.directory

  return (
    <div className="space-y-6">
      <Alert>
        <CloudIcon />
        <AlertTitle>登录 115 网盘</AlertTitle>
        <AlertDescription>
          使用 115 手机客户端扫码并确认，登录后请选择一个媒体目录。
        </AlertDescription>
      </Alert>

      {!connected ? (
        <div className="flex flex-col items-center gap-4 rounded-2xl border border-dashed p-8">
          {account.isPending ? (
            <LoaderCircleIcon className="size-6 animate-spin text-muted-foreground" />
          ) : (
            <>
              <QrCodeIcon className="size-10 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">尚未登录 115 网盘</p>
              <PanLoginDialog>
                <Button type="button">
                  <QrCodeIcon />
                  扫码登录 115
                </Button>
              </PanLoginDialog>
            </>
          )}
          {account.isError ? (
            <InlineError onRetry={() => void account.refetch()} retrying={account.isFetching}>
              暂时无法读取登录状态，请检查网络后重试。
            </InlineError>
          ) : null}
        </div>
      ) : profile ? (
        <div className="space-y-4 rounded-2xl border p-4">
          <PanAccountInfo account={profile} />
          <div className="flex items-center justify-between gap-4 rounded-xl bg-muted/50 p-3">
            <div className="min-w-0 space-y-0.5">
              <div className="text-sm font-medium">媒体目录</div>
              <div className="truncate text-xs text-muted-foreground">
                {selected ? selected.path || selected.name : '尚未选择'}
              </div>
            </div>
            <PanDirectoryPicker
              accountID={profile.id}
              selected={selected}
              onSelected={onDirectoryChange}
            >
              <Button type="button" variant="outline">
                <FolderOpenIcon />
                {selected ? '更换目录' : '选择目录'}
              </Button>
            </PanDirectoryPicker>
          </div>
          <PanStorageUsage space={profile.space} />
        </div>
      ) : null}

      <StepActions onBack={onBack}>
        <Button type="button" disabled={!connected || !selected} onClick={onNext}>
          继续
        </Button>
      </StepActions>
    </div>
  )
}
