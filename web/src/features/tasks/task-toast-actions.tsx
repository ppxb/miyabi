import { ArrowUpRightIcon, PlayIcon, XIcon, type LucideIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function TaskToastActions({
  id,
  onView,
  onPlay
}: {
  id: string
  onView?: () => void
  onPlay?: () => void
}) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const actions: { label: string; icon: LucideIcon; onClick?: () => void }[] = [
    ...(onPlay
      ? [{ label: '播放', icon: PlayIcon, onClick: onPlay }]
      : onView
        ? [{ label: '查看', icon: ArrowUpRightIcon, onClick: onView }]
        : []),
    { label: '关闭通知', icon: XIcon }
  ]

  return (
    <div ref={setContainer} className="ml-auto flex shrink-0 items-center gap-0.5">
      {actions.map(action => (
        <Tooltip key={action.label}>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              className="cursor-pointer"
              onClick={() => {
                action.onClick?.()
                toast.dismiss(id)
              }}
            >
              <action.icon />
            </Button>
          </TooltipTrigger>
          <TooltipContent
            container={container?.closest<HTMLElement>('[data-sonner-toaster]')}
            side="top"
          >
            {action.label}
          </TooltipContent>
        </Tooltip>
      ))}
    </div>
  )
}
