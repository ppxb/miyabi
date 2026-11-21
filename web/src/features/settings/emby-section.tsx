import { useMemo, useState } from 'react'
import { RefreshCwIcon, SaveIcon, TvMinimalIcon } from 'lucide-react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import { type EmbyConfig, useEmbyConfig, useTestEmbyConfig, useUpdateEmbyConfig } from '@/api/emby'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { SettingRow, SettingsActions, SettingsSection } from './shared'

interface EmbyFormData {
  enabled: boolean
  server_url: string
  api_key: string
  media_path: string
  sync_actors: boolean
  public_url: string
}

function configToFormData(config: EmbyConfig): EmbyFormData {
  return {
    enabled: config.enabled,
    server_url: config.server_url,
    api_key: config.api_key,
    media_path: config.media_path,
    sync_actors: config.sync_actors ?? true,
    public_url: config.public_url ?? ''
  }
}

export function EmbySection() {
  const emby = useEmbyConfig()
  const [resetToken, setResetToken] = useState(0)

  return (
    <SettingsSection icon={<TvMinimalIcon className="size-4" />} title="Emby">
      {emby.isPending ? <Skeleton className="h-4 w-40" /> : null}
      {emby.data ? (
        <EmbyForm
          key={resetToken}
          config={emby.data}
          onSaved={() => setResetToken(token => token + 1)}
        />
      ) : null}

      {emby.isError ? (
        <InlineError onRetry={() => void emby.refetch()} retrying={emby.isFetching}>
          后端服务暂不可用，无法读取 Emby 配置。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}

function EmbyForm({ config, onSaved }: { config: EmbyConfig; onSaved: () => void }) {
  const updateConfig = useUpdateEmbyConfig()
  const testConfig = useTestEmbyConfig()

  const initial = useMemo(() => configToFormData(config), [config])
  const [form, setForm] = useState<EmbyFormData>(initial)

  const isDirty =
    form.enabled !== initial.enabled ||
    form.server_url !== initial.server_url ||
    form.api_key !== initial.api_key ||
    form.media_path !== initial.media_path ||
    form.public_url !== initial.public_url ||
    form.sync_actors !== initial.sync_actors

  const disabled = updateConfig.isPending

  function updateField<K extends keyof EmbyFormData>(key: K, value: EmbyFormData[K]) {
    setForm(prev => ({ ...prev, [key]: value }))
  }

  function handleToggle(checked: boolean) {
    if (checked) {
      updateField('enabled', true)
      return
    }

    updateField('enabled', false)
    if (!config.enabled) return

    updateConfig.mutate(
      { ...config, enabled: false },
      {
        onSuccess: () => {
          toast.success('已关闭 Emby')
          onSaved()
        },
        onError: error => {
          toast.error(describeApiError(error))
          setForm(initial)
        }
      }
    )
  }

  function handleTest() {
    const trimmedServerUrl = form.server_url.trim()
    const trimmedKey = form.api_key.trim()

    if (!trimmedServerUrl) {
      toast.error('请填写 Emby 服务器地址')
      return
    }
    if (!trimmedKey) {
      toast.error('请填写 Emby API Key')
      return
    }

    testConfig.mutate(
      { server_url: trimmedServerUrl, api_key: trimmedKey },
      {
        onSuccess: data => {
          toast.success('Emby 连接成功', {
            description: `${data.server_name} (v${data.version})`
          })
        },
        onError: error => {
          toast.error(describeApiError(error))
        }
      }
    )
  }

  function handleSave() {
    const trimmedServerUrl = form.server_url.trim()
    const trimmedKey = form.api_key.trim()

    if (!trimmedServerUrl) {
      toast.error('请填写服务器地址')
      return
    }
    if (!trimmedKey) {
      toast.error('请填写 API Key')
      return
    }

    const payload: EmbyConfig = {
      enabled: form.enabled,
      server_url: trimmedServerUrl,
      api_key: trimmedKey,
      media_path: form.media_path.trim(),
      sync_actors: form.sync_actors,
      public_url: form.public_url.trim()
    }

    updateConfig.mutate(payload, {
      onSuccess: () => {
        toast.success('Emby 设置已保存')
        onSaved()
      },
      onError: error => {
        toast.error(describeApiError(error))
      }
    })
  }

  return (
    <>
      <SettingRow title="启用 Emby" description="连接和管理 Emby" inline>
        <Switch checked={form.enabled} disabled={disabled} onCheckedChange={handleToggle} />
      </SettingRow>

      {form.enabled ? (
        <>
          <SettingRow
            title="同步演员头像"
            description="自动匹配 GFriends 高清女优头像并同步至 Emby"
            inline
          >
            <Switch
              checked={form.sync_actors}
              disabled={disabled}
              onCheckedChange={checked => updateField('sync_actors', checked)}
            />
          </SettingRow>

          <SettingRow title="服务器地址" description="Emby 服务的完整地址，含协议与端口">
            <Input
              autoFocus={form.enabled && !initial.enabled}
              value={form.server_url}
              placeholder="http://192.168.1.100:8096"
              disabled={disabled}
              onChange={e => updateField('server_url', e.target.value)}
            />
          </SettingRow>

          <SettingRow
            title="Miyabi 对外服务地址"
            description="生成 STRM 播放文件时写入的对外地址，供播放器直接访问。留空自动使用局域网 IP"
          >
            <Input
              value={form.public_url}
              placeholder="http://<局域网IP>:8080"
              disabled={disabled}
              onChange={e => updateField('public_url', e.target.value)}
            />
          </SettingRow>

          <SettingRow title="API Key" description="在 Emby 管理后台「高级」→「API 密钥」中生成">
            <Input
              type="password"
              value={form.api_key}
              placeholder="填入 Emby API Key"
              disabled={disabled}
              onChange={e => updateField('api_key', e.target.value)}
            />
          </SettingRow>

          <SettingRow
            title="Emby 媒体库路径"
            description="Emby 容器内挂载的对应目录。留空则默认使用本地导出路径"
          >
            <Input
              value={form.media_path}
              placeholder="/media"
              disabled={disabled}
              onChange={e => updateField('media_path', e.target.value)}
            />
          </SettingRow>

          <SettingsActions>
            <Button
              type="button"
              variant="outline"
              disabled={disabled || testConfig.isPending}
              onClick={handleTest}
            >
              {testConfig.isPending ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <RefreshCwIcon data-icon="inline-start" />
              )}
              测试连接
            </Button>
            <Button type="button" disabled={disabled || !isDirty} onClick={handleSave}>
              {updateConfig.isPending ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <SaveIcon data-icon="inline-start" />
              )}
              保存
            </Button>
          </SettingsActions>
        </>
      ) : null}
    </>
  )
}
