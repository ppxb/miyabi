import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import { SearchPage } from '@/features/search/page'

type SearchParams = { q?: string; page?: number }

export const Route = createFileRoute('/search')({
  validateSearch: (search: Record<string, unknown>): SearchParams => {
    const q = search.q ?? ''
    const page = Number(search.page ?? 1)
    if (typeof q !== 'string' || !Number.isInteger(page) || page < 1) {
      throw new Error('搜索条件无效')
    }
    const keyword = q.trim()
    return keyword ? { q: keyword, ...(page > 1 ? { page } : {}) } : {}
  },
  component: SearchRoute,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="搜索条件无效" />
    </AppPage>
  )
})

function SearchRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <SearchPage
      keyword={search.q ?? ''}
      page={search.page ?? 1}
      onSearch={q => void navigate({ search: q ? { q } : {} })}
      onPageChange={page =>
        void navigate({
          search: { q: search.q, ...(page > 1 ? { page } : {}) },
          resetScroll: false
        })
      }
    />
  )
}
