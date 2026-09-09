import { useDiscoverMovies, type JavDBZone } from '@/api/discover'
import { AppPage } from '@/components/app-page'
import { PageBackButton } from '@/components/page-back-button'
import { PageHeader } from '@/components/page-header'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { DISCOVER_PAGE_SIZE, DISCOVER_ZONES } from './constants'
import { METADATA_LABELS, type MetadataSearch } from './metadata-search'
import { DiscoverResults } from './results'

export function MetadataSearchPage({
  search,
  onPageChange,
  onZoneChange
}: {
  search: MetadataSearch
  onPageChange: (page: number) => void
  onZoneChange: (zone: JavDBZone) => void
}) {
  const movies = useDiscoverMovies({
    page: search.page,
    limit: DISCOVER_PAGE_SIZE,
    sort: 'release',
    order: 'desc',
    ...(search.kind === 'tag'
      ? { zone: search.zone, tagIds: [search.id] }
      : { entityType: search.kind, entityID: search.id })
  })

  return (
    <AppPage>
      <PageBackButton />
      <PageHeader title={search.name} description={`${METADATA_LABELS[search.kind]}相关影片`}>
        {search.kind === 'tag' ? (
          <Select value={search.zone} onValueChange={value => onZoneChange(value as JavDBZone)}>
            <SelectTrigger className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent position="popper" align="end">
              <SelectGroup>
                {DISCOVER_ZONES.map(zone => (
                  <SelectItem key={zone.value} value={zone.value}>
                    {zone.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        ) : null}
      </PageHeader>
      <DiscoverResults
        movies={movies.data}
        loading={movies.isPending || movies.isPlaceholderData}
        fetching={movies.isFetching}
        error={movies.isError}
        searching
        page={search.page}
        onPageChange={onPageChange}
        onRetry={() => movies.refetch()}
      />
    </AppPage>
  )
}
