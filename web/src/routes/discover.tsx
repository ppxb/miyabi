import { createFileRoute } from '@tanstack/react-router'

import { DiscoverPage } from '@/features/discover/page'
import { validateDiscoverSearch } from '@/features/discover/search'
import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'

export const Route = createFileRoute('/discover')({
  validateSearch: validateDiscoverSearch,
  component: DiscoverPage,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="发现页筛选条件无效" />
    </AppPage>
  )
})
