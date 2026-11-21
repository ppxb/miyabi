import { createFileRoute } from '@tanstack/react-router'

import { DiscoverPage } from '@/features/discover/page'
import { validateDiscoverSearch } from '@/features/discover/search'

export const Route = createFileRoute('/discover')({
  validateSearch: validateDiscoverSearch,
  component: DiscoverPage
})
