import { useEffect, useState, type FormEvent } from 'react'
import { KeyRoundIcon, ShieldCheckIcon } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { StepActions } from './step-actions'

export function AccountStep({
  configured,
  currentUsername,
  draft,
  onNext
}: {
  configured: boolean
  currentUsername: string
  draft: { username: string; password: string } | null
  onNext: (draft?: { username: string; password: string }) => void
}) {
  const [editing, setEditing] = useState(!configured || draft !== null)
  const [username, setUsername] = useState(draft?.username ?? currentUsername)
  const [password, setPassword] = useState(draft?.password ?? '')
  const [confirm, setConfirm] = useState(draft?.password ?? '')
  const [localError, setLocalError] = useState('')

  useEffect(() => {
    if (!editing) setUsername(currentUsername)
  }, [currentUsername, editing])

  function resetForm() {
    setUsername(currentUsername)
    setPassword('')
    setConfirm('')
    setLocalError('')
  }

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
    onNext({ username: username.trim(), password })
  }

  if (configured && !editing) {
    return (
      <div className="space-y-6">
        <Alert>
          <ShieldCheckIcon />
          <AlertTitle>管理员账号已设置</AlertTitle>
          <AlertDescription>
            当前用户名：{currentUsername || 'admin'}。需要修改时点击「修改账号密码」。
          </AlertDescription>
        </Alert>
        <StepActions showBack={false} onBack={() => {}}>
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" onClick={() => setEditing(true)}>
              <KeyRoundIcon />
              修改账号密码
            </Button>
            <Button type="button" onClick={() => onNext()}>
              继续
            </Button>
          </div>
        </StepActions>
      </div>
    )
  }

  return (
    <form className="space-y-6" onSubmit={handleSubmit}>
      <div className="space-y-2">
        <Label htmlFor="setup-username">用户名</Label>
        <Input
          id="setup-username"
          value={username}
          autoComplete="username"
          autoFocus
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
          placeholder="请输入密码（至少 8 位）"
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
          placeholder="请再次输入密码"
          onChange={event => setConfirm(event.target.value)}
        />
      </div>
      {localError ? <p className="text-sm text-destructive">{localError}</p> : null}
      <StepActions showBack={false} onBack={() => {}}>
        <div className="flex items-center gap-2">
          {configured ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                setEditing(false)
                resetForm()
              }}
            >
              取消
            </Button>
          ) : null}
          <Button type="submit">{configured ? '保存并继续' : '继续'}</Button>
        </div>
      </StepActions>
    </form>
  )
}
