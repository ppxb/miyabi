import { PlayIcon, XIcon, type LucideIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function TaskToastActions({ id, onPlay }: { id: string; onPlay?: () => void }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const actions: { label: string; icon: LucideIcon; onClick?: () => void }[] = [
    ...(onPlay ? [{ label: '播放', icon: PlayIcon, onClick: onPlay }] : []),
    { label: '关闭通知', icon: XIcon }
  ]

  return (
    <div ref={setContainer} className="ml-auto flex shrink-0 items-center gap-1">
      {actions.map(action => (
        <Tooltip key={action.label}>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"

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
