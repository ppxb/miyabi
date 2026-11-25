import { DownloadIcon } from 'lucide-react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import {
  type MagnetPreferences,
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
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
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

function PreferenceSelect<T extends string>({
  value,
  options,
  disabled,
  onChange
}: {
  value: T
  options: ReadonlyArray<{ value: T; label: string }>
  disabled?: boolean
  onChange: (value: T) => void
}) {
  return (
    <Select
      value={value}
      items={options}
      disabled={disabled}
      onValueChange={next => {
        if (next !== null) onChange(next)
      }}
    >
      <SelectTrigger className="w-32">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map(option => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function DownloadSection() {
  const settings = useSubscriptionSettings()
  const update = useUpdateSubscriptionSettings()
  const config = settings.data
  const disabled = settings.isLoading || settings.isError || update.isPending

  function save(patch: Partial<SubscriptionConfig>, preferences?: Partial<MagnetPreferences>) {
    if (!config) return
    update.mutate(
      { ...config, ...patch, preferences: { ...config.preferences, ...preferences } },
      {
        onSuccess: () => toast.success('下载设置已保存'),
        onError: error => {
          toast.error(describeApiError(error))
        }
      }
    )
  }

  return (
    <SettingsSection icon={<DownloadIcon className="size-4" />} title="下载">
      {settings.isPending ? <Skeleton className="h-4 w-40" /> : null}
      <SettingRow
        title="自动切换磁力"
        description="下载长时间没有进展时，取消当前下载并尝试下一条符合偏好的磁力"
        inline
      >
        <Switch
          checked={config?.download.auto_switch ?? true}
          disabled={disabled}
          onCheckedChange={checked => save({ download: { auto_switch: checked } })}
        />
      </SettingRow>

      <SettingRow
        title="字幕"
        description="自动切换磁力时按此挑选；「必须」时只选择符合字幕条件的磁力"
      >
        <PreferenceSelect
          value={config?.preferences.subtitle ?? 'preferred'}
          options={levelOptions}
          disabled={disabled}
          onChange={value => save({}, { subtitle: value })}
        />
      </SettingRow>

      <SettingRow title="高清" description="「必须」时只接受站方标注高清或名称含 4K 的磁力">
        <PreferenceSelect
          value={config?.preferences.hd ?? 'preferred'}
          options={levelOptions}
          disabled={disabled}
          onChange={value => save({}, { hd: value })}
        />
      </SettingRow>

      <SettingRow title="无码 / 破解" description="「排除」时过滤掉无码、破解、流出资源">
        <PreferenceSelect
          value={config?.preferences.uncensored ?? 'any'}
          options={uncensoredOptions}
          disabled={disabled}
          onChange={value => save({}, { uncensored: value })}
        />
      </SettingRow>

      {settings.isError ? (
        <InlineError onRetry={() => void settings.refetch()} retrying={settings.isFetching}>
          后端服务暂不可用，无法读取下载设置。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}
