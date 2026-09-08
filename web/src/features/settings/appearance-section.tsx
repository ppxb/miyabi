import { MonitorCogIcon, MonitorIcon, MoonIcon, SunIcon } from 'lucide-react'
import { useTheme } from 'next-themes'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SettingRow, SettingsSection } from './shared'

const THEME_OPTIONS = [
  { value: 'system', label: '跟随系统', icon: MonitorIcon },
  { value: 'light', label: '浅色主题', icon: SunIcon },
  { value: 'dark', label: '深色主题', icon: MoonIcon }
]

export function AppearanceSection() {
  const { theme, setTheme } = useTheme()

  return (
    <SettingsSection icon={<MonitorCogIcon className="size-4" />} title="外观">
      <SettingRow title="主题" description="控制应用的明暗色主题" inline>
        <Tabs value={theme} onValueChange={setTheme}>
          <TabsList aria-label="主题模式">
            {THEME_OPTIONS.map(option => (
              <TabsTrigger key={option.value} value={option.value} aria-label={option.label}>
                <option.icon className="size-4" aria-hidden />
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </SettingRow>
    </SettingsSection>
  )
}
