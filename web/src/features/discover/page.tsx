import {
  type BrowseMoviesParams,
  type JavDBZone,
  type TagCategory,
  useDiscoverMovies,
  useDiscoverTags
} from '@/api/discover'
import { AppPage } from '@/components/app-page'
import { InlineError } from '@/components/error-state'
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
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { type DiscoverView, useDiscoverStore } from '@/stores/discover'
import { categoryBrowseParams } from './category-params'
import { DISCOVER_PAGE_SIZE as PAGE_SIZE, DISCOVER_ZONES as zones } from './constants'
import { DiscoverResults } from './results'

const MAIN_CATEGORY = 'main'
// Month requires a year; duration has no slot in the upstream filter mask.
const UNSUPPORTED_CATEGORIES = new Set(['month', 'duration'])

export function DiscoverPage() {
  const view = useDiscoverStore(state => state.view)
  const pages = useDiscoverStore(state => state.pages)
  const setView = useDiscoverStore(state => state.setView)
  const setPage = useDiscoverStore(state => state.setPage)
  const page = pages[view]

  return (
    <AppPage>
      <PageHeader title="发现" description="浏览 JavDB 的最新发行、即将发行和分类内容" />
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
    </AppPage>
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
  const { zone, categoryID, tagID, main } = category
  const taxonomy = useDiscoverTags(zone)
  const mainOptions = taxonomy.data?.find(item => item.id === MAIN_CATEGORY)?.tags ?? []
  const categories = (taxonomy.data ?? []).filter(
    item => item.id !== MAIN_CATEGORY && !UNSUPPORTED_CATEGORIES.has(item.id)
  )
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
        mainOptions={mainOptions}
        main={main}
        onZoneChange={value => updateCategory({ zone: value, categoryID: '', tagID: '', main: '' })}
        onCategoryChange={value => updateCategory({ categoryID: value, tagID: '' })}
        onTagChange={value =>
          updateCategory({
            categoryID: selectedCategory?.id ?? categoryID,
            tagID: value === 'all' ? '' : value
          })
        }
        onMainChange={value => updateCategory({ main: value === 'all' ? '' : value })}
        onRetry={() => taxonomy.refetch()}
      />
      <BrowseResults
        params={{
          page,
          limit: PAGE_SIZE,
          sort: 'release',
          order: 'desc',
          ...categoryBrowseParams({ ...category, categoryID: selectedCategory?.id ?? '' })
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

function CategoryFilters({
  zone,
  loading,
  error,
  categories,
  categoryID,
  tagID,
  mainOptions,
  main,
  onZoneChange,
  onCategoryChange,
  onTagChange,
  onMainChange,
  onRetry
}: {
  zone: JavDBZone
  loading: boolean
  error: boolean
  categories: TagCategory[]
  categoryID: string
  tagID: string
  mainOptions: TagCategory['tags']
  main: string
  onZoneChange: (value: JavDBZone) => void
  onCategoryChange: (value: string) => void
  onTagChange: (value: string) => void
  onMainChange: (value: string) => void
  onRetry: () => void
}) {
  const selectedCategory = categories.find(category => category.id === categoryID)
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
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
          <Skeleton className="h-9 w-full rounded-full sm:w-48" />
        </>
      ) : error ? (
        <InlineError onRetry={onRetry} retryLabel="重试分类">
          分类加载失败
        </InlineError>
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
          {mainOptions.length > 0 ? (
            <Select value={main || 'all'} onValueChange={onMainChange}>
              <SelectTrigger className="w-full sm:w-48" aria-label="通用筛选">
                <SelectValue placeholder="全部条件" />
              </SelectTrigger>
              <SelectContent position="popper" align="start">
                <SelectGroup>
                  <SelectItem value="all">全部条件</SelectItem>
                  {mainOptions.map(option => (
                    <SelectItem key={option.id} value={option.id}>
                      {option.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          ) : null}
        </>
      )}
    </div>
  )
}
