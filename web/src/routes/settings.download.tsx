import { createFileRoute } from '@tanstack/react-router'

import { DownloadSection } from '@/features/settings/download-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/download')({
  component: SettingsDownloadPage
})

function SettingsDownloadPage() {
  return (
    <SettingsCard>
      <DownloadSection />
    </SettingsCard>
  )
}
