import { LoaderCircleIcon } from 'lucide-react'
import { useEffect, useState, type FormEvent, type PropsWithChildren } from 'react'

import { useAccessGateConfig, useAccessGateLogin } from '@/api/auth'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

export function AccessGate({ children }: PropsWithChildren) {
  const config = useAccessGateConfig()
  const { refetch } = config
  const login = useAccessGateLogin()
  const [password, setPassword] = useState('')

  useEffect(() => {
    function handleUnauthorized() {
      void refetch()
    }

    window.addEventListener('miyabi:unauthorized', handleUnauthorized)
    return () => {
      window.removeEventListener('miyabi:unauthorized', handleUnauthorized)
    }
  }, [refetch])

  const isGranted = config.data?.enabled === false || config.data?.authenticated === true

  if (isGranted) return children

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    login.mutate(password, {
      onSuccess: () => {
        setPassword('')
      }
    })
  }

  return (
    <main className="flex min-h-dvh items-center justify-center bg-muted px-4 py-10">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <CardTitle>访问 Miyabi</CardTitle>
          <CardDescription>请输入部署时配置的访问密码</CardDescription>
        </CardHeader>
        <CardContent>
          {config.isPending ? (
            <div className="flex items-center justify-center gap-2 py-6 text-sm text-muted-foreground">
              <LoaderCircleIcon className="size-4 animate-spin" />
              正在检查访问配置
            </div>
          ) : config.isError ? (
            <InlineError
              className="flex-col text-center"
              onRetry={() => void config.refetch()}
              retrying={config.isFetching}
            >
              无法读取访问配置，请确认后端服务已启动。
            </InlineError>
          ) : (
            <form className="space-y-4" onSubmit={handleSubmit}>
              <Input
                type="password"
                value={password}
                placeholder="访问密码"
                autoComplete="current-password"
                autoFocus
                disabled={login.isPending}
                onChange={event => setPassword(event.target.value)}
              />
              {login.error ? <InlineError>{login.error.message}</InlineError> : null}
              <Button type="submit" className="w-full" disabled={!password || login.isPending}>
                {login.isPending ? <LoaderCircleIcon className="size-4 animate-spin" /> : null}
                进入
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </main>
  )
}
