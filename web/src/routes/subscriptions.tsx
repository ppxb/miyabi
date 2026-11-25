import { createFileRoute } from '@tanstack/react-router'

import { SubscriptionsPage } from '@/features/subscriptions/page'
import { validateSubscriptionsSearch } from '@/features/subscriptions/search'

export const Route = createFileRoute('/subscriptions')({
  validateSearch: validateSubscriptionsSearch,
  component: SubscriptionsRoute
})

function SubscriptionsRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <SubscriptionsPage
      search={search}
      onSearchChange={next => void navigate({ search: next, resetScroll: false })}
    />
  )
}
