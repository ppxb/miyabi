import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getRouteApi } from '@tanstack/react-router'
import { categoryFromSearch, updateCategorySearch, type DiscoverView } from './search'
import { BrowseResults } from './browse-results'
import { CategoryContent } from './category-content'
import { DISCOVER_PAGE_SIZE as PAGE_SIZE } from './constants'

const route = getRouteApi('/discover')

export function DiscoverPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const view = search.view ?? 'released'
  const page = search[`${view}Page`] ?? 1
  const setPage = (view: DiscoverView, page: number) =>
    void navigate({
      search: previous => ({ ...previous, [`${view}Page`]: page > 1 ? page : undefined })
    })
  const setView = (view: DiscoverView) =>
    void navigate({
      search: previous => ({ ...previous, view: view === 'released' ? undefined : view })
    })

  return (
    <AppPage>
      <PageHeader title="发现" description="浏览最新发行、即将发行和分类内容" />
      <div className="min-w-0 space-y-6">
        <Tabs value={view} onValueChange={value => setView(value as DiscoverView)}>
          <TabsList>
            <TabsTrigger value="released">最新发行</TabsTrigger>
            <TabsTrigger value="upcoming">即将发行</TabsTrigger>
            <TabsTrigger value="category">分类浏览</TabsTrigger>
          </TabsList>
        </Tabs>

        {view === 'category' ? (
          <CategoryContent
            category={categoryFromSearch(search)}
            onCategoryChange={filters =>
              void navigate({ search: previous => updateCategorySearch(previous, filters) })
            }
            page={page}
            onPageChange={next => setPage('category', next)}
          />
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
