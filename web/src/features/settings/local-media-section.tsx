import { useState } from 'react'
import { FolderOpenIcon, LoaderCircleIcon } from 'lucide-react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import { useSetLocalDirectory, useSetupStatus } from '@/api/setup'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SettingRow, SettingsSection } from './shared'

export function LocalMediaSection() {
  const status = useSetupStatus()
  const save = useSetLocalDirectory()
  const [draft, setDraft] = useState<string | null>(null)
  const value = draft ?? status.data?.local_directory ?? ''

  function handleSave() {
    const path = value.trim()
    if (!path) {
      toast.error('请填写本地媒体目录')
      return
    }
    save.mutate(path, {
      onSuccess: () => toast.success('本地媒体目录已保存'),
      onError: error => toast.error(describeApiError(error))
    })
  }

  return (
    <SettingsSection icon={<FolderOpenIcon className="size-4" />} title="本地媒体">
      <SettingRow
        title="媒体目录"
        description="扫描本机已有的视频、STRM 和 NFO 文件；保存后可在媒体库中同步。"
      >
        <div className="flex w-full items-center gap-2 sm:w-96">
          <Input
            value={value}
            placeholder="D:\\Media 或 /media"
            disabled={status.isPending || save.isPending}
            onChange={event => setDraft(event.target.value)}
          />
          <Button
            type="button"
            size="sm"
            disabled={status.isPending || save.isPending || value.trim() === ''}
            onClick={handleSave}
          >
            {save.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
            保存
          </Button>
        </div>
      </SettingRow>
      {status.isError || save.isError ? (
        <InlineError
          onRetry={() => {
            if (status.isError) void status.refetch()
            else if (save.variables) save.mutate(save.variables)
          }}
          retrying={status.isFetching || save.isPending}
        >
          本地媒体目录读取或保存失败，请重试。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}
