import { useState, type FormEvent } from 'react'
import { LoaderCircleIcon, ShieldCheckIcon } from 'lucide-react'

import { useSetupAccount } from '@/api/setup'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { StepActions } from './step-actions'

export function AccountStep({ configured, onNext }: { configured: boolean; onNext: () => void }) {
  const save = useSetupAccount()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [localError, setLocalError] = useState('')

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLocalError('')
    if (!username.trim()) {
      setLocalError('请填写用户名')
      return
    }
    if (password.length < 8) {
      setLocalError('密码至少需要 8 位')
      return
    }
    if (password !== confirm) {
      setLocalError('两次输入的密码不一致')
      return
    }
    save.mutate(
      { username: username.trim(), password },
      {
        onSuccess: onNext,
        onError: error => setLocalError(error.message)
      }
    )
  }

  if (configured) {
    return (
      <div className="space-y-6">
        <Alert>
          <ShieldCheckIcon />
          <AlertTitle>管理员账号已设置</AlertTitle>
          <AlertDescription>可以继续完成剩余的初始化设置。</AlertDescription>
        </Alert>
        <StepActions showBack={false} onBack={() => {}}>
          <Button type="button" onClick={onNext}>
            继续
          </Button>
        </StepActions>
      </div>
    )
  }

  return (
    <form className="space-y-6" onSubmit={handleSubmit}>
      <div className="space-y-4">
        <Label htmlFor="setup-username">用户名</Label>
        <Input
          id="setup-username"
          value={username}
          autoComplete="username"
          autoFocus
          disabled={save.isPending}
          placeholder="请输入用户名"
          onChange={event => setUsername(event.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="setup-password">密码</Label>
        <Input
          id="setup-password"
          type="password"
          value={password}
          autoComplete="new-password"
          disabled={save.isPending}
          placeholder="请输入密码"
          onChange={event => setPassword(event.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="setup-confirm">确认密码</Label>
        <Input
          id="setup-confirm"
          type="password"
          value={confirm}
          autoComplete="new-password"
          disabled={save.isPending}
          placeholder="请再次输入密码"
          onChange={event => setConfirm(event.target.value)}
        />
      </div>
      {localError ? <p className="text-sm text-destructive">{localError}</p> : null}
      <StepActions showBack={false} onBack={() => {}}>
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
          继续
        </Button>
      </StepActions>
    </form>
  )
}
