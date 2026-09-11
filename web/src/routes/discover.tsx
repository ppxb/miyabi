import { createFileRoute } from '@tanstack/react-router'

import { DiscoverPage } from '@/features/discover/page'

export const Route = createFileRoute('/discover')({
  component: DiscoverPage
})
