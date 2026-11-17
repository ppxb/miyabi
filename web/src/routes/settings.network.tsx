import { createFileRoute } from '@tanstack/react-router'

import { NetworkSection } from '@/features/settings/network-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/network')({
  component: SettingsNetworkPage
})

function SettingsNetworkPage() {
  return (
    <SettingsCard>
      <NetworkSection />
    </SettingsCard>
  )
}
