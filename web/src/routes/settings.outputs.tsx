import { createFileRoute } from '@tanstack/react-router'

import { EmbySection } from '@/features/settings/emby-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/outputs')({
  component: SettingsOutputsPage
})

function SettingsOutputsPage() {
  return (
    <SettingsCard>
      <EmbySection />
    </SettingsCard>
  )
}
