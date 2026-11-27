import { Fragment, useEffect, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { SearchIcon, UserIcon } from 'lucide-react'
import { useScrollFade } from '@animate-ui/react/primitives/effects/scroll-fade'

import { apiGet, imageURL } from '@/api/client'
import type { ActorPage } from '@/api/actors'
import type { LibraryPage } from '@/api/library'
import { MediaImage } from '@/components/media-image'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator
} from '@/components/ui/command'
import { Kbd, KbdGroup } from '@/components/ui/kbd'

const MODIFIER_KEY =
  typeof navigator !== 'undefined' && /Mac|iPhone|iPad|iPod/.test(navigator.platform) ? '⌘' : 'Ctrl'

const RECENT_LIMIT = 12
// The server caps one page at 100; search walks every page instead of truncating results.
const SEARCH_PAGE_SIZE = 100

type SearchItem = {
  id: string
  group: string
  label: string
  badge?: string
  details?: string[]
  image?: string
  kind: 'movie' | 'actor'
  run: () => void
}

function groupBy(items: SearchItem[]) {
  const groups = [...new Set(items.map(item => item.group))]
  return groups.map(group => ({
    group,
    items: items.filter(item => item.group === group)
  }))
}

function useDebounced(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])
  return debounced
}

function movieDetails(movie: LibraryPage['movies'][number]) {
  return [
    [movie.release_date, movie.duration > 0 ? `${movie.duration} 分钟` : '']
      .filter(Boolean)
      .join(' / '),
    movie.actors
      .map(actor => actor.name)
      .filter(Boolean)
      .join(' / ')
  ].filter(Boolean)
}

/**
 * The result list is the scroll container, so the fade is attached to it. A component of its own
 * keeps the hook mounting together with the dialog: while the palette is closed the list is not in
 * the DOM and the fade would never attach.
 */
function FadingCommandList({ children }: { children: ReactNode }) {
  const listRef = useRef<HTMLDivElement>(null)

  useScrollFade(listRef, { blur: 4 })

  return (
    <CommandList
      ref={listRef}
      className="h-[min(30rem,calc(88vh-3.5rem))] max-h-none [&_[cmdk-list-sizer]]:h-full"
    >
      {children}
    </CommandList>
  )
}

/** Header trigger; the palette itself is mounted once by the shell and opened through props. */
export function SearchTrigger({ onOpen }: { onOpen: () => void }) {
  return (
    <Button onClick={onOpen} variant="secondary" className="font-normal text-muted-foreground">
      <SearchIcon data-icon="inline-start" />
      搜索
      <KbdGroup>
        <Kbd>{MODIFIER_KEY}</Kbd>
        <Kbd>J</Kbd>
      </KbdGroup>
    </Button>
  )
}

function ResultRow({ item }: { item: SearchItem }) {
  const isActor = item.kind === 'actor'
  return (
    <div className="flex w-full min-w-0 items-center gap-3">
      {isActor ? (
        <Avatar className="size-10 shrink-0">
          <AvatarImage src={item.image ? imageURL(item.image) : undefined} alt={item.label} />
          <AvatarFallback>
            <UserIcon className="size-4 text-muted-foreground" />
          </AvatarFallback>
        </Avatar>
      ) : (
        <div className="h-24 w-36 shrink-0 overflow-hidden rounded-md">
          <MediaImage source={item.image ?? ''} width={288} className="object-cover" />
        </div>
      )}
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium">{item.label}</span>
        {item.badge ? (
          <Badge variant="outline" className="tabular-nums">
            {item.badge}
          </Badge>
        ) : null}
        {item.details?.map((line, index) => (
          <span key={index} className="truncate text-xs text-muted-foreground">
            {line}
          </span>
        ))}
      </span>
    </div>
  )
}

async function fetchAllMovies(q: string, signal: AbortSignal) {
  const movies: LibraryPage['movies'] = []
  for (let page = 1; ; page++) {
    const data = await apiGet<LibraryPage>(
      '/api/library/browse',
      { q, page, limit: SEARCH_PAGE_SIZE },
      signal
    )
    movies.push(...data.movies)
    if (!data.has_more) return movies
  }
}

async function fetchAllActors(q: string, signal: AbortSignal) {
  const actors: ActorPage['actors'] = []
  for (let page = 1; ; page++) {
    const data = await apiGet<ActorPage>(
      '/api/actors',
      { q, page, limit: SEARCH_PAGE_SIZE },
      signal
    )
    actors.push(...data.actors)
    if (!data.has_more) return actors
  }
}

