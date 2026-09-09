import { LoaderCircleIcon } from 'lucide-react'
import { useState, type FormEvent, type PropsWithChildren } from 'react'

import { useAccessGateConfig, useAccessGateLogin } from '@/api/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

const ACCESS_GATE_KEY = 'miyabi-access-granted'

export function AccessGate({ children }: PropsWithChildren) {
  const [granted, setGranted] = useState(() => sessionStorage.getItem(ACCESS_GATE_KEY) === 'true')
  const [password, setPassword] = useState('')
  const config = useAccessGateConfig(!granted)
  const login = useAccessGateLogin()

  if (granted || config.data?.enabled === false) return children

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    login.mutate(password, {
      onSuccess: () => {
        sessionStorage.setItem(ACCESS_GATE_KEY, 'true')
        setPassword('')
        setGranted(true)
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
            <div className="space-y-4 text-center">
              <p className="text-sm text-muted-foreground">
                无法读取访问配置，请确认后端服务已启动。
              </p>
              <Button
                variant="outline"
                className="w-full cursor-pointer"
                disabled={config.isFetching}
                onClick={() => void config.refetch()}
              >
                {config.isFetching ? <LoaderCircleIcon className="size-4 animate-spin" /> : null}
                重试
              </Button>
            </div>
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
              {login.error ? (
                <p className="text-sm text-destructive">{login.error.message}</p>
              ) : null}
              <Button
                type="submit"
                className="w-full cursor-pointer"
                disabled={!password || login.isPending}
              >
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
