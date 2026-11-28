import { useQueryClient } from '@tanstack/react-query'
import { CheckCircle2Icon, QrCodeIcon, RefreshCwIcon } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useEffect, useRef, useState, type ReactElement } from 'react'

import {
  invalidatePanSource,
  panKeys,
  useBeginGuangyaLogin,
  useGuangyaLoginStatus,
  type GuangyaLoginSession
} from '@/api/pan'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'

/** Device (QR) login for the Guangya private API. */
export function GuangyaLoginDialog({
  children,
  onAuthorized
}: {
  children: ReactElement
  onAuthorized?: (accountID: string) => void
}) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const login = useBeginGuangyaLogin()
  // Binding the account switches the source card away from its login branch,
  // which unmounts this dialog. Wait for the exit animation to finish first.
  const authorizedAccountID = useRef('')

  function changeOpen(next: boolean) {
    setOpen(next)
    if (next) {
      // Reset before starting a new session so the previous QR is not shown,
      // but keep it mounted while closing so the exit animation stays stable.
      login.reset()
      login.mutate()
    } else {
      void queryClient.invalidateQueries({ queryKey: panKeys.accounts })
    }
  }

  function completeLogin(accountID: string) {
    void queryClient.invalidateQueries({ queryKey: panKeys.accounts })
    invalidatePanSource(queryClient)
    authorizedAccountID.current = accountID
    changeOpen(false)
  }

  function finishClose(next: boolean) {
    if (next) return
    const accountID = authorizedAccountID.current
    authorizedAccountID.current = ''
    if (onAuthorized && accountID) {
      onAuthorized(accountID)
    }
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen} onOpenChangeComplete={finishClose}>
      <DialogTrigger render={children} />
      <DialogContent>
        <DialogHeader>
          <DialogTitle>登录光鸭云盘</DialogTitle>
          <DialogDescription>请使用光鸭 App 扫码，并在手机上确认登录。</DialogDescription>
        </DialogHeader>
        <LoginContent
          session={login.data}
          pending={login.isPending}
          failed={login.isError}
          onRetry={() => login.mutate()}
          onAuthorized={completeLogin}
        />
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
  session?: GuangyaLoginSession
  pending: boolean
  failed: boolean
  onRetry: () => void
  onAuthorized: (accountID: string) => void
}) {
  const status = useGuangyaLoginStatus(session?.id ?? '')
  const state = status.data?.state
  // A failing poll retries on its own budget before it reports an error, so an
  // error is a run of failures long enough to mean the backend is unreachable.
  // Until then the last answer stays on screen and the retries only show as such.
  const exhausted = status.isError
  const retrying = !exhausted && status.failureCount > 0
  const deadCode = failed || state === 'expired' || state === 'canceled'
  // A failing poll keeps the QR code on screen: it is the one the user is aiming
  // at, and it may still be live once the connection recovers.
  const showQR = session !== undefined && !deadCode
  const needsRetry = exhausted || deadCode
  // onAuthorized changes identity on every parent render; calling it again on
  // each render would re-enter the close/bind flow endlessly. Handle one
  // successful authorization per login session.
  const authorizedSessionId = useRef<string | null>(null)

  useEffect(() => {
    if (state !== 'authorized' || !session?.id || authorizedSessionId.current === session.id) return
    authorizedSessionId.current = session.id
    onAuthorized(status.data?.account_id ?? '')
  }, [state, status.data?.account_id, session?.id, onAuthorized])

  let message = '等待扫码'
  if (pending) {
    message = '正在获取二维码…'
  } else if (failed) {
    message = '二维码获取失败，请检查后端服务和网络后重试。'
  } else if (exhausted) {
    message = '登录未完成，请重新获取二维码。'
  } else if (state === 'expired') {
    message = '二维码已过期，请重新获取。'
  } else if (state === 'canceled') {
    message = '已取消授权或会话已被替换，请重新扫码。'
  } else if (state === 'scanned') {
    message = '已扫码，请在手机上确认登录'
  } else if (state === 'authorized') {
    message = '登录成功'
  } else if (retrying) {
    message = '网络异常，正在重试…'
  }

  return (
    <div className="flex flex-col items-center gap-5">
      <div className="flex aspect-square w-full max-w-64 items-center justify-center overflow-hidden rounded-2xl border bg-muted">
        {pending ? (
          <Spinner className="size-8 text-muted-foreground" />
        ) : showQR ? (
          <div className="size-full bg-white p-3">
            <QRCodeSVG
              value={session.qr_code_url}
              size={256}
              level="M"
              bgColor="#ffffff"
              fgColor="#000000"
              title="光鸭登录二维码"
              className="size-full"
            />
          </div>
        ) : (
          <QrCodeIcon className="size-12 text-muted-foreground" />
        )}
      </div>
      <p className="flex items-center gap-2 text-center text-sm text-muted-foreground">
        {state === 'scanned' ? <CheckCircle2Icon className="size-4 shrink-0 text-success" /> : null}
        {message}
      </p>
      {needsRetry && !pending ? (
        <Button type="button" variant="outline" onClick={onRetry}>
          <RefreshCwIcon className="size-4" />
          重新获取二维码
        </Button>
      ) : null}
    </div>
  )
}
