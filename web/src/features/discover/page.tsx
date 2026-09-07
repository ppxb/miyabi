import {
  type BrowseMoviesParams,
  type JavDBZone,
  type TagCategory,
  useDiscoverMovies,
  useDiscoverTags
} from '@/api/discover'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { type DiscoverView, useDiscoverStore } from '@/stores/discover'
import { DISCOVER_PAGE_SIZE as PAGE_SIZE, DISCOVER_ZONES as zones } from './constants'
import { DiscoverResults } from './results'

const MAIN_CATEGORY = 'main'
const YEAR_CATEGORY = 'year'
// Month requires a year; duration has no slot in the upstream filter mask.
const UNSUPPORTED_CATEGORIES = new Set(['month', 'duration'])

export function DiscoverContent() {
  const view = useDiscoverStore(state => state.view)
  const pages = useDiscoverStore(state => state.pages)
  const setView = useDiscoverStore(state => state.setView)
  const setPage = useDiscoverStore(state => state.setPage)
  const page = pages[view]

  return (
    <div className="min-w-0 space-y-6">
      <Tabs value={view} onValueChange={value => setView(value as DiscoverView)}>
        <TabsList>
          <TabsTrigger value="released">最新</TabsTrigger>
          <TabsTrigger value="upcoming">即将发行</TabsTrigger>
          <TabsTrigger value="category">分类浏览</TabsTrigger>
        </TabsList>
      </Tabs>

      {view === 'category' ? (
        <CategoryContent page={page} onPageChange={next => setPage('category', next)} />
      ) : (
        <BrowseResults
          key={view}
          params={{
            page,
            limit: PAGE_SIZE,
            order: 'desc',
            ...(view === 'released' ? { main: ['m'], sort: 'update' } : { sort: 'release' })
          }}
          onPageChange={next => setPage(view, next)}
        />
      )}
    </div>
  )
}

function CategoryContent({
  page,
  onPageChange
}: {
  page: number
  onPageChange: (page: number) => void
}) {
  const category = useDiscoverStore(state => state.category)
  const updateCategory = useDiscoverStore(state => state.updateCategory)
  const { zone, categoryID, tagID } = category
  const taxonomy = useDiscoverTags(zone)
  const categories = (taxonomy.data ?? []).filter(item => !UNSUPPORTED_CATEGORIES.has(item.id))
  const selectedCategory = categories.find(item => item.id === categoryID) ?? categories[0]

  return (
    <div className="space-y-6">
      <CategoryFilters
        zone={zone}
        loading={taxonomy.isLoading}
        error={taxonomy.isError}
        categories={categories}
        categoryID={selectedCategory?.id ?? ''}
        tagID={tagID}
        onZoneChange={value => updateCategory({ zone: value, categoryID: '', tagID: '' })}
        onCategoryChange={value => updateCategory({ categoryID: value, tagID: '' })}
        onTagChange={value =>
          updateCategory({
            categoryID: selectedCategory?.id ?? categoryID,
            tagID: value === 'all' ? '' : value
          })
        }
        onRetry={() => taxonomy.refetch()}
      />
      <BrowseResults
        key={JSON.stringify(category)}
        params={{
          zone,
          page,
          limit: PAGE_SIZE,
          sort: 'release',
          order: 'desc',
          ...tagFilter(categoryID || selectedCategory?.id, tagID)
        }}
        onPageChange={onPageChange}
      />
    </div>
  )
}

function BrowseResults({
  params,
  onPageChange
}: {
  params: BrowseMoviesParams & { page: number }
  onPageChange: (page: number) => void
}) {
  const movies = useDiscoverMovies(params)
  return (
    <DiscoverResults
      movies={movies.data}
      loading={movies.isPending}
      fetching={movies.isFetching}
      error={movies.isError}
      searching={false}
      page={params.page}
      onPageChange={onPageChange}
      onRetry={() => movies.refetch()}
    />
  )
}

function tagFilter(categoryID: string | undefined, tagID: string): BrowseMoviesParams {
  if (!tagID) return {}
  switch (categoryID) {
    case MAIN_CATEGORY:
      return { main: [tagID] }
    case YEAR_CATEGORY:
      return { year: tagID }
    default:
      return { tagIds: [tagID] }
  }
}

function CategoryFilters({
  zone,
  loading,
  error,
  categories,
  categoryID,
  tagID,
  onZoneChange,
  onCategoryChange,
  onTagChange,
  onRetry
}: {
  zone: JavDBZone
  loading: boolean
  error: boolean
  categories: TagCategory[]
  categoryID: string
  tagID: string
  onZoneChange: (value: JavDBZone) => void
  onCategoryChange: (value: string) => void
  onTagChange: (value: string) => void
  onRetry: () => void
}) {
  const selectedCategory = categories.find(category => category.id === categoryID)
  return (
    <div className="flex flex-col gap-2 sm:flex-row">
      <Select value={zone} onValueChange={value => onZoneChange(value as JavDBZone)}>
        <SelectTrigger className="w-full sm:w-32">
          <SelectValue />
        </SelectTrigger>
        <SelectContent position="popper" align="start">
          <SelectGroup>
            {zones.map(item => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>

      {loading ? (
        <>
          <Skeleton className="h-9 w-full rounded-full sm:w-48" />
          <Skeleton className="h-9 w-full rounded-full sm:w-56" />
        </>
      ) : error ? (
        <div className="flex items-center gap-3 text-sm text-destructive">
          <span>分类加载失败</span>
          <Button type="button" variant="outline" size="sm" onClick={onRetry}>
            重试分类
          </Button>
        </div>
      ) : (
        <>
          <Select value={categoryID} onValueChange={onCategoryChange}>
            <SelectTrigger className="w-full sm:w-48">
              <SelectValue placeholder="选择分类" />
            </SelectTrigger>
            <SelectContent position="popper" align="start">
              <SelectGroup>
                {categories.map(category => (
                  <SelectItem key={category.id} value={category.id}>
                    {category.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Select value={tagID || 'all'} onValueChange={onTagChange}>
            <SelectTrigger className="w-full sm:w-56">
              <SelectValue placeholder="全部标签" />
            </SelectTrigger>
            <SelectContent position="popper" align="start">
              <SelectGroup>
                <SelectItem value="all">全部标签</SelectItem>
                {selectedCategory?.tags.map(tag => (
                  <SelectItem key={tag.id} value={tag.id}>
                    {tag.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </>
      )}
    </div>
  )
}
