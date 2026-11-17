import { createFileRoute } from '@tanstack/react-router'

import { Separator } from '@/components/ui/separator'
import { DataSection } from '@/features/settings/data-section'
import { TasksSection } from '@/features/settings/tasks-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/data')({
  component: SettingsDataPage
})

function SettingsDataPage() {
  return (
    <SettingsCard>
      <TasksSection />
      <Separator />
      <DataSection />
    </SettingsCard>
  )
}
