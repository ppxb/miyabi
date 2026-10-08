import { BellIcon } from 'lucide-react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import {
  type MagnetPreferences,
  type DownloadConfig,
  type SubscriptionConfig,
  useSubscriptionSettings,
  useUpdateSubscriptionSettings
} from '@/api/subscription-settings'
import { InlineError } from '@/components/error-state'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { SettingRow, SettingsSection } from './shared'

const levelOptions = [
  { value: 'preferred', label: '优先' },
  { value: 'required', label: '必须' },
  { value: 'any', label: '不限' }
] as const

const uncensoredOptions = [
  { value: 'any', label: '不限' },
  { value: 'preferred', label: '优先' },
  { value: 'required', label: '必须' },
  { value: 'exclude', label: '排除' }
] as const

const downloadLimits = [
  {
    key: 'zero_progress_minutes',
    title: '零进度等待时间',
    description: '下载开始后，进度一直为零时的等待分钟数',
    fallback: 15,
    max: 1440
  },
  {
    key: 'stalled_minutes',
    title: '下载停滞等待时间',
    description: '已有进度但不再增长时的等待分钟数',
    fallback: 30,
    max: 1440
  },
  {
    key: 'completion_grace_minutes',
    title: '临近完成等待时间',
    description: '进度达到 95% 后给予更长等待，单位为分钟',
    fallback: 60,
    max: 1440
  },
  {
    key: 'max_attempts',
    title: '磁力尝试上限',
    description: '包含首次下载；达到上限后继续等待，可手动取消',
    fallback: 3,
    max: 10
  }
] as const

export function SubscriptionSection() {
  const settings = useSubscriptionSettings()
  const update = useUpdateSubscriptionSettings()
  const config = settings.data
  const disabled = settings.isLoading || settings.isError || update.isPending

  function save(patch: Partial<SubscriptionConfig>, preferences?: Partial<MagnetPreferences>) {
    if (!config) return
    update.mutate(
      { ...config, ...patch, preferences: { ...config.preferences, ...preferences } },
      {
        onSuccess: () => toast.success('订阅与下载设置已保存'),
        onError: error => {
          toast.error(describeApiError(error))
        }
      }
    )
  }

  function saveDownload(patch: Partial<DownloadConfig>) {
    if (config) save({ download: { ...config.download, ...patch } })
  }

  return (
    <SettingsSection icon={<BellIcon className="size-4" />} title="订阅与下载">
      <SettingRow
        title="影片默认自动入库"
        description="新订阅的影片出现符合偏好的磁力后自动加入 115"
        inline
      >
        <Switch
          checked={config?.movie_auto_download ?? true}
          disabled={disabled}
          onCheckedChange={checked => save({ movie_auto_download: checked })}
        />
      </SettingRow>

      <SettingRow
        title="演员新作默认自动入库"
        description="演员订阅发现的新作是否默认自动加入 115"
        inline
      >
        <Switch
          checked={config?.actor_auto_download ?? false}
          disabled={disabled}
          onCheckedChange={checked => save({ actor_auto_download: checked })}
        />
      </SettingRow>

      <SettingRow
        title="自动切换磁力"
        description="下载长时间没有进展时，取消当前下载并尝试下一条符合偏好的磁力"
        inline
      >
        <Switch
          checked={config?.download.auto_switch ?? true}
          disabled={disabled}
          onCheckedChange={checked => saveDownload({ auto_switch: checked })}
        />
      </SettingRow>

      {downloadLimits.map(limit => (
        <SettingRow key={limit.key} title={limit.title} description={limit.description}>
          <Input
            key={config?.download[limit.key]}
            type="number"
            min={1}
            max={limit.max}
            step={1}
            aria-label={limit.title}
            className="w-24"
            defaultValue={config?.download[limit.key] ?? limit.fallback}
            disabled={disabled || !config?.download.auto_switch}
            onKeyDown={event => {
              if (event.key === 'Enter') event.currentTarget.blur()
            }}
            onBlur={event => {
              const value = event.currentTarget.valueAsNumber
              if (!Number.isInteger(value) || value < 1 || value > limit.max) {
                event.currentTarget.value = String(config?.download[limit.key] ?? limit.fallback)
                return
              }
              if (value !== config?.download[limit.key]) saveDownload({ [limit.key]: value })
            }}
          />
        </SettingRow>
      ))}

      <SettingRow title="字幕" description="订阅与换源共用；「必须」时只选择符合字幕条件的磁力">
        <Select
          value={config?.preferences.subtitle ?? 'preferred'}
          disabled={disabled}
          onValueChange={value => save({}, { subtitle: value as MagnetPreferences['subtitle'] })}
        >
          <SelectTrigger className="w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {levelOptions.map(option => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingRow>

      <SettingRow title="高清" description="「必须」时只接受站方标注高清或名称含 4K 的磁力">
        <Select
          value={config?.preferences.hd ?? 'preferred'}
          disabled={disabled}
          onValueChange={value => save({}, { hd: value as MagnetPreferences['hd'] })}
        >
          <SelectTrigger className="w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {levelOptions.map(option => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingRow>

      <SettingRow title="无码 / 破解" description="「排除」时过滤掉无码、破解、流出资源">
        <Select
          value={config?.preferences.uncensored ?? 'any'}
          disabled={disabled}
          onValueChange={value =>
            save({}, { uncensored: value as MagnetPreferences['uncensored'] })
          }
        >
          <SelectTrigger className="w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {uncensoredOptions.map(option => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingRow>

      {settings.isError ? (
        <InlineError>后端服务暂不可用，无法读取订阅与下载设置。</InlineError>
      ) : null}
    </SettingsSection>
  )
}
