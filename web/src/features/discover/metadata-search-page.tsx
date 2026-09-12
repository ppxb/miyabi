import { useEffect } from 'react'

import { useDiscoverMovies, useDiscoverTags, type JavDBZone } from '@/api/discover'
import { AppPage } from '@/components/app-page'
import { InlineError } from '@/components/error-state'
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
import { Skeleton } from '@/components/ui/skeleton'
import { useDiscoverStore } from '@/stores/discover'
import { CommonFilterSelect } from './common-filter-select'
import { DISCOVER_PAGE_SIZE, DISCOVER_ZONES } from './constants'
import { metadataBrowseParams } from './metadata-params'
import { METADATA_LABELS, type MetadataSearch } from './metadata-search'
import { DiscoverResults } from './results'

export function MetadataSearchPage({
  search,
  onPageChange,
  onZoneChange,
  onMainChange
}: {
  search: MetadataSearch
  onPageChange: (page: number) => void
  onZoneChange: (zone: JavDBZone | undefined) => void
  onMainChange: (main: string) => void
}) {
  const categoryZone = useDiscoverStore(state => state.category.zone)
  const updateCategory = useDiscoverStore(state => state.updateCategory)
  const taxonomy = useDiscoverTags(
    search.kind === 'tag' ? (search.zone ?? categoryZone) : categoryZone
  )
  const mainOptions = taxonomy.data?.find(category => category.id === 'main')?.tags ?? []

  // Preserve the active common filter when opening another movie's metadata,
  // including when this search was reached through a bookmark or browser back.
  useEffect(() => {
    if (useDiscoverStore.getState().category.main !== search.main) {
      updateCategory({ main: search.main })
    }
  }, [search.main, updateCategory])

  const movies = useDiscoverMovies({
    page: search.page,
    limit: DISCOVER_PAGE_SIZE,
    sort: 'release',
    order: 'desc',
    ...metadataBrowseParams(search)
  })

  return (
    <AppPage>
      <PageBackButton />
      <PageHeader title={search.name} description={`${METADATA_LABELS[search.kind]}相关影片`}>
        {search.kind === 'tag' ? (
          <Select
            value={search.zone ?? 'all'}
            onValueChange={value =>
              onZoneChange(value === 'all' ? undefined : (value as JavDBZone))
            }
          >
            <SelectTrigger className="w-32" aria-label="影片分区">
              <SelectValue />
            </SelectTrigger>
            <SelectContent position="popper" align="end">
              <SelectGroup>
                <SelectItem value="all">全部分区</SelectItem>
                {DISCOVER_ZONES.map(zone => (
                  <SelectItem key={zone.value} value={zone.value}>
                    {zone.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        ) : null}
        {taxonomy.isPending ? (
          <Skeleton className="h-9 w-48 rounded-full" />
        ) : taxonomy.isError ? (
          <InlineError onRetry={() => void taxonomy.refetch()} retrying={taxonomy.isFetching}>
            通用筛选加载失败
          </InlineError>
        ) : mainOptions.length > 0 ? (
          <CommonFilterSelect
            options={mainOptions}
            value={search.main}
            onValueChange={onMainChange}
          />
        ) : null}
      </PageHeader>
      <DiscoverResults
        movies={movies.data}
        loading={movies.isPending}
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
