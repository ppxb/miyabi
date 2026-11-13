import { useState, type FormEvent } from 'react'
import { KeyRoundIcon, LoaderCircleIcon } from 'lucide-react'
import { toast } from 'sonner'

import { useSetupAccount } from '@/api/setup'
import { describeApiError } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SettingsSection } from './shared'

export function AccountSection() {
  const save = useSetupAccount()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!username.trim()) {
      toast.error('请填写用户名')
      return
    }
    if (password.length < 8) {
      toast.error('密码至少需要 8 位')
      return
    }
    if (password !== confirm) {
      toast.error('两次输入的密码不一致')
      return
    }
    save.mutate(
      { username: username.trim(), password },
      {
        onSuccess: () => {
          setPassword('')
          setConfirm('')
          toast.success('管理员账号已更新')
        },
        onError: error => toast.error(describeApiError(error))
      }
    )
  }

  return (
    <SettingsSection icon={<KeyRoundIcon className="size-4" />} title="管理员账号">
      <form className="space-y-4" onSubmit={handleSubmit}>
        <div className="space-y-2">
          <Label htmlFor="account-username">用户名</Label>
          <Input
            id="account-username"
            value={username}
            autoComplete="username"
            disabled={save.isPending}
            placeholder="输入新的管理员用户名"
            onChange={event => setUsername(event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="account-password">新密码</Label>
          <Input
            id="account-password"
            type="password"
            value={password}
            autoComplete="new-password"
            disabled={save.isPending}
            placeholder="至少 8 位"
            onChange={event => setPassword(event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="account-confirm">确认新密码</Label>
          <Input
            id="account-confirm"
            type="password"
            value={confirm}
            autoComplete="new-password"
            disabled={save.isPending}
            placeholder="再次输入新密码"
            onChange={event => setConfirm(event.target.value)}
          />
        </div>
        <div className="flex justify-end">
          <Button type="submit" size="sm" disabled={save.isPending}>
            {save.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
            更新账号
          </Button>
        </div>
      </form>
    </SettingsSection>
  )
}
