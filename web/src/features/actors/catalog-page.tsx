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

const PAGE_SIZE = 40

function displayAvatar(src?: string) {
  if (!src) return undefined
  return src.startsWith('/api/actors/') ? src : imageURL(src)
}

export function ActorsCatalogPage() {
  const [query, setQuery] = useState('')
  const [debounced, setDebounced] = useState('')
  const [page, setPage] = useState(1)

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebounced(query)
      setPage(1)
    }, 300)
    return () => clearTimeout(timer)
  }, [query])

  const catalog = useActorCatalog({ q: debounced, page, limit: PAGE_SIZE })
  const items = catalog.data?.actors ?? []
  const totalPages = Math.max(1, Math.ceil((catalog.data?.total ?? 0) / PAGE_SIZE))

  return (
    <AppPage>
      <PageHeader title="演员" description="按演员浏览作品" />
      <Input
        className="w-full sm:w-60"
        placeholder="搜索演员名称"
        value={query}
        onChange={event => setQuery(event.target.value)}
      />
      {catalog.isPending ? (
        <p className="text-sm text-muted-foreground">正在读取演员</p>
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
                    src={displayAvatar(actor.avatar)}
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
            onPageChange={setPage}
          />
        </>
      )}
    </AppPage>
  )
}
