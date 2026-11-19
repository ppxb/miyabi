import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import { LibraryPage } from '@/features/library/page'

export const Route = createFileRoute('/')({
  component: LibraryPage,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="媒体库加载失败" />
    </AppPage>
  )
})
