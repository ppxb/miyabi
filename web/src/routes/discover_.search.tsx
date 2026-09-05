import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { PageBackButton } from '@/components/page-back-button'
import { validateMetadataSearch } from '@/features/discover/metadata-search'
import { MetadataSearchPage } from '@/features/discover/metadata-search-page'

export const Route = createFileRoute('/discover_/search')({
  validateSearch: validateMetadataSearch,
  component: MetadataSearchRoute,
  errorComponent: () => (
    <AppPage>
      <PageBackButton />
      <EmptyState emoji="(･o･;)" title="搜索条件无效" />
    </AppPage>
  )
})

function MetadataSearchRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <MetadataSearchPage
      search={search}
      onPageChange={page => void navigate({ search: previous => ({ ...previous, page }) })}
      onZoneChange={zone => void navigate({ search: previous => ({ ...previous, zone, page: 1 }) })}
    />
  )
}
