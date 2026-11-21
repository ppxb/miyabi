import { Fragment, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { SearchIcon, UserIcon, type LucideIcon } from 'lucide-react'
import { useScrollFade } from '@animate-ui/react/primitives/effects/scroll-fade'

import { apiGet, imageURL } from '@/api/client'
import type { ActorPage } from '@/api/actors'
import type { LibraryPage } from '@/api/library'
import { APP_NAV_ITEMS } from '@/components/app-navigation'
import { MediaImage } from '@/components/media-image'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
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
import { MovieDetailDialogContext } from '@/features/movie-detail/dialog-context'

const MODIFIER_KEY =
  typeof navigator !== 'undefined' && /Mac|iPhone|iPad|iPod/.test(navigator.platform) ? '⌘' : 'Ctrl'

const RESULT_LIMIT = 6

type SearchItem = {
  id: string
  group: string
  label: string
  details?: string[]
  image?: string
  kind: 'page' | 'movie' | 'actor'
  icon?: LucideIcon
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

function movieDetails(movie: LibraryPage['movies'][number], hasTitle: boolean) {
  return [
    hasTitle ? movie.code : '',
    movie.release_date ?? '',
    movie.duration > 0 ? `${movie.duration} 分钟` : '',
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
    <CommandList ref={listRef} className="h-[min(30rem,calc(88vh-3.5rem))] max-h-none">
      {children}
    </CommandList>
  )
}

/** Header trigger; the palette itself is mounted once by the shell and opened through props. */
export function SearchTrigger({ onOpen }: { onOpen: () => void }) {
  return (
    <Button onClick={onOpen} variant="ghost" className="font-normal text-muted-foreground">
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
  if (item.kind === 'page') {
    return (
      <span className="flex min-w-0 items-center gap-2">
        {item.icon && <item.icon />}
        <span className="truncate">{item.label}</span>
      </span>
    )
  }
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
          <MediaImage source={item.image ?? ''} className="object-cover" />
        </div>
      )}
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium">{item.label}</span>
        {item.details?.map((line, index) => (
          <span key={index} className="truncate text-xs text-muted-foreground">
            {line}
          </span>
        ))}
      </span>
    </div>
  )
}

export function SearchDialog({
  open,
  onOpenChange
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const navigate = useNavigate()
  const openMovie = useContext(MovieDetailDialogContext)
  const [query, setQuery] = useState('')
  const keyword = query.trim()
  const debounced = useDebounced(keyword, 200)

  const handleOpenChange = (value: boolean) => {
    onOpenChange(value)
    if (!value) setQuery('')
  }

  const movies = useQuery({
    queryKey: ['header-search', 'movies', debounced] as const,
    queryFn: ({ signal }) =>
      apiGet<LibraryPage>('/api/library/movies', { q: debounced, limit: RESULT_LIMIT }, signal),
    enabled: debounced.length > 0,
    staleTime: 30_000
  })
  const actors = useQuery({
    queryKey: ['header-search', 'actors', debounced] as const,
    queryFn: ({ signal }) =>
      apiGet<ActorPage>('/api/actors', { q: debounced, page: 1, limit: RESULT_LIMIT }, signal),
    enabled: debounced.length > 0,
    staleTime: 30_000
  })

  const items: SearchItem[] = APP_NAV_ITEMS.filter(
    item => !keyword || item.label.toLowerCase().includes(keyword.toLowerCase())
  ).map(item => ({
    id: item.to,
    group: '页面',
    label: item.label,
    kind: 'page',
    icon: item.icon,
    run: () => {
      void navigate({ to: item.to })
    }
  }))

  // Only render server results once the debounced term matches the input, and
  // never after the input is cleared — this avoids stale rows popping in/out.
  const settled = keyword.length > 0 && debounced === keyword
  if (settled) {
    for (const movie of movies.data?.movies ?? []) {
      const hasTitle = Boolean(movie.title)
      items.push({
        id: `movie:${movie.id}`,
        group: '影片',
        label: hasTitle ? movie.title : movie.code,
        details: movieDetails(movie, hasTitle),
        image: movie.cover || movie.poster,
        kind: 'movie',
        run: () => {
          openMovie?.({ libraryId: movie.id })
        }
      })
    }
    for (const actor of actors.data?.actors ?? []) {
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

  const loading = keyword.length > 0 && (!settled || movies.isPending || actors.isPending)
  const groupedItems = groupBy(items)

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
      className="top-1/2 max-h-[90vh] -translate-y-1/2 sm:max-w-3xl"
    >
      <Command shouldFilter={false}>
        <CommandInput
          placeholder="输入番号、影片名或演员进行搜索"
          value={query}
          onValueChange={setQuery}
        />
        <FadingCommandList>
          {items.length === 0 ? (
            <CommandEmpty>{loading ? '搜索中' : '没有找到结果'}</CommandEmpty>
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
