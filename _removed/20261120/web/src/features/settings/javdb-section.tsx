import { useMemo, useState } from 'react'
import { KeyRoundIcon, LoaderCircleIcon, RefreshCwIcon } from 'lucide-react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import {
  type JavDBConfig,
  useJavDBConfig,
  useTestJavDBLogin,
  useUpdateJavDBConfig
} from '@/api/javdb'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SettingRow, SettingsSection } from './shared'

type JavDBForm = JavDBConfig

function configToForm(config?: JavDBConfig | null): JavDBForm {
  return {
    username: config?.username ?? '',
    password: config?.password ?? ''
  }
}

export function JavDBSection() {
  const query = useJavDBConfig()
  const updateConfig = useUpdateJavDBConfig()
  const testLogin = useTestJavDBLogin()

  const config = query.data
  const initial = useMemo(() => configToForm(config), [config])
  const [form, setForm] = useState<JavDBForm | null>(null)
  const current = form ?? initial

  const isDirty =
    Boolean(config) &&
    form !== null &&
    (current.username !== initial.username || current.password !== initial.password)

  // Keep the saved form visible until the observer receives the same values.
  if (form !== null && !isDirty && !updateConfig.isPending) setForm(null)

  const disabled = query.isLoading || query.isError || updateConfig.isPending

  function updateField<K extends keyof JavDBForm>(key: K, value: JavDBForm[K]) {
    setForm(prev => ({ ...(prev ?? initial), [key]: value }))
  }

  function normalized(): JavDBConfig | null {
    const username = current.username.trim()
    const password = current.password.trim()
    if (!username !== !password) {
      toast.error('JavDB 用户名和密码需同时填写')
      return null
    }
    return { username, password }
  }

  function handleTest() {
    const payload = normalized()
    if (!payload) return
    if (!payload.username) {
      toast.error('请填写 JavDB 用户名和密码')
      return
    }
    testLogin.mutate(payload, {
      onSuccess: data => {
        const name = data.username || payload.username
        toast.success('JavDB 登录成功', {
          description: data.user_id ? `${name}（ID ${data.user_id}）` : name
        })
      },
      onError: error => toast.error(describeApiError(error))
    })
  }

  function handleSave() {
    const payload = normalized()
    if (!payload) return
    updateConfig.mutate(payload, {
      onSuccess: next => {
        setForm(configToForm(next))
        toast.success(payload.username ? 'JavDB 账号已保存' : '已清除 JavDB 账号')
      },
      onError: error => toast.error(describeApiError(error))
    })
  }

  return (
    <SettingsSection icon={<KeyRoundIcon className="size-4" />} title="JavDB 账号">
      <SettingRow title="用户名" description="搜索与番号解析需要登录；账号仅保存在本地数据库。">
        <Input
          autoComplete="username"
          value={current.username}
          placeholder="JavDB 用户名"
          disabled={disabled}
          onChange={event => updateField('username', event.target.value)}
        />
      </SettingRow>

      <SettingRow title="密码" description="仅保存在本地">
        <Input
          type="password"
          autoComplete="current-password"
          value={current.password}
          placeholder="JavDB 密码"
          disabled={disabled}
          onChange={event => updateField('password', event.target.value)}
        />
      </SettingRow>

      <div className="flex items-center justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || testLogin.isPending}
          onClick={handleTest}
        >
          <RefreshCwIcon className={testLogin.isPending ? 'animate-spin' : undefined} />
          测试登录
        </Button>
        <Button type="button" size="sm" disabled={disabled || !isDirty} onClick={handleSave}>
          {updateConfig.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
          保存
        </Button>
      </div>

      {query.isError ? <InlineError>后端服务暂不可用，无法读取 JavDB 配置。</InlineError> : null}
    </SettingsSection>
  )
}
