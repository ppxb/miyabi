import { createRouter } from '@tanstack/react-router'

import { routeTree } from '@/routeTree.gen'
import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  scrollRestoration: true,
  defaultErrorComponent: ({ error }) => (
    <AppPage>
      <ErrorState message={error instanceof Error ? error.message : '页面加载失败'} />
    </AppPage>
  )
})
