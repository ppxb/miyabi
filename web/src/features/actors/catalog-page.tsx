import { Link } from '@tanstack/react-router'
import { useEffect, useState } from 'react'

import { useActorCatalog } from '@/api/actors'
import { imageURL } from '@/api/client'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { PageHeader } from '@/components/page-header'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'

const PAGE_SIZE = 40

export function ActorsCatalogPage({
  q,
  page,
  onSearchChange
}: {
  q?: string
  page: number
  onSearchChange: (next: { q?: string; page?: number }) => void
}) {
  const [query, setQuery] = useState(q ?? '')
  const [syncedQ, setSyncedQ] = useState(q ?? '')

  // Reflect external URL changes (back/forward) into the input, but never erase
  // what the user is still typing: skip the write when the URL value merely
  // echoes our own trimmed submission ("abc " -> "abc").
  const externalQ = q ?? ''
  if (externalQ !== syncedQ) {
    setSyncedQ(externalQ)
    if (externalQ !== query.trim()) {
      setQuery(externalQ)
    }
  }

  useEffect(() => {
    const current = q ?? ''
    if (query.trim() === current) return
    const timer = setTimeout(() => {
      onSearchChange({ q: query.trim() || undefined, page: undefined })
    }, 300)
    return () => clearTimeout(timer)
  }, [query, q, onSearchChange])

  const catalog = useActorCatalog({ q: q ?? '', page, limit: PAGE_SIZE })
  const items = catalog.data?.actors ?? []
  const totalPages = Math.max(1, Math.ceil((catalog.data?.total ?? 0) / PAGE_SIZE))

  return (
    <AppPage>
      <PageHeader title="演员" description="按演员浏览作品" />
      <Input
        aria-label="搜索演员名称"
        className="w-full sm:w-60"
        placeholder="搜索演员名称"
        value={query}
        onChange={event => setQuery(event.target.value)}
      />
      {catalog.isPending ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
          {Array.from({ length: 10 }).map((_, index) => (
            <div key={index} className="flex flex-col items-center gap-2 p-3">
              <Skeleton className="size-32 rounded-full" />
              <Skeleton className="h-4 w-20" />
              <Skeleton className="h-3 w-12" />
            </div>
          ))}
        </div>
      ) : catalog.isError ? (
        <ErrorState
          message="无法读取演员，请检查后端服务后重试"
          onRetry={() => void catalog.refetch()}
          retrying={catalog.isFetching}
        />
      ) : items.length === 0 ? (
        <EmptyState title="没有匹配的演员" />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
            {items.map(actor => (
              <Link
                key={actor.key}
                to="/actors/$key"
                params={{ key: actor.key }}
                className="group flex flex-col items-center gap-2 rounded-2xl p-3 text-center transition-colors hover:bg-muted"
              >
                <Avatar className="size-32">
                  <AvatarImage
                    src={actor.avatar ? imageURL(actor.avatar) : undefined}
                    alt={actor.name}
                    loading="eager"
                  />
                  <AvatarFallback>{actor.name.slice(0, 1)}</AvatarFallback>
                </Avatar>
                <span className="line-clamp-1 text-sm font-medium">{actor.name}</span>
                <span className="text-xs text-muted-foreground">{actor.movie_count} 部</span>
              </Link>
            ))}
          </div>
          <ListPagination
            page={page}
            totalPages={totalPages}
            hasMore={catalog.data?.has_more ?? false}
            disabled={catalog.isFetching}
            onPageChange={next =>
              onSearchChange({ q: q?.trim() || undefined, page: next > 1 ? next : undefined })
            }
          />
        </>
      )}
    </AppPage>
  )
}
