import { SearchIcon, XIcon } from 'lucide-react'
import { type FormEvent, useMemo, useState } from 'react'

import {
  type DiscoverMovie,
  type JavDBZone,
  type TagCategory,
  useDiscoverMovies,
  useDiscoverTags,
  useSearchMovies
} from '@/api/discover'
import { EmptyState } from '@/components/empty-state'
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

const zones: Array<{ value: JavDBZone; label: string }> = [
  { value: 'censored', label: '有码' },
  { value: 'uncensored', label: '无码' },
  { value: 'fc2', label: 'FC2' },
  { value: 'western', label: '欧美' }
]

export function DiscoverContent() {
  const [zone, setZone] = useState<JavDBZone>('censored')
  const [view, setView] = useState<DiscoverView>('released')
  const [draftKeyword, setDraftKeyword] = useState('')
  const [keyword, setKeyword] = useState('')
  const [categoryID, setCategoryID] = useState('')
  const [tagID, setTagID] = useState('')

  const categoryMode = view === 'category' && keyword.length === 0
  const browse = useDiscoverMovies({
    zone,
    tagIds: categoryMode && tagID ? [tagID] : undefined,
    sort: 'release',
    order: 'desc',
    limit: 40
  })
  const search = useSearchMovies({ query: keyword, zone, sort: 'release', limit: 40 })
  const taxonomy = useDiscoverTags(zone, categoryMode)
  const categories = taxonomy.data ?? []
  const selectedCategory = categories.find(category => category.id === categoryID) ?? categories[0]
  const activeQuery = keyword ? search : browse
  const movies = useMemo(
    () => filterMovies(activeQuery.data ?? [], keyword, view),
    [activeQuery.data, keyword, view]
  )

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const nextKeyword = draftKeyword.trim()
    setDraftKeyword(nextKeyword)
    setKeyword(nextKeyword)
  }

  function clearSearch() {
    setDraftKeyword('')
    setKeyword('')
  }

  function changeZone(value: string) {
    setZone(value as JavDBZone)
    setCategoryID('')
    setTagID('')
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

      {keyword ? (
        <div className="flex items-center justify-between gap-3">
          <h2 className="min-w-0 truncate text-xl font-semibold">“{keyword}” 的搜索结果</h2>
          <Button type="button" variant="outline" size="sm" onClick={clearSearch}>
            返回发现
          </Button>
        </div>
      ) : (
        <Tabs value={view} onValueChange={value => setView(value as DiscoverView)}>
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
          }}
          onTagChange={value => setTagID(value === 'all' ? '' : value)}
          onRetry={() => taxonomy.refetch()}
        />
      ) : null}

      <DiscoverResults
        movies={movies}
        loading={activeQuery.isLoading}
        error={activeQuery.isError}
        searching={keyword.length > 0}
        onRetry={() => activeQuery.refetch()}
      />
    </div>
  )
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
                {category.name_zht || category.name}
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
                {tag.name_zht || tag.name}
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
  error,
  searching,
  onRetry
}: {
  movies: DiscoverMovie[]
  loading: boolean
  error: boolean
  searching: boolean
  onRetry: () => void
}) {
  if (loading) return <MovieGridSkeleton count={8} />
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
  if (movies.length === 0) {
    return <EmptyState emoji="(･o･;)" title={searching ? '没有搜索结果' : '暂无内容'} />
  }
  return <MovieGrid movies={movies} />
}

function filterMovies(movies: DiscoverMovie[], keyword: string, view: DiscoverView) {
  if (keyword || view === 'category') return movies
  return movies.filter(movie =>
    view === 'upcoming' ? movie.release_status === 'upcoming' : movie.release_status === 'released'
  )
}
