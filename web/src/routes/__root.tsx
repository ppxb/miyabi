import { createRootRoute, Outlet } from '@tanstack/react-router'

import { AppShell } from '@/components/app-shell'
import { AccessGate } from '@/features/access-gate/access-gate'
import { MovieDetailDialogProvider } from '@/features/movie-detail/dialog'
import { SetupGate } from '@/features/onboarding/setup-gate'

export const Route = createRootRoute({
  component: RootLayout
})

function RootLayout() {
  return (
    <AccessGate>
      <SetupGate>
        <AppShell>
          <MovieDetailDialogProvider>
            <Outlet />
          </MovieDetailDialogProvider>
        </AppShell>
      </SetupGate>
    </AccessGate>
  )
}
