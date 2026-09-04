import { createRootRoute, Outlet } from "@tanstack/react-router"

import { AppShell } from "@/components/app-shell"

export const Route = createRootRoute({
  component: RootLayout,
})

function RootLayout() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}
