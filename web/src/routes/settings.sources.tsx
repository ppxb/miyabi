import { createFileRoute } from '@tanstack/react-router'

import { SourcesSection } from '@/features/sources/sources-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/sources')({
  component: SettingsSourcesPage
})

function SettingsSourcesPage() {
  return (
    <SettingsCard>
      <SourcesSection />
    </SettingsCard>
  )
}
