import { createRootRoute, Outlet } from '@tanstack/react-router'

import { AppShell } from '@/components/app-shell'
import { AccessGate } from '@/features/access-gate/access-gate'

export const Route = createRootRoute({
  component: RootLayout
})

function RootLayout() {
  return (
    <AccessGate>
      <AppShell>
        <Outlet />
      </AppShell>
    </AccessGate>
  )
}
