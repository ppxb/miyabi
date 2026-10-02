import { useState } from 'react'
import { FolderPlusIcon, LoaderCircleIcon, PlayIcon, Trash2Icon } from 'lucide-react'
import { toast } from 'sonner'

import {
  type LocalSource,
  useAddLocalSource,
  useLocalSources,
  useRemoveLocalSource,
  useScanLocalSource,
  useUpdateLocalSource
} from '@/api/local-sources'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { SettingRow, SettingsSection } from './shared'

export function LocalSourceSection() {
  const sources = useLocalSources()
  const add = useAddLocalSource()
  const [name, setName] = useState('')
  const [path, setPath] = useState('')

  function submit() {
    const trimmed = path.trim()
    if (!trimmed) {
      toast.error('请填写本地目录路径')
      return
    }
    add.mutate(
      { name: name.trim(), path: trimmed },
      {
        onSuccess: source => {
          setName('')
          setPath('')
          toast.success(`已添加「${source.name}」`, { description: '扫描后即可导入影片' })
        },
        onError: error => toast.error(error.message)
      }
    )
  }

  return (
    <SettingsSection icon={<FolderPlusIcon className="size-4" />} title="本地媒体目录">
      {sources.isPending ? <Skeleton className="h-9 w-full" /> : null}
      {sources.data?.map(source => (
        <LocalSourceRow key={source.id} source={source} />
      ))}
      {sources.data?.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          尚未添加本地媒体目录。Miyabi 会扫描其中的视频与 .strm 文件并参与刮削。
        </p>
      ) : null}

      <SettingRow
        title="添加目录"
        description="填写 Miyabi 容器内可见的路径，Docker 部署需与宿主机挂载一致"
      >
        <div className="flex flex-col gap-2 sm:flex-row">
          <Input
            value={name}
            placeholder="名称（可选）"
            disabled={add.isPending}
            onChange={event => setName(event.target.value)}
          />
          <Input
            value={path}
            placeholder="/media/inbox"
            disabled={add.isPending}
            onChange={event => setPath(event.target.value)}
            onKeyDown={event => {
              if (event.key === 'Enter') submit()
            }}
          />
          <Button type="button" size="sm" disabled={add.isPending} onClick={submit}>
            {add.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
            添加
          </Button>
        </div>
      </SettingRow>

      {sources.isError ? (
        <InlineError onRetry={() => void sources.refetch()} retrying={sources.isFetching}>
          无法读取本地媒体目录。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}

function LocalSourceRow({ source }: { source: LocalSource }) {
  const update = useUpdateLocalSource()
  const remove = useRemoveLocalSource()
  const scan = useScanLocalSource()
  const busy = update.isPending || remove.isPending || scan.isPending

  return (
    <SettingRow title={source.name} description={source.path} inline>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          // A disabled directory is skipped by every scan, so hide the action.
          disabled={busy || !source.enabled}
          onClick={() =>
            scan.mutate(source.id, {
              onSuccess: () => toast.success(`已开始扫描「${source.name}」`),
              onError: error => toast.error(error.message)
            })
          }
        >
          {scan.isPending ? <LoaderCircleIcon className="animate-spin" /> : <PlayIcon />}
          扫描
        </Button>
        <Switch
          checked={source.enabled}
          disabled={busy}
          onCheckedChange={enabled =>
            update.mutate(
              { id: source.id, enabled },
              { onError: error => toast.error(error.message) }
            )
          }
        />
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          title="删除"
          disabled={busy}
          onClick={() =>
            remove.mutate(source.id, {
              onSuccess: () => toast.success(`已删除「${source.name}」`),
              onError: error => toast.error(error.message)
            })
          }
        >
          {remove.isPending ? <LoaderCircleIcon className="animate-spin" /> : <Trash2Icon />}
        </Button>
      </div>
    </SettingRow>
  )
}
