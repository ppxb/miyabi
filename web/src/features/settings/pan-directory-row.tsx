import { FolderMinusIcon, FolderOpenIcon, LoaderCircleIcon } from 'lucide-react'

import { useClearPanDirectory, type PanDirectory } from '@/api/pan'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { PanDirectoryDialog } from './pan-directory-dialog'
import { SettingRow } from './shared'

export function PanDirectoryRow({
  accountID,
  directory,
  disabled
}: {
  accountID: string
  directory?: PanDirectory
  disabled: boolean
}) {
  const clear = useClearPanDirectory(accountID)
  const action = directory ? '更换媒体目录' : '挂载媒体目录'

  return (
    <>
      <SettingRow title="媒体目录" description="选择 115 中的目录作为媒体库来源" inline>
        <div className="flex items-center gap-2">
          <Tooltip>
            <PanDirectoryDialog accountID={accountID} directory={directory}>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  disabled={disabled || clear.isPending}
                >
                  <FolderOpenIcon className="size-4" />
                </Button>
              </TooltipTrigger>
            </PanDirectoryDialog>
            <TooltipContent>{action}</TooltipContent>
          </Tooltip>
          {directory ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  disabled={disabled || clear.isPending}
                  onClick={() => clear.mutate()}
                >
                  {clear.isPending ? (
                    <LoaderCircleIcon className="size-4 animate-spin" />
                  ) : (
                    <FolderMinusIcon className="size-4" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>取消挂载</TooltipContent>
            </Tooltip>
          ) : null}
        </div>
      </SettingRow>
      {clear.isError ? <InlineError>取消挂载未完成，请检查后端服务后重试。</InlineError> : null}
    </>
  )
}
