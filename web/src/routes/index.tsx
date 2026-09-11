import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import { LibraryPage } from '@/features/library/page'

export const Route = createFileRoute('/')({
  validateSearch: (search: Record<string, unknown>): { page?: number } => {
    const page = Number(search.page ?? 1)
    if (!Number.isInteger(page) || page < 1) throw new Error('媒体库页码无效')
    return page > 1 ? { page } : {}
  },
  component: LibraryRoute,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="媒体库页码无效" />
    </AppPage>
  )
})

function LibraryRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <LibraryPage
      page={search.page ?? 1}
      onPageChange={page => void navigate({ search: page > 1 ? { page } : {}, resetScroll: false })}
    />
  )
}
