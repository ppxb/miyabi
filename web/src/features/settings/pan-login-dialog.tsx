import { useQueryClient } from '@tanstack/react-query'
import { CheckCircle2Icon, LoaderCircleIcon, QrCodeIcon, RefreshCwIcon } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'

import {
  invalidatePanSource,
  panKeys,
  useBeginPanLogin,
  usePanLoginStatus,
  type PanAccountStatus,
  type PanLoginSession
} from '@/api/pan'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'

export function PanLoginDialog({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const login = useBeginPanLogin()

  function changeOpen(next: boolean) {
    setOpen(next)
    if (next) {
      login.mutate()
    } else {
      login.reset()
      void queryClient.invalidateQueries({ queryKey: panKeys.account })
    }
  }

  function completeLogin() {
    queryClient.setQueryData<PanAccountStatus>(panKeys.account, { connected: true })
    invalidatePanSource(queryClient)
    changeOpen(false)
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>登录 115 网盘</DialogTitle>
          <DialogDescription>请使用 115 手机客户端扫码，并在手机上确认登录。</DialogDescription>
        </DialogHeader>
        {open ? (
          <LoginContent
            session={login.data}
            pending={login.isPending}
            failed={login.isError}
            onRetry={() => login.mutate()}
            onAuthorized={completeLogin}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function LoginContent({
  session,
  pending,
  failed,
  onRetry,
  onAuthorized
}: {
  session?: PanLoginSession
  pending: boolean
  failed: boolean
  onRetry: () => void
  onAuthorized: () => void
}) {
  const status = usePanLoginStatus(session?.id ?? '')
  const state = status.data?.state
  const unavailable = failed || status.isError || state === 'expired' || state === 'canceled'

  useEffect(() => {
    if (state === 'authorized') {
      onAuthorized()
    }
  }, [state, onAuthorized])

  let message = '等待扫码'
  if (pending) {
    message = '正在获取二维码…'
  } else if (failed) {
    message = '二维码获取失败，请检查后端服务和网络后重试。'
  } else if (status.isError) {
    message = '登录未完成，请重新获取二维码。'
  } else if (state === 'expired') {
    message = '二维码已过期，请重新获取。'
  } else if (state === 'canceled') {
    message = '已取消授权，可以重新扫码。'
  } else if (state === 'scanned') {
    message = '已扫码，请在手机上确认登录'
  } else if (state === 'authorized') {
    message = '登录成功'
  }

  return (
    <div className="flex flex-col items-center gap-5">
      <div className="flex aspect-square w-full max-w-64 items-center justify-center overflow-hidden rounded-2xl border bg-muted">
        {pending ? (
          <LoaderCircleIcon className="size-8 animate-spin text-muted-foreground" />
        ) : session && !unavailable ? (
          <img
            src={session.qr_code}
            width={256}
            height={256}
            className="size-full bg-white object-contain p-3"
          />
        ) : (
          <QrCodeIcon className="size-12 text-muted-foreground" />
        )}
      </div>
      <p className="flex items-center gap-2 text-center text-sm text-muted-foreground">
        {state === 'scanned' ? (
          <CheckCircle2Icon className="size-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
        ) : null}
        {message}
      </p>
      {unavailable ? (
        <Button type="button" variant="outline" onClick={onRetry}>
          <RefreshCwIcon className="size-4" />
          重新获取二维码
        </Button>
      ) : null}
    </div>
  )
}
