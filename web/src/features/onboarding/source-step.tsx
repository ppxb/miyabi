import { CloudIcon, HardDriveIcon, LayersIcon } from 'lucide-react'

import { cn } from 'cn'
import { Button } from '@/components/ui/button'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { StepActions } from './step-actions'

export type SourceMode = 'local' | '115' | 'both'

const OPTIONS: Array<{
  value: SourceMode
  title: string
  description: string
  icon: typeof HardDriveIcon
}> = [
  {
    value: 'local',
    title: '本地媒体',
    description: '扫描本机已有的视频文件、STRM 和 NFO',
    icon: HardDriveIcon
  },
  {
    value: '115',
    title: '115 网盘',
    description: '扫码登录 115，挂载网盘中的媒体目录',
    icon: CloudIcon
  },
  {
    value: 'both',
    title: '本地 + 115',
    description: '同时使用本地媒体和 115 网盘',
    icon: LayersIcon
  }
]

export function SourceStep({
  mode,
  onChange,
  onNext,
  onBack
}: {
  mode: SourceMode | ''
  onChange: (mode: SourceMode) => void
  onNext: () => void
  onBack: () => void
}) {
  return (
    <div className="space-y-6">
      <RadioGroup
        value={mode}
        onValueChange={value => onChange(value as SourceMode)}
        className="gap-3"
      >
        {OPTIONS.map(option => {
          const Icon = option.icon
          const selected = mode === option.value
          return (
            <label
              key={option.value}
              htmlFor={`source-${option.value}`}
              className={cn(
                'flex cursor-pointer items-start gap-3 rounded-2xl border p-4 transition-colors',
                selected ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted/50'
              )}
            >
              <RadioGroupItem
                id={`source-${option.value}`}
                value={option.value}
                className="mt-0.5"
              />
              <span className="flex min-w-0 items-start gap-3">
                <Icon className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
                <span className="space-y-1">
                  <span className="block text-sm font-medium">{option.title}</span>
                  <span className="block text-xs leading-5 text-muted-foreground">
                    {option.description}
                  </span>
                </span>
              </span>
            </label>
          )
        })}
      </RadioGroup>
      <StepActions onBack={onBack}>
        <Button type="button" disabled={!mode} onClick={onNext}>
          继续
        </Button>
      </StepActions>
    </div>
  )
}
