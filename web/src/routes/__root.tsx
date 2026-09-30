import { createRootRoute, Outlet } from '@tanstack/react-router'

import { AppShell } from '@/components/app-shell'
import { AccessGate } from '@/features/access-gate/access-gate'
import { MovieDetailDialogProvider } from '@/features/movie-detail/dialog'

export const Route = createRootRoute({
  component: RootLayout
})

function RootLayout() {
  return (
    <AccessGate>
      <AppShell>
        <MovieDetailDialogProvider>
          <Outlet />
        </MovieDetailDialogProvider>
      </AppShell>
    </AccessGate>
  )
}
