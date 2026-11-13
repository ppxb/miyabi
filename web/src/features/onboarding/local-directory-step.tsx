import { useState, type FormEvent } from 'react'
import { FolderOpenIcon, LoaderCircleIcon } from 'lucide-react'

import { useSetLocalDirectory } from '@/api/setup'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { StepActions } from './step-actions'

export function LocalDirectoryStep({
  initialPath,
  onNext,
  onBack
}: {
  initialPath: string
  onNext: () => void
  onBack: () => void
}) {
  const save = useSetLocalDirectory()
  const [path, setPath] = useState(initialPath)
  const [error, setError] = useState('')

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    if (!path.trim()) {
      setError('请填写本地媒体目录')
      return
    }
    save.mutate(path.trim(), {
      onSuccess: onNext,
      onError: err => setError(err.message)
    })
  }

  return (
    <form className="space-y-6" onSubmit={handleSubmit}>
      <Alert variant="info">
        <FolderOpenIcon />
        <AlertTitle>绑定本地媒体目录</AlertTitle>
        <AlertDescription>
          Docker 部署请填写容器内路径，例如 <code>/media</code>；桌面版可直接填写本机路径，例如{' '}
          <code>D:\Media</code>。
        </AlertDescription>
      </Alert>
      <div className="space-y-2">
        <Label htmlFor="local-directory">媒体目录路径</Label>
        <Input
          id="local-directory"
          value={path}
          autoFocus
          disabled={save.isPending}
          placeholder="D:\Media 或 /media"
          onChange={event => setPath(event.target.value)}
        />
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      <StepActions onBack={onBack}>
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
          保存并继续
        </Button>
      </StepActions>
    </form>
  )
}
