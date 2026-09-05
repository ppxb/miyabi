import { SearchIcon, XIcon } from 'lucide-react'
import { type FormEvent, useState } from 'react'

import {
  type BrowseMoviesParams,
  type DiscoverMovie,
  type JavDBZone,
  type TagCategory,
  useDiscoverMovies,
  useDiscoverTags,
  useSearchMovies
} from '@/api/discover'
import { EmptyState } from '@/components/empty-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGrid, MovieGridSkeleton } from '@/components/movie'
import { Button } from '@/components/ui/button'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput
} from '@/components/ui/input-group'
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

type DiscoverView = 'released' | 'upcoming' | 'category'

const PAGE_SIZE = 20

const zones: Array<{ value: JavDBZone; label: string }> = [
  { value: 'censored', label: '有码' },
  { value: 'uncensored', label: '无码' },
  { value: 'fc2', label: 'FC2' },
  { value: 'western', label: '欧美' }
]

// Taxonomy categories whose "tags" are filter mask fields rather than content tag ids.
const MAIN_CATEGORY = 'main'
const YEAR_CATEGORY = 'year'
// JavDB ignores month without a year and has no mask slot for duration, so neither works as a single filter.
const UNSUPPORTED_CATEGORIES = new Set(['month', 'duration'])

export function DiscoverContent() {
  const [zone, setZone] = useState<JavDBZone>('censored')
  const [view, setView] = useState<DiscoverView>('released')
  const [page, setPage] = useState(1)
  const [draftKeyword, setDraftKeyword] = useState('')
  const [keyword, setKeyword] = useState('')
  const [categoryID, setCategoryID] = useState('')
  const [tagID, setTagID] = useState('')

  const searching = keyword.length > 0
  const categoryMode = view === 'category' && !searching
  const taxonomy = useDiscoverTags(zone, categoryMode)
  const categories = (taxonomy.data ?? []).filter(
    category => !UNSUPPORTED_CATEGORIES.has(category.id)
  )
  const selectedCategory = categories.find(category => category.id === categoryID) ?? categories[0]

  const browse = useDiscoverMovies(browseParams(view, zone, selectedCategory?.id, tagID, page))
  const search = useSearchMovies({ query: keyword, zone, sort: 'release', page, limit: PAGE_SIZE })
  const activeQuery = searching ? search : browse

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const nextKeyword = draftKeyword.trim()
    setDraftKeyword(nextKeyword)
    setKeyword(nextKeyword)
    setPage(1)
  }

  function clearSearch() {
    setDraftKeyword('')
    setKeyword('')
    setPage(1)
  }

  function changeZone(value: string) {
    setZone(value as JavDBZone)
    setCategoryID('')
    setTagID('')
    setPage(1)
  }

  function changeView(value: string) {
    setView(value as DiscoverView)
    setPage(1)
  }

  return (
    <div className="min-w-0 space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row">
        <form className="min-w-0 flex-1" onSubmit={submitSearch}>
          <InputGroup className="h-10">
            <InputGroupAddon>
              <InputGroupButton type="submit" size="icon-xs" aria-label="搜索">
                <SearchIcon />
              </InputGroupButton>
            </InputGroupAddon>
            <InputGroupInput
              type="search"
              role="searchbox"
              inputMode="search"
              enterKeyHint="search"
              value={draftKeyword}
              placeholder="搜索番号、标题或演员"
              aria-label="搜索影片"
              onChange={event => setDraftKeyword(event.target.value)}
            />
            {draftKeyword ? (
              <InputGroupAddon align="inline-end">
                <InputGroupButton size="icon-xs" aria-label="清空搜索" onClick={clearSearch}>
                  <XIcon />
                </InputGroupButton>
              </InputGroupAddon>
            ) : null}
          </InputGroup>
        </form>

        <Select value={zone} onValueChange={changeZone}>
          <SelectTrigger className="h-10 w-full sm:w-32" aria-label="影片分区">
            <SelectValue />
          </SelectTrigger>
          <SelectContent position="popper" align="end">
            <SelectGroup>
              {zones.map(item => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>

      {searching ? (
        <div className="flex items-center justify-between gap-3">
          <h2 className="min-w-0 truncate text-xl font-semibold">“{keyword}” 的搜索结果</h2>
          <Button type="button" variant="outline" size="sm" onClick={clearSearch}>
            返回发现
          </Button>
        </div>
      ) : (
        <Tabs value={view} onValueChange={changeView}>
          <TabsList className="grid w-full grid-cols-3">
            <TabsTrigger value="released">最新发行</TabsTrigger>
            <TabsTrigger value="upcoming">即将发行</TabsTrigger>
            <TabsTrigger value="category">分类浏览</TabsTrigger>
          </TabsList>
        </Tabs>
      )}

      {categoryMode ? (
        <CategoryFilters
          loading={taxonomy.isLoading}
          error={taxonomy.error?.message}
          categories={categories}
          categoryID={selectedCategory?.id ?? ''}
          tagID={tagID}
          onCategoryChange={value => {
            setCategoryID(value)
            setTagID('')
            setPage(1)
          }}
          onTagChange={value => {
            setTagID(value === 'all' ? '' : value)
            setPage(1)
          }}
          onRetry={() => taxonomy.refetch()}
        />
      ) : null}

      <DiscoverResults
        movies={activeQuery.data}
        loading={activeQuery.isLoading}
        fetching={activeQuery.isFetching}
        error={activeQuery.isError}
        searching={searching}
        page={page}
        onPageChange={setPage}
        onRetry={() => activeQuery.refetch()}
      />
    </div>
  )
}

function browseParams(
  view: DiscoverView,
  zone: JavDBZone,
  categoryID: string | undefined,
  tagID: string,
  page: number
): BrowseMoviesParams {
  const base = { zone, page, limit: PAGE_SIZE }
  switch (view) {
    case 'released':
      // Show JavDB's latest magnet updates, with future release dates labeled on cards.
      return { ...base, main: ['m'], sort: 'update', order: 'desc' }
    case 'upcoming':
      return { ...base, sort: 'release', order: 'desc' }
    case 'category':
      return { ...base, sort: 'release', order: 'desc', ...tagFilter(categoryID, tagID) }
  }
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
  loading,
  error,
  categories,
  categoryID,
  tagID,
  onCategoryChange,
  onTagChange,
  onRetry
}: {
  loading: boolean
  error?: string
  categories: TagCategory[]
  categoryID: string
  tagID: string
  onCategoryChange: (value: string) => void
  onTagChange: (value: string) => void
  onRetry: () => void
}) {
  if (loading) {
    return (
      <div className="flex gap-2">
        <Skeleton className="h-9 w-36 rounded-full" />
        <Skeleton className="h-9 w-44 rounded-full" />
      </div>
    )
  }
  if (error) {
    return (
      <div className="flex items-center gap-3 text-sm text-destructive">
        <span>{error}</span>
        <Button type="button" variant="outline" size="sm" onClick={onRetry}>
          重试分类
        </Button>
      </div>
    )
  }

  const selectedCategory = categories.find(category => category.id === categoryID)
  return (
    <div className="flex flex-col gap-2 sm:flex-row">
      <Select value={categoryID} onValueChange={onCategoryChange}>
        <SelectTrigger className="w-full sm:w-48" aria-label="标签分类">
          <SelectValue placeholder="选择分类" />
        </SelectTrigger>
        <SelectContent position="popper" align="start">
          <SelectGroup>
            {categories.map(category => (
              <SelectItem key={category.id} value={category.id}>
                {category.name_zht}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>

      <Select value={tagID || 'all'} onValueChange={onTagChange}>
        <SelectTrigger className="w-full sm:w-56" aria-label="内容标签">
          <SelectValue placeholder="全部标签" />
        </SelectTrigger>
        <SelectContent position="popper" align="start">
          <SelectGroup>
            <SelectItem value="all">全部标签</SelectItem>
            {selectedCategory?.tags.map(tag => (
              <SelectItem key={tag.id} value={tag.id}>
                {tag.name_zht}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

function DiscoverResults({
  movies,
  loading,
  fetching,
  error,
  searching,
  page,
  onPageChange,
  onRetry
}: {
  movies: DiscoverMovie[] | undefined
  loading: boolean
  fetching: boolean
  error: boolean
  searching: boolean
  page: number
  onPageChange: (page: number) => void
  onRetry: () => void
}) {
  if (error) {
    return (
      <EmptyState
        emoji="Ò︵Ó"
        title="数据加载失败"
        actions={
          <Button type="button" variant="outline" size="sm" onClick={onRetry}>
            重试
          </Button>
        }
      />
    )
  }
  if (loading || !movies) return <MovieGridSkeleton count={8} />
  if (movies.length === 0 && page === 1) {
    return <EmptyState emoji="(･o･;)" title={searching ? '没有搜索结果' : '暂无内容'} />
  }

  return (
    <>
      <MovieGrid movies={movies} />
      <ListPagination
        page={page}
        hasMore={movies.length === PAGE_SIZE}
        disabled={fetching}
        onPageChange={onPageChange}
      />
    </>
  )
}
