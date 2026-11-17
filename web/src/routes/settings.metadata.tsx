import { createFileRoute } from '@tanstack/react-router'

import { Separator } from '@/components/ui/separator'
import { JavDBSection } from '@/features/settings/javdb-section'
import { ScrapingSection } from '@/features/settings/scraping-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/metadata')({
  component: SettingsMetadataPage
})

function SettingsMetadataPage() {
  return (
    <SettingsCard>
      <ScrapingSection />
      <Separator />
      <JavDBSection />
    </SettingsCard>
  )
}
