import { useEffect, useState, type ReactElement } from 'react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import { useBeginGuangyaLogin, useVerifyGuangyaLogin } from '@/api/pan'
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
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

/** Two-step SMS login for the Guangya private API. */
export function GuangyaLoginDialog({
  children,
  onAuthorized
}: {
  children: ReactElement
  onAuthorized?: (accountID: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [phone, setPhone] = useState('')
  const [code, setCode] = useState('')
  const [sessionID, setSessionID] = useState('')
  const [cooldown, setCooldown] = useState(0)
  const begin = useBeginGuangyaLogin()
  const verify = useVerifyGuangyaLogin()

  useEffect(() => {
    if (cooldown <= 0) return
    const timer = window.setTimeout(() => setCooldown(value => value - 1), 1000)
    return () => window.clearTimeout(timer)
  }, [cooldown])

  function changeOpen(next: boolean) {
    setOpen(next)
    // Reset only on open: clearing fields while the popup is animating out makes
    // the code row disappear before the dialog has finished closing.
    if (next) {
      setPhone('')
      setCode('')
      setSessionID('')
      setCooldown(0)
      begin.reset()
      verify.reset()
    }
  }

  function sendCode() {
    begin.mutate(phone.trim(), {
      onSuccess: session => {
        setSessionID(session.id)
        setCooldown(60)
        toast.success('验证码已发送，请查看短信')
      },
      onError: error => toast.error(describeApiError(error))
    })
  }

  function submit() {
    verify.mutate(
      { id: sessionID, code: code.trim() },
      {
        onSuccess: status => {
          const accountID = status.account?.id ?? status.account_id ?? ''
          if (accountID) {
            onAuthorized?.(accountID)
          }
          toast.success('光鸭账号登录成功')
          changeOpen(false)
        },
        onError: error => toast.error(describeApiError(error))
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger render={children} />
      <DialogContent>
        <DialogHeader>
          <DialogTitle>登录光鸭云盘</DialogTitle>
          <DialogDescription>
            输入手机号获取验证码，验证码会发送到光鸭绑定的手机。
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="guangya-phone">手机号</FieldLabel>
            <Input
              id="guangya-phone"
              inputMode="tel"
              autoComplete="tel"
              placeholder="请输入手机号码"
              value={phone}
              disabled={begin.isPending || verify.isPending}
              onChange={event => {
                setPhone(event.target.value)
                setSessionID('')
                setCode('')
                // A different number needs its own code; allow sending again.
                setCooldown(0)
              }}
            />
            <FieldDescription>中国手机号可只输入 11 位，系统会自动补 +86。</FieldDescription>
          </Field>
          {sessionID ? (
            <Field>
              <FieldLabel htmlFor="guangya-code">短信验证码</FieldLabel>
              <Input
                id="guangya-code"
                inputMode="numeric"
                autoComplete="one-time-code"
                placeholder="请输入验证码"
                value={code}
                disabled={verify.isPending}
                onChange={event => setCode(event.target.value)}
              />
            </Field>
          ) : null}
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={begin.isPending || verify.isPending || cooldown > 0 || phone.trim() === ''}
              onClick={sendCode}
            >
              {begin.isPending ? <Spinner data-icon="inline-start" /> : null}
              {begin.isPending
                ? '发送中…'
                : cooldown > 0
                  ? `${cooldown}s 后重新发送`
                  : '发送验证码'}
            </Button>
            {sessionID ? (
              <Button
                type="button"
                disabled={verify.isPending || code.trim() === ''}
                onClick={submit}
              >
                {verify.isPending ? <Spinner data-icon="inline-start" /> : null}
                登录
              </Button>
            ) : null}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
