import { createFileRoute, redirect } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { PageBackButton } from '@/components/page-back-button'
import { validateMetadataSearch } from '@/features/discover/metadata-search'
import { MetadataSearchPage } from '@/features/discover/metadata-search-page'

export const Route = createFileRoute('/discover_/search')({
  validateSearch: validateMetadataSearch,
  beforeLoad: ({ search, location }) => {
    if (search.kind !== 'tag' && Object.hasOwn(location.search, 'zone')) {
      throw redirect({ to: '/discover/search', search, replace: true })
    }
  },
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
      key={
        search.kind === 'tag' ? `tag:${search.id}:${search.zone}` : `${search.kind}:${search.id}`
      }
      search={search}
      onPageChange={page => void navigate({ search: previous => ({ ...previous, page }) })}
      onZoneChange={zone =>
        void navigate({
          search: previous => (previous.kind === 'tag' ? { ...previous, zone, page: 1 } : previous)
        })
      }
    />
  )
}
