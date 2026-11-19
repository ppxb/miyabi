import { createFileRoute } from '@tanstack/react-router'

import { ActorSourcesSection, ScrapingSection } from '@/features/settings/scraping-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/metadata')({
  component: SettingsMetadataPage
})

function SettingsMetadataPage() {
  return (
    <SettingsCard>
      <ScrapingSection />
      <ActorSourcesSection />
    </SettingsCard>
  )
}
