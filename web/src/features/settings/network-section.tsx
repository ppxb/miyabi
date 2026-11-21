import { useMemo, useRef, useState } from 'react'
import { GlobeIcon, SaveIcon } from 'lucide-react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import { type NetworkConfig, useNetworkConfig, useUpdateNetworkConfig } from '@/api/network'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { SettingRow, SettingsActions, SettingsSection } from './shared'

interface NetworkFormData {
  enabled: boolean
  url: string
}

function configToFormData(config: NetworkConfig): NetworkFormData {
  return { enabled: config.enabled, url: config.url }
}

export function NetworkSection() {
  const network = useNetworkConfig()
  const [resetToken, setResetToken] = useState(0)

  return (
    <SettingsSection icon={<GlobeIcon className="size-4" />} title="网络代理">
      {network.isPending ? <Skeleton className="h-4 w-40" /> : null}
      {network.data ? (
        <NetworkForm
          key={resetToken}
          config={network.data}
          onSaved={() => setResetToken(token => token + 1)}
        />
      ) : null}

      {network.isError ? (
        <InlineError onRetry={() => void network.refetch()} retrying={network.isFetching}>
          后端服务暂不可用，无法读取网络设置。请检查后端服务状态后刷新重试。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}

function NetworkForm({ config, onSaved }: { config: NetworkConfig; onSaved: () => void }) {
  const updateConfig = useUpdateNetworkConfig()
  const inputRef = useRef<HTMLInputElement>(null)

  const initial = useMemo(() => configToFormData(config), [config])
  const [form, setForm] = useState<NetworkFormData>(initial)

  const isDirty = form.enabled !== initial.enabled || form.url !== initial.url
  const disabled = updateConfig.isPending

  function updateField<K extends keyof NetworkFormData>(key: K, value: NetworkFormData[K]) {
    setForm(prev => ({ ...prev, [key]: value }))
  }

  function handleToggle(checked: boolean) {
    if (checked) {
      updateField('enabled', true)
      return
    }

    updateField('enabled', false)
    updateConfig.mutate(
      { ...config, enabled: false },
      {
        onSuccess: () => {
          toast.success('已关闭网络代理')
          onSaved()
        },
        onError: error => {
          toast.error(describeApiError(error))
          setForm(initial)
        }
      }
    )
  }

  function handleSave() {
    const normalized = normalizeProxyInput(form.url)
    if (!normalized) {
      toast.error('请填写代理地址以开启服务')
      inputRef.current?.focus()
      return
    }

    updateConfig.mutate(
      { enabled: form.enabled, url: normalized },
      {
        onSuccess: () => {
          toast.success(
            form.enabled ? (config.enabled ? '代理地址已保存' : '已开启网络代理') : '代理设置已保存'
          )
          onSaved()
        },
        onError: error => {
          toast.error(describeApiError(error))
        }
      }
    )
  }

  return (
    <>
      <SettingRow title="启用代理服务" description="将代理除 115 以外的所有网络请求" inline>
        <Switch checked={form.enabled} disabled={disabled} onCheckedChange={handleToggle} />
      </SettingRow>

      {form.enabled ? (
        <>
          <SettingRow title="代理地址" description="支持 HTTP、HTTPS 与 SOCKS5 代理协议">
            <Input
              ref={inputRef}
              autoFocus={!initial.enabled}
              value={form.url}
              placeholder="http://127.0.0.1:7890"
              disabled={disabled}
              onChange={e => updateField('url', e.target.value)}
            />
          </SettingRow>

          <SettingsActions>
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

function normalizeProxyInput(input: string): string {
  const trimmed = input.trim()
  if (!trimmed || /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(trimmed)) return trimmed
  return `http://${trimmed}`
}
