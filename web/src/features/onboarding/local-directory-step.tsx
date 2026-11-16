import { useState, type FormEvent } from 'react'
import { FolderOpenIcon } from 'lucide-react'

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
  onNext: (path: string) => void
  onBack: () => void
}) {
  const [path, setPath] = useState(initialPath)
  const [error, setError] = useState('')

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const value = path.trim()
    if (!value) {
      setError('请填写本地媒体目录')
      return
    }
    setError('')
    onNext(value)
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
          placeholder="D:\Media 或 /media"
          onChange={event => setPath(event.target.value)}
        />
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      <StepActions onBack={onBack}>
        <Button type="submit">继续</Button>
      </StepActions>
    </form>
  )
}
