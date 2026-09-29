import { createFileRoute } from '@tanstack/react-router'

import { SubscriptionsPage } from '@/features/subscriptions/page'
import { validateSubscriptionsSearch } from '@/features/subscriptions/search'
import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'

export const Route = createFileRoute('/subscriptions')({
  validateSearch: validateSubscriptionsSearch,
  component: SubscriptionsRoute,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="订阅页筛选条件无效" />
    </AppPage>
  )
})

function SubscriptionsRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <SubscriptionsPage
      key={`${search.view}:${search.actorID}:${search.page}:${search.actorPage}`}
      search={search}
      onSearchChange={next => void navigate({ search: next, resetScroll: false })}
    />
  )
}
