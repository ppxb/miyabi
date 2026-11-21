import { createRootRoute, Outlet } from '@tanstack/react-router'

import { AppShell } from '@/components/app-shell'
import { AccessGate } from '@/features/access-gate/access-gate'
import { MovieDetailDialogProvider } from '@/features/movie-detail/dialog'
import { ThemeSync } from '@/stores/theme'

export const Route = createRootRoute({
  component: RootLayout
})

function RootLayout() {
  return (
    <>
      <ThemeSync />
      <AccessGate>
        <MovieDetailDialogProvider>
          <AppShell>
            <Outlet />
          </AppShell>
        </MovieDetailDialogProvider>
      </AccessGate>
    </>
  )
}
