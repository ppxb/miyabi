import { useMemo, useState } from 'react'
import { LoaderCircleIcon, RefreshCwIcon, TvMinimalIcon } from 'lucide-react'
import { toast } from 'sonner'

import { type EmbyConfig, useEmbyConfig, useTestEmbyConfig, useUpdateEmbyConfig } from '@/api/emby'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { combineServerUrl, splitServerUrl } from './emby-url'
import { SettingRow, SettingsSection } from './shared'

interface EmbyFormData {
  enabled: boolean
  host: string
  port: string
  api_key: string
  media_path: string
  sync_actors: boolean
  public_url: string
}

function configToFormData(config?: EmbyConfig | null): EmbyFormData {
  const { host, port } = splitServerUrl(config?.server_url ?? '')
  return {
    enabled: config?.enabled ?? false,
    host,
    port,
    api_key: config?.api_key ?? '',
    media_path: config?.media_path ?? '',
    sync_actors: config?.sync_actors ?? true,
    public_url: config?.public_url ?? ''
  }
}

export function EmbySection() {
  const emby = useEmbyConfig()
  const updateConfig = useUpdateEmbyConfig()
  const testConfig = useTestEmbyConfig()

  const config = emby.data
  const initial = useMemo(() => configToFormData(config), [config])
  const [form, setForm] = useState<EmbyFormData | null>(null)

  const current = form ?? initial

  const isDirty =
    Boolean(config) &&
    form !== null &&
    (current.enabled !== initial.enabled ||
      current.host !== initial.host ||
      current.port !== initial.port ||
      current.api_key !== initial.api_key ||
      current.media_path !== initial.media_path ||
      current.public_url !== initial.public_url ||
      current.sync_actors !== initial.sync_actors)

  const disabled = emby.isLoading || emby.isError || updateConfig.isPending

  function updateField<K extends keyof EmbyFormData>(key: K, value: EmbyFormData[K]) {
    setForm(prev => ({
      ...(prev ?? initial),
      [key]: value
    }))
  }

  function handleHostBlur() {
    const raw = current.host.trim()
    if (!raw) return
    const { host, port } = splitServerUrl(raw)
    if (host !== raw) {
      setForm(prev => ({
        ...(prev ?? initial),
        host,
        port
      }))
    }
  }

  function handleTest() {
    const trimmedHost = current.host.trim()
    const trimmedKey = current.api_key.trim()

    if (!trimmedHost) {
      toast.error('请填写 Emby 服务器地址')
      return
    }
    if (!trimmedKey) {
      toast.error('请填写 Emby API Key')
      return
    }

    const serverUrl = combineServerUrl(trimmedHost, current.port)

    testConfig.mutate(
      { server_url: serverUrl, api_key: trimmedKey },
      {
        onSuccess: data => {
          toast.success('Emby 连接成功', {
            description: `${data.server_name} (v${data.version})`
          })
        },
        onError: error => {
          toast.error(error instanceof Error ? error.message : '连接 Emby 服务器失败')
        }
      }
    )
  }

  function handleSave() {
    const trimmedHost = current.host.trim()
    const trimmedKey = current.api_key.trim()

    if (current.enabled) {
      if (!trimmedHost) {
        toast.error('启用 Emby 集成时必须填写服务器地址')
        return
      }
      if (!trimmedKey) {
        toast.error('启用 Emby 集成时必须填写 API Key')
        return
      }
    }

    const serverUrl = combineServerUrl(trimmedHost, current.port)

    const payload: EmbyConfig = {
      enabled: current.enabled,
      server_url: serverUrl,
      api_key: trimmedKey,
      media_path: current.media_path.trim(),
      sync_actors: current.sync_actors,
      public_url: current.public_url.trim()
    }

    updateConfig.mutate(payload, {
      onSuccess: () => {
        setForm(null)
        toast.success('Emby 设置已保存')
      },
      onError: error => {
        toast.error(error instanceof Error ? error.message : '保存 Emby 设置失败')
      }
    })
  }

  return (
    <SettingsSection icon={<TvMinimalIcon className="size-4" />} title="Emby">
      <SettingRow
        title="启用 Emby"
        description="媒体扫描与刮削完成后，主动通知 Emby 增量刷新"
        inline
      >
        <Switch
          checked={current.enabled}
          disabled={disabled}
          onCheckedChange={checked => updateField('enabled', checked)}
        />
      </SettingRow>

      <SettingRow
        title="同步演员头像"
        description="自动匹配 GFriends 高清女优头像并同步至 Emby"
        inline
      >
        <Switch
          checked={current.sync_actors}
          disabled={disabled || !current.enabled}
          onCheckedChange={checked => updateField('sync_actors', checked)}
        />
      </SettingRow>

      <SettingRow title="服务器地址" description="Emby 服务的 IP 或域名">
        <Input
          value={current.host}
          placeholder="http://192.168.1.100"
          disabled={disabled}
          onChange={e => updateField('host', e.target.value)}
          onBlur={handleHostBlur}
        />
      </SettingRow>

      <SettingRow title="端口" description="Emby 服务端口，默认 8096">
        <Input
          value={current.port}
          placeholder="8096"
          disabled={disabled}
          onChange={e => updateField('port', e.target.value)}
        />
      </SettingRow>

      <SettingRow
        title="Miyabi 对外服务地址"
        description="生成 STRM 播放文件时写入的对外地址，供播放器直接访问。留空自动使用局域网 IP"
      >
        <Input
          value={current.public_url}
          placeholder="http://<局域网IP>:8080"
          disabled={disabled}
          onChange={e => updateField('public_url', e.target.value)}
        />
      </SettingRow>

      <SettingRow title="API Key" description="在 Emby 管理后台「高级」→「API 密钥」中生成">
        <Input
          type="password"
          value={current.api_key}
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
          value={current.media_path}
          placeholder="/media"
          disabled={disabled}
          onChange={e => updateField('media_path', e.target.value)}
        />
      </SettingRow>

      <div className="flex items-center justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || testConfig.isPending}
          onClick={handleTest}
        >
          <RefreshCwIcon className={cn('size-3.5', testConfig.isPending && 'animate-spin')} />
          测试连接
        </Button>
        <Button type="button" size="sm" disabled={disabled || !isDirty} onClick={handleSave}>
          {updateConfig.isPending ? (
            <LoaderCircleIcon className="mr-1.5 size-3.5 animate-spin" />
          ) : null}
          保存
        </Button>
      </div>

      {emby.isError ? <InlineError>后端服务暂不可用，无法读取 Emby 配置。</InlineError> : null}
    </SettingsSection>
  )
}
