import { createFileRoute } from '@tanstack/react-router'

import { Separator } from '@/components/ui/separator'
import { PanSection } from '@/features/settings/pan-section'
import { SourcesSection } from '@/features/sources/sources-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/sources')({
  component: SettingsSourcesPage
})

function SettingsSourcesPage() {
  return (
    <SettingsCard>
      <SourcesSection />
      <Separator />
      <PanSection />
    </SettingsCard>
  )
}
