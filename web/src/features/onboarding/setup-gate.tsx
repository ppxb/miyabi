import { type PropsWithChildren } from 'react'
import { LoaderCircleIcon } from 'lucide-react'

import { useSetupStatus } from '@/api/setup'
import { ErrorState } from '@/components/error-state'
import { OnboardingPage } from './onboarding-page'

export function SetupGate({ children }: PropsWithChildren) {
  const setup = useSetupStatus()

  if (setup.isPending) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-background text-sm text-muted-foreground">
        <LoaderCircleIcon className="mr-2 size-4 animate-spin" />
        正在准备 Miyabi
      </main>
    )
  }

  if (setup.isError || !setup.data) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-background px-4">
        <ErrorState
          message="无法读取初始化状态，请确认后端服务已启动。"
          onRetry={() => void setup.refetch()}
          retrying={setup.isFetching}
        />
      </main>
    )
  }

  if (setup.data.needs_setup) {
    return <OnboardingPage />
  }

  return children
}