export function SearchDialog({
  open,
  onOpenChange,
  onPlay
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onPlay: (movieId: number) => void
}) {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const keyword = query.trim()
  const debounced = useDebounced(keyword, 200)
  const searching = keyword.length > 0
  const settled = searching && debounced === keyword

  const handleOpenChange = (value: boolean) => {
    onOpenChange(value)
    if (!value) setQuery('')
  }

  // Opening the palette shows recent additions; server queries only run while typing.
  const recent = useQuery({
    queryKey: ['header-search', 'recent'] as const,
    queryFn: ({ signal }) =>
      apiGet<LibraryPage>('/api/library/movies', { page: 1, limit: RECENT_LIMIT }, signal),
    enabled: open && !searching,
    staleTime: 30_000
  })

  const movies = useQuery({
    queryKey: ['header-search', 'movies', debounced] as const,
    queryFn: ({ signal }) => fetchAllMovies(debounced, signal),
    enabled: searching && debounced.length > 0,
    placeholderData: keepPreviousData,
    staleTime: 30_000
  })
  const actors = useQuery({
    queryKey: ['header-search', 'actors', debounced] as const,
    queryFn: ({ signal }) => fetchAllActors(debounced, signal),
    enabled: searching && debounced.length > 0,
    placeholderData: keepPreviousData,
    staleTime: 30_000
  })

  const movieItem = (movie: LibraryPage['movies'][number], group: string): SearchItem => {
    const hasTitle = Boolean(movie.title)
    return {
      id: `movie:${movie.id}`,
      group,
      label: hasTitle ? movie.title : movie.code,
      badge: hasTitle ? movie.code : undefined,
      details: movieDetails(movie),
      image: movie.cover || movie.poster,
      kind: 'movie',
      run: () => onPlay(movie.id)
    }
  }

  const items: SearchItem[] = []
  if (!searching) {
    for (const movie of recent.data?.movies ?? []) items.push(movieItem(movie, '最近加入'))
  } else {
    // Previous results stay on screen while the next keyword settles, so the list never flashes
    // an empty "searching" state mid-typing.
    for (const movie of movies.data ?? []) items.push(movieItem(movie, '影片'))
    for (const actor of actors.data ?? []) {
      items.push({
        id: `actor:${actor.key}`,
        group: '演员',
        label: actor.name,
        details: actor.movie_count > 0 ? [`${actor.movie_count} 部作品`] : undefined,
        image: actor.avatar,
        kind: 'actor',
        run: () => {
          void navigate({ to: '/actors/$key', params: { key: actor.key } })
        }
      })
    }
  }

  const loading = searching ? movies.isPending && actors.isPending : open && recent.isPending
  const failed = searching && settled && (movies.isError || actors.isError)
  const recentFailed = !searching && recent.isError
  const groupedItems = groupBy(items)
  const emptyMessage = loading
    ? searching
      ? '搜索中'
      : '加载中'
    : searching
      ? failed
        ? '搜索失败，请检查后端服务'
        : '没有找到结果'
      : recentFailed
        ? '无法加载最近加入，请检查后端服务'
        : '无影片'

  const handleSelect = (item: SearchItem) => {
    handleOpenChange(false)
    item.run()
  }

  return (
    <CommandDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="搜索"
      description="搜索本地影片与演员"
      className="top-1/2 max-h-[90vh] -translate-y-1/2 sm:max-w-2xl"
    >
      <Command shouldFilter={false} vimBindings={false}>
        <CommandInput
          placeholder="输入番号、影片名或演员进行搜索"
          value={query}
          onValueChange={setQuery}
        />
        <FadingCommandList>
          {failed && items.length > 0 ? (
            <p className="px-2 py-1.5 text-xs text-destructive">部分结果加载失败，请检查后端服务</p>
          ) : null}
          {items.length === 0 ? (
            <CommandEmpty className="flex h-full items-center justify-center">
              {emptyMessage}
            </CommandEmpty>
          ) : null}
          {groupedItems.map(({ group, items: groupItems }, index) => (
            <Fragment key={group}>
              {index > 0 && <CommandSeparator />}
              <CommandGroup heading={group}>
                {groupItems.map(item => (
                  <CommandItem key={item.id} value={item.id} onSelect={() => handleSelect(item)}>
                    <ResultRow item={item} />
                  </CommandItem>
                ))}
              </CommandGroup>
            </Fragment>
          ))}
        </FadingCommandList>
      </Command>
    </CommandDialog>
  )
}
